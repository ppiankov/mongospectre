package analyzer

// Severity indicates the risk level of a finding.
type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
	SeverityInfo   Severity = "info"
)

// FindingType identifies the category of audit finding.
type FindingType string

const (
	FindingUnusedCollection    FindingType = "UNUSED_COLLECTION"
	FindingUnusedIndex         FindingType = "UNUSED_INDEX"
	FindingMissingIndex        FindingType = "MISSING_INDEX"
	FindingDuplicateIndex      FindingType = "DUPLICATE_INDEX"
	FindingOversizedCollection FindingType = "OVERSIZED_COLLECTION"
	FindingMissingTTL          FindingType = "MISSING_TTL"
)

// Finding represents a single audit detection result.
type Finding struct {
	Type       FindingType `json:"type"`
	Severity   Severity    `json:"severity"`
	Database   string      `json:"database"`
	Collection string      `json:"collection"`
	Index      string      `json:"index,omitempty"`
	Message    string      `json:"message"`
}

// MaxSeverity returns the highest severity found in a list of findings.
// Returns SeverityInfo if the list is empty.
func MaxSeverity(findings []Finding) Severity {
	order := map[Severity]int{
		SeverityInfo:   0,
		SeverityLow:    1,
		SeverityMedium: 2,
		SeverityHigh:   3,
	}
	max := SeverityInfo
	for _, f := range findings {
		if order[f.Severity] > order[max] {
			max = f.Severity
		}
	}
	return max
}

// ExitCode maps severity to a process exit code.
func ExitCode(s Severity) int {
	switch s {
	case SeverityHigh:
		return 2
	case SeverityMedium:
		return 1
	default:
		return 0
	}
}
