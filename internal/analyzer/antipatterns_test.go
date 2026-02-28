package analyzer

import (
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

func TestDetectUnboundedArrays(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "events",
		ArrayLengths: map[string]int64{
			"tags":    50,
			"entries": 200,
		},
	}
	findings := detectUnboundedArrays(&s)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingUnboundedArray {
		t.Errorf("expected type %s, got %s", FindingUnboundedArray, findings[0].Type)
	}
}

func TestDetectUnboundedArrays_Under(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "events",
		ArrayLengths: map[string]int64{
			"tags": 100,
		},
	}
	if findings := detectUnboundedArrays(&s); len(findings) != 0 {
		t.Errorf("expected 0 findings for 100 elements, got %d", len(findings))
	}
}

func TestDetectDeepNesting(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "docs",
		Fields: []mongoinspect.FieldFrequency{
			{Path: "a.b.c.d.e.f", Count: 10, Types: map[string]int64{"string": 10}},
			{Path: "x.y", Count: 10, Types: map[string]int64{"string": 10}},
		},
	}
	findings := detectDeepNesting(&s)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingDeepNesting {
		t.Errorf("expected type %s, got %s", FindingDeepNesting, findings[0].Type)
	}
}

func TestDetectDeepNesting_Shallow(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "docs",
		Fields: []mongoinspect.FieldFrequency{
			{Path: "a.b.c.d.e", Count: 10, Types: map[string]int64{"string": 10}},
		},
	}
	if findings := detectDeepNesting(&s); len(findings) != 0 {
		t.Errorf("expected 0 findings for depth 5, got %d", len(findings))
	}
}

func TestDetectDeepNesting_ArrayPath(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "docs",
		Fields: []mongoinspect.FieldFrequency{
			{Path: "items[].variants[].options[].name.value.data", Count: 5, Types: map[string]int64{"string": 5}},
		},
	}
	findings := detectDeepNesting(&s)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for deep nested array path, got %d", len(findings))
	}
}

func TestDetectLargeDocument(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "blobs",
		MaxDocSize: 2_000_000,
	}
	findings := detectLargeDocument(&s)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingLargeDocument {
		t.Errorf("expected type %s, got %s", FindingLargeDocument, findings[0].Type)
	}
}

func TestDetectLargeDocument_Small(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "users",
		MaxDocSize: 1_000_000,
	}
	if findings := detectLargeDocument(&s); len(findings) != 0 {
		t.Errorf("expected 0 findings for 1MB doc, got %d", len(findings))
	}
}

func TestDetectFieldNameCollision(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "users",
		Fields: []mongoinspect.FieldFrequency{
			{Path: "address", Count: 100, Types: map[string]int64{"object": 80, "string": 20}},
		},
	}
	findings := detectFieldNameCollision(&s)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingFieldNameCollision {
		t.Errorf("expected type %s, got %s", FindingFieldNameCollision, findings[0].Type)
	}
}

func TestDetectFieldNameCollision_NoConflict(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "users",
		Fields: []mongoinspect.FieldFrequency{
			{Path: "address", Count: 100, Types: map[string]int64{"object": 100}},
			{Path: "name", Count: 100, Types: map[string]int64{"string": 90, "null": 10}},
		},
	}
	if findings := detectFieldNameCollision(&s); len(findings) != 0 {
		t.Errorf("expected 0 findings for consistent types, got %d", len(findings))
	}
}

func TestDetectExcessiveFieldCount(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:      "db",
		Collection:    "logs",
		MaxFieldCount: 250,
	}
	findings := detectExcessiveFieldCount(&s)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingExcessiveFieldCount {
		t.Errorf("expected type %s, got %s", FindingExcessiveFieldCount, findings[0].Type)
	}
	if findings[0].Severity != SeverityInfo {
		t.Errorf("severity = %s, want info", findings[0].Severity)
	}
}

func TestDetectExcessiveFieldCount_Under(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:      "db",
		Collection:    "users",
		MaxFieldCount: 200,
	}
	if findings := detectExcessiveFieldCount(&s); len(findings) != 0 {
		t.Errorf("expected 0 findings for 200 fields, got %d", len(findings))
	}
}

func TestDetectNumericFieldNames(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "migrated",
		Fields: []mongoinspect.FieldFrequency{
			{Path: "data.0.value", Count: 10, Types: map[string]int64{"string": 10}},
			{Path: "name", Count: 10, Types: map[string]int64{"string": 10}},
		},
	}
	findings := detectNumericFieldNames(&s)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingNumericFieldNames {
		t.Errorf("expected type %s, got %s", FindingNumericFieldNames, findings[0].Type)
	}
	if findings[0].Severity != SeverityInfo {
		t.Errorf("severity = %s, want info", findings[0].Severity)
	}
}

func TestDetectNumericFieldNames_Normal(t *testing.T) {
	s := mongoinspect.FieldSampleResult{
		Database:   "db",
		Collection: "users",
		Fields: []mongoinspect.FieldFrequency{
			{Path: "address.street", Count: 10, Types: map[string]int64{"string": 10}},
			{Path: "name", Count: 10, Types: map[string]int64{"string": 10}},
		},
	}
	if findings := detectNumericFieldNames(&s); len(findings) != 0 {
		t.Errorf("expected 0 findings for normal paths, got %d", len(findings))
	}
}

func TestDetectAntiPatterns_Empty(t *testing.T) {
	if findings := DetectAntiPatterns(nil); findings != nil {
		t.Errorf("expected nil for nil input, got %d findings", len(findings))
	}
	if findings := DetectAntiPatterns([]mongoinspect.FieldSampleResult{}); findings != nil {
		t.Errorf("expected nil for empty input, got %d findings", len(findings))
	}
}

func TestFieldPathDepth(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{"name", 1},
		{"a.b", 2},
		{"a.b.c.d.e", 5},
		{"a.b.c.d.e.f", 6},
		{"items[].name", 2},
		{"items[].variants[].options[].name", 4},
		{"a[].b[].c[].d[].e[].f", 6},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := fieldPathDepth(tt.path)
			if got != tt.want {
				t.Errorf("fieldPathDepth(%q) = %d, want %d", tt.path, got, tt.want)
			}
		})
	}
}
