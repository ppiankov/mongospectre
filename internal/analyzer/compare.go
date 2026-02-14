package analyzer

import (
	"fmt"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

// CompareType identifies the category of comparison finding.
type CompareType string

const (
	CompareMissingInTarget CompareType = "MISSING_IN_TARGET"
	CompareMissingInSource CompareType = "MISSING_IN_SOURCE"
	CompareIndexDrift      CompareType = "INDEX_DRIFT"
)

// CompareFinding represents a difference between two clusters.
type CompareFinding struct {
	Type         CompareType `json:"type"`
	Severity     Severity    `json:"severity"`
	Database     string      `json:"database"`
	Collection   string      `json:"collection"`
	Index        string      `json:"index,omitempty"`
	Message      string      `json:"message"`
	SourceDetail string      `json:"sourceDetail,omitempty"`
	TargetDetail string      `json:"targetDetail,omitempty"`
}

// Compare detects drift between source and target cluster collections.
func Compare(source, target []mongoinspect.CollectionInfo) []CompareFinding {
	// Build lookups by collection name (lowercase).
	sourceByName := indexByName(source)
	targetByName := indexByName(target)

	var findings []CompareFinding

	// 1. MISSING_IN_TARGET: in source, not in target.
	for name, sc := range sourceByName {
		if _, ok := targetByName[name]; !ok {
			findings = append(findings, CompareFinding{
				Type:       CompareMissingInTarget,
				Severity:   SeverityHigh,
				Database:   sc.Database,
				Collection: sc.Name,
				Message:    fmt.Sprintf("collection %q exists in source but not in target", sc.Name),
			})
		}
	}

	// 2. MISSING_IN_SOURCE: in target, not in source.
	for name, tc := range targetByName {
		if _, ok := sourceByName[name]; !ok {
			findings = append(findings, CompareFinding{
				Type:       CompareMissingInSource,
				Severity:   SeverityMedium,
				Database:   tc.Database,
				Collection: tc.Name,
				Message:    fmt.Sprintf("collection %q exists in target but not in source", tc.Name),
			})
		}
	}

	// 3. INDEX_DRIFT: same collection, different indexes.
	for name, sc := range sourceByName {
		tc, ok := targetByName[name]
		if !ok {
			continue
		}
		findings = append(findings, compareIndexes(sc, tc)...)
	}

	return findings
}

// compareIndexes checks for index differences between two copies of the same collection.
func compareIndexes(source, target *mongoinspect.CollectionInfo) []CompareFinding {
	sourceIdx := indexSetByName(source.Indexes)
	targetIdx := indexSetByName(target.Indexes)

	var findings []CompareFinding

	// Indexes in source but not target.
	for name, si := range sourceIdx {
		if name == "_id_" {
			continue
		}
		ti, ok := targetIdx[name]
		if !ok {
			findings = append(findings, CompareFinding{
				Type:         CompareIndexDrift,
				Severity:     SeverityMedium,
				Database:     source.Database,
				Collection:   source.Name,
				Index:        name,
				Message:      fmt.Sprintf("index %q exists in source but not in target", name),
				SourceDetail: formatKeyFields(si.Key),
			})
			continue
		}
		// Same name, different key pattern.
		if formatKeyFields(si.Key) != formatKeyFields(ti.Key) {
			findings = append(findings, CompareFinding{
				Type:         CompareIndexDrift,
				Severity:     SeverityHigh,
				Database:     source.Database,
				Collection:   source.Name,
				Index:        name,
				Message:      fmt.Sprintf("index %q has different key pattern: source=%s target=%s", name, formatKeyFields(si.Key), formatKeyFields(ti.Key)),
				SourceDetail: formatKeyFields(si.Key),
				TargetDetail: formatKeyFields(ti.Key),
			})
		}
	}

	// Indexes in target but not source.
	for name := range targetIdx {
		if name == "_id_" {
			continue
		}
		if _, ok := sourceIdx[name]; !ok {
			findings = append(findings, CompareFinding{
				Type:         CompareIndexDrift,
				Severity:     SeverityLow,
				Database:     target.Database,
				Collection:   target.Name,
				Index:        name,
				Message:      fmt.Sprintf("index %q exists in target but not in source", name),
				TargetDetail: formatKeyFields(targetIdx[name].Key),
			})
		}
	}

	return findings
}

func indexByName(colls []mongoinspect.CollectionInfo) map[string]*mongoinspect.CollectionInfo {
	m := make(map[string]*mongoinspect.CollectionInfo)
	for i := range colls {
		m[strings.ToLower(colls[i].Name)] = &colls[i]
	}
	return m
}

func indexSetByName(indexes []mongoinspect.IndexInfo) map[string]mongoinspect.IndexInfo {
	m := make(map[string]mongoinspect.IndexInfo)
	for _, idx := range indexes {
		m[idx.Name] = idx
	}
	return m
}

func formatKeyFields(keys []mongoinspect.KeyField) string {
	parts := make([]string, len(keys))
	for i, kf := range keys {
		parts[i] = fmt.Sprintf("%s:%d", kf.Field, kf.Direction)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
