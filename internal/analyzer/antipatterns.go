package analyzer

import (
	"fmt"
	"strconv"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

const (
	maxArrayElements int64 = 100
	maxNestingDepth        = 5
	maxDocSizeBytes  int64 = 1_000_000 // 1 MB
	maxFieldCount          = 200
)

// DetectAntiPatterns analyzes sampled documents for common MongoDB data modeling mistakes.
func DetectAntiPatterns(samples []mongoinspect.FieldSampleResult) []Finding {
	if len(samples) == 0 {
		return nil
	}

	var findings []Finding
	for _, s := range samples {
		findings = append(findings, detectUnboundedArrays(&s)...)
		findings = append(findings, detectDeepNesting(&s)...)
		findings = append(findings, detectLargeDocument(&s)...)
		findings = append(findings, detectFieldNameCollision(&s)...)
		findings = append(findings, detectExcessiveFieldCount(&s)...)
		findings = append(findings, detectNumericFieldNames(&s)...)
	}
	return findings
}

// detectUnboundedArrays flags array fields with more than maxArrayElements elements.
func detectUnboundedArrays(s *mongoinspect.FieldSampleResult) []Finding {
	var findings []Finding
	for path, length := range s.ArrayLengths {
		if length > maxArrayElements {
			findings = append(findings, Finding{
				Type:       FindingUnboundedArray,
				Severity:   SeverityLow,
				Database:   s.Database,
				Collection: s.Collection,
				Message:    fmt.Sprintf("array field %q has up to %d elements — risk of unbounded growth", path, length),
			})
		}
	}
	return findings
}

// detectDeepNesting flags fields with path depth exceeding maxNestingDepth.
func detectDeepNesting(s *mongoinspect.FieldSampleResult) []Finding {
	seen := make(map[string]bool)
	var findings []Finding
	for _, f := range s.Fields {
		depth := fieldPathDepth(f.Path)
		if depth > maxNestingDepth && !seen[f.Path] {
			seen[f.Path] = true
			findings = append(findings, Finding{
				Type:       FindingDeepNesting,
				Severity:   SeverityLow,
				Database:   s.Database,
				Collection: s.Collection,
				Message:    fmt.Sprintf("field %q has nesting depth %d — hard to index and query", f.Path, depth),
			})
		}
	}
	return findings
}

// fieldPathDepth counts the nesting depth of a dot-separated field path,
// excluding array markers ([]). E.g., "a.b[].c.d.e.f" = 6.
func fieldPathDepth(path string) int {
	cleaned := strings.ReplaceAll(path, "[]", "")
	return strings.Count(cleaned, ".") + 1
}

// detectLargeDocument flags collections with documents approaching the BSON size limit.
func detectLargeDocument(s *mongoinspect.FieldSampleResult) []Finding {
	if s.MaxDocSize <= maxDocSizeBytes {
		return nil
	}
	return []Finding{{
		Type:       FindingLargeDocument,
		Severity:   SeverityLow,
		Database:   s.Database,
		Collection: s.Collection,
		Message:    fmt.Sprintf("largest sampled document is %s — approaching 16 MB BSON limit", formatBytes(s.MaxDocSize)),
	}}
}

// detectFieldNameCollision flags fields that appear as both object and scalar types.
func detectFieldNameCollision(s *mongoinspect.FieldSampleResult) []Finding {
	var findings []Finding
	for _, f := range s.Fields {
		hasObject := false
		hasScalar := false
		for typeName := range f.Types {
			if typeName == "object" {
				hasObject = true
			} else if typeName != "null" && typeName != "array" {
				hasScalar = true
			}
		}
		if hasObject && hasScalar {
			findings = append(findings, Finding{
				Type:       FindingFieldNameCollision,
				Severity:   SeverityLow,
				Database:   s.Database,
				Collection: s.Collection,
				Message:    fmt.Sprintf("field %q is both an object and scalar across documents", f.Path),
			})
		}
	}
	return findings
}

// detectExcessiveFieldCount flags documents with too many top-level fields.
func detectExcessiveFieldCount(s *mongoinspect.FieldSampleResult) []Finding {
	if s.MaxFieldCount <= maxFieldCount {
		return nil
	}
	return []Finding{{
		Type:       FindingExcessiveFieldCount,
		Severity:   SeverityInfo,
		Database:   s.Database,
		Collection: s.Collection,
		Message:    fmt.Sprintf("document has up to %d top-level fields — consider restructuring", s.MaxFieldCount),
	}}
}

// detectNumericFieldNames flags fields with purely numeric path segments,
// which typically indicate arrays stored as objects (SQL migration artifact).
func detectNumericFieldNames(s *mongoinspect.FieldSampleResult) []Finding {
	var findings []Finding
	seen := make(map[string]bool)
	for _, f := range s.Fields {
		segments := strings.Split(strings.ReplaceAll(f.Path, "[]", ""), ".")
		for _, seg := range segments {
			if seg == "" {
				continue
			}
			if _, err := strconv.Atoi(seg); err == nil {
				if !seen[f.Path] {
					seen[f.Path] = true
					findings = append(findings, Finding{
						Type:       FindingNumericFieldNames,
						Severity:   SeverityInfo,
						Database:   s.Database,
						Collection: s.Collection,
						Message:    fmt.Sprintf("field path %q contains numeric segment — likely array stored as object", f.Path),
					})
				}
				break
			}
		}
	}
	return findings
}

// formatBytes returns a human-readable size string.
func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d bytes", b)
	}
}
