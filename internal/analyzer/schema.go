package analyzer

import (
	"fmt"
	"sort"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

const (
	rareFieldThreshold         = 0.10 // field present in <10% of sampled docs
	undocumentedFieldThreshold = 0.90 // field present in >90% of sampled docs with no code ref
)

// DetectSchemaDrift compares code field references against sampled document fields
// to detect field-level drift between code and live data.
func DetectSchemaDrift(scan *scanner.ScanResult, samples []mongoinspect.FieldSampleResult) []Finding {
	if len(samples) == 0 {
		return nil
	}

	// Build a map of code field references by collection (lowercased).
	// codeFields[collection][field] = true
	codeFields := buildCodeFieldMap(scan)

	// Build a lookup from sample results by collection.
	sampleByCollection := make(map[string]mongoinspect.FieldSampleResult, len(samples))
	for _, s := range samples {
		key := strings.ToLower(s.Collection)
		sampleByCollection[key] = s
	}

	var findings []Finding

	// Check code fields against samples.
	for collName, fields := range codeFields {
		sample, ok := sampleByCollection[collName]
		if !ok {
			continue // no sample data for this collection
		}

		// Build field lookup from sample.
		sampleFields := make(map[string]mongoinspect.FieldFrequency, len(sample.Fields))
		for _, f := range sample.Fields {
			sampleFields[f.Path] = f
		}

		for _, field := range sortedBoolKeys(fields) {
			sf, exists := sampleFields[field]
			if !exists {
				findings = append(findings, Finding{
					Type:       FindingMissingField,
					Severity:   SeverityMedium,
					Database:   sample.Database,
					Collection: sample.Collection,
					Message:    fmt.Sprintf("field %q referenced in code but not found in %d sampled documents", field, sample.SampleSize),
				})
				continue
			}

			ratio := float64(sf.Count) / float64(sample.SampleSize)
			if ratio < rareFieldThreshold {
				findings = append(findings, Finding{
					Type:       FindingRareField,
					Severity:   SeverityLow,
					Database:   sample.Database,
					Collection: sample.Collection,
					Message:    fmt.Sprintf("field %q referenced in code but present in only %d/%d sampled documents (%.0f%%)", field, sf.Count, sample.SampleSize, ratio*100),
				})
			}
		}
	}

	// Check for type inconsistencies and undocumented fields.
	for _, sample := range samples {
		collKey := strings.ToLower(sample.Collection)
		collCodeFields := codeFields[collKey]

		for _, sf := range sample.Fields {
			// Type inconsistency: multiple non-null BSON types.
			nonNullTypes := countNonNullTypes(sf.Types)
			if nonNullTypes > 1 {
				typeList := formatTypeList(sf.Types)
				findings = append(findings, Finding{
					Type:       FindingTypeInconsistency,
					Severity:   SeverityMedium,
					Database:   sample.Database,
					Collection: sample.Collection,
					Message:    fmt.Sprintf("field %q has inconsistent types in %d sampled documents: %s", sf.Path, sample.SampleSize, typeList),
				})
			}

			// Undocumented field: high presence in sample but no code reference.
			if isSystemField(sf.Path) {
				continue
			}
			ratio := float64(sf.Count) / float64(sample.SampleSize)
			if ratio >= undocumentedFieldThreshold && !collCodeFields[sf.Path] {
				findings = append(findings, Finding{
					Type:       FindingUndocumentedField,
					Severity:   SeverityInfo,
					Database:   sample.Database,
					Collection: sample.Collection,
					Message:    fmt.Sprintf("field %q found in %d/%d sampled documents (%.0f%%) but not referenced in code", sf.Path, sf.Count, sample.SampleSize, ratio*100),
				})
			}
		}
	}

	return findings
}

// buildCodeFieldMap extracts unique field names by collection from FieldRefs and WriteRefs.
func buildCodeFieldMap(scan *scanner.ScanResult) map[string]map[string]bool {
	result := make(map[string]map[string]bool)

	for _, fr := range scan.FieldRefs {
		coll := strings.ToLower(strings.TrimSpace(fr.Collection))
		if coll == "" || fr.Field == "" {
			continue
		}
		if result[coll] == nil {
			result[coll] = make(map[string]bool)
		}
		result[coll][fr.Field] = true
	}

	for _, wr := range scan.WriteRefs {
		coll := strings.ToLower(strings.TrimSpace(wr.Collection))
		if coll == "" || wr.Field == "" {
			continue
		}
		if result[coll] == nil {
			result[coll] = make(map[string]bool)
		}
		result[coll][wr.Field] = true
	}

	return result
}

// countNonNullTypes returns the number of distinct BSON types excluding "null".
func countNonNullTypes(types map[string]int64) int {
	count := 0
	for t := range types {
		if t != "null" {
			count++
		}
	}
	return count
}

// formatTypeList returns a sorted, human-readable summary of type counts.
func formatTypeList(types map[string]int64) string {
	keys := make([]string, 0, len(types))
	for t := range types {
		keys = append(keys, t)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, t := range keys {
		parts = append(parts, fmt.Sprintf("%s(%d)", t, types[t]))
	}
	return strings.Join(parts, ", ")
}

// isSystemField returns true for fields that should not be flagged as undocumented.
func isSystemField(path string) bool {
	return path == "_id" || strings.HasPrefix(path, "_id.")
}
