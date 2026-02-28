package analyzer

import (
	"encoding/json"
	"os"
	"time"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

// BaselineStatus indicates whether a finding is new, resolved, or unchanged.
type BaselineStatus string

const (
	StatusNew       BaselineStatus = "new"
	StatusResolved  BaselineStatus = "resolved"
	StatusUnchanged BaselineStatus = "unchanged"
)

// BaselineFinding wraps a Finding with a diff status against a baseline.
type BaselineFinding struct {
	Finding
	Status BaselineStatus `json:"status"`
}

// baselineReport is the minimal structure needed to load a previous JSON report.
type baselineReport struct {
	Metadata    baselineMetadata              `json:"metadata"`
	Findings    []Finding                     `json:"findings"`
	Collections []mongoinspect.CollectionInfo `json:"collections"`
}

// baselineMetadata holds the subset of report metadata needed for growth analysis.
type baselineMetadata struct {
	Timestamp string `json:"timestamp"`
}

// LoadBaseline reads a previous JSON report file and returns its findings.
func LoadBaseline(path string) ([]Finding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report baselineReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return report.Findings, nil
}

// LoadBaselineWithCollections reads a previous JSON report and returns findings,
// collection stats, and the report timestamp. Missing collections or timestamp
// return zero values without error (caller decides whether to skip growth analysis).
func LoadBaselineWithCollections(path string) ([]Finding, []mongoinspect.CollectionInfo, time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	var report baselineReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, nil, time.Time{}, err
	}
	var ts time.Time
	if report.Metadata.Timestamp != "" {
		ts, _ = time.Parse(time.RFC3339, report.Metadata.Timestamp)
	}
	return report.Findings, report.Collections, ts, nil
}

// DiffBaseline compares current findings against baseline findings.
// Returns tagged findings with status new/resolved/unchanged.
func DiffBaseline(current, baseline []Finding) []BaselineFinding {
	baselineSet := make(map[string]bool)
	for _, f := range baseline {
		baselineSet[findingKey(&f)] = true
	}

	currentSet := make(map[string]bool)
	for _, f := range current {
		currentSet[findingKey(&f)] = true
	}

	var result []BaselineFinding

	// Current findings: new or unchanged.
	for _, f := range current {
		status := StatusNew
		if baselineSet[findingKey(&f)] {
			status = StatusUnchanged
		}
		result = append(result, BaselineFinding{Finding: f, Status: status})
	}

	// Baseline findings not in current: resolved.
	for _, f := range baseline {
		if !currentSet[findingKey(&f)] {
			result = append(result, BaselineFinding{Finding: f, Status: StatusResolved})
		}
	}

	return result
}

// findingKey creates a stable identity for a finding based on type+location.
func findingKey(f *Finding) string {
	key := string(f.Type) + "|" + f.Database + "|" + f.Collection
	if f.Index != "" {
		key += "|" + f.Index
	}
	return key
}
