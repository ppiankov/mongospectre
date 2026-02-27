package analyzer

import (
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

func TestDetectSchemaDrift_MissingField(t *testing.T) {
	scan := &scanner.ScanResult{
		FieldRefs: []scanner.FieldRef{
			{Collection: "users", Field: "zipCode", File: "app.go", Line: 10},
		},
	}
	samples := []mongoinspect.FieldSampleResult{
		{
			Database:   "mydb",
			Collection: "users",
			SampleSize: 100,
			Fields: []mongoinspect.FieldFrequency{
				{Path: "name", Count: 100, Types: map[string]int64{"string": 100}},
				{Path: "age", Count: 95, Types: map[string]int64{"int32": 95}},
			},
		},
	}

	findings := DetectSchemaDrift(scan, samples)
	found := false
	for _, f := range findings {
		if f.Type == FindingMissingField && f.Collection == "users" {
			found = true
			if f.Severity != SeverityMedium {
				t.Errorf("severity = %s, want medium", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected MISSING_FIELD finding for 'zipCode'")
	}
}

func TestDetectSchemaDrift_RareField(t *testing.T) {
	scan := &scanner.ScanResult{
		FieldRefs: []scanner.FieldRef{
			{Collection: "users", Field: "middleName", File: "app.go", Line: 10},
		},
	}
	samples := []mongoinspect.FieldSampleResult{
		{
			Database:   "mydb",
			Collection: "users",
			SampleSize: 100,
			Fields: []mongoinspect.FieldFrequency{
				{Path: "middleName", Count: 3, Types: map[string]int64{"string": 3}},
			},
		},
	}

	findings := DetectSchemaDrift(scan, samples)
	found := false
	for _, f := range findings {
		if f.Type == FindingRareField && f.Collection == "users" {
			found = true
			if f.Severity != SeverityLow {
				t.Errorf("severity = %s, want low", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected RARE_FIELD finding for 'middleName'")
	}
}

func TestDetectSchemaDrift_UndocumentedField(t *testing.T) {
	scan := &scanner.ScanResult{
		FieldRefs: []scanner.FieldRef{
			{Collection: "users", Field: "name", File: "app.go", Line: 10},
		},
	}
	samples := []mongoinspect.FieldSampleResult{
		{
			Database:   "mydb",
			Collection: "users",
			SampleSize: 100,
			Fields: []mongoinspect.FieldFrequency{
				{Path: "_id", Count: 100, Types: map[string]int64{"objectId": 100}},
				{Path: "name", Count: 100, Types: map[string]int64{"string": 100}},
				{Path: "legacyField", Count: 95, Types: map[string]int64{"string": 95}},
			},
		},
	}

	findings := DetectSchemaDrift(scan, samples)
	found := false
	for _, f := range findings {
		if f.Type == FindingUndocumentedField && f.Collection == "users" {
			found = true
			if f.Severity != SeverityInfo {
				t.Errorf("severity = %s, want info", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected UNDOCUMENTED_FIELD finding for 'legacyField'")
	}

	// _id should NOT be flagged as undocumented.
	for _, f := range findings {
		if f.Type == FindingUndocumentedField && f.Message != "" {
			if f.Message == "_id" {
				t.Error("_id should not be flagged as undocumented")
			}
		}
	}
}

func TestDetectSchemaDrift_TypeInconsistency(t *testing.T) {
	scan := &scanner.ScanResult{}
	samples := []mongoinspect.FieldSampleResult{
		{
			Database:   "mydb",
			Collection: "users",
			SampleSize: 100,
			Fields: []mongoinspect.FieldFrequency{
				{Path: "age", Count: 100, Types: map[string]int64{
					"int32":  80,
					"string": 20,
				}},
			},
		},
	}

	findings := DetectSchemaDrift(scan, samples)
	found := false
	for _, f := range findings {
		if f.Type == FindingTypeInconsistency && f.Collection == "users" {
			found = true
			if f.Severity != SeverityMedium {
				t.Errorf("severity = %s, want medium", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected TYPE_INCONSISTENCY finding for 'age'")
	}
}

func TestDetectSchemaDrift_TypeInconsistency_NullIgnored(t *testing.T) {
	scan := &scanner.ScanResult{}
	samples := []mongoinspect.FieldSampleResult{
		{
			Database:   "mydb",
			Collection: "users",
			SampleSize: 100,
			Fields: []mongoinspect.FieldFrequency{
				{Path: "nickname", Count: 100, Types: map[string]int64{
					"string": 80,
					"null":   20,
				}},
			},
		},
	}

	findings := DetectSchemaDrift(scan, samples)
	for _, f := range findings {
		if f.Type == FindingTypeInconsistency {
			t.Error("should not flag type inconsistency when only difference is null")
		}
	}
}

func TestDetectSchemaDrift_AllPresent(t *testing.T) {
	scan := &scanner.ScanResult{
		FieldRefs: []scanner.FieldRef{
			{Collection: "users", Field: "name", File: "app.go", Line: 10},
			{Collection: "users", Field: "email", File: "app.go", Line: 11},
		},
	}
	samples := []mongoinspect.FieldSampleResult{
		{
			Database:   "mydb",
			Collection: "users",
			SampleSize: 100,
			Fields: []mongoinspect.FieldFrequency{
				{Path: "_id", Count: 100, Types: map[string]int64{"objectId": 100}},
				{Path: "name", Count: 100, Types: map[string]int64{"string": 100}},
				{Path: "email", Count: 98, Types: map[string]int64{"string": 98}},
			},
		},
	}

	findings := DetectSchemaDrift(scan, samples)
	for _, f := range findings {
		if f.Type == FindingMissingField || f.Type == FindingRareField {
			t.Errorf("unexpected finding: %s for %s", f.Type, f.Message)
		}
	}
}

func TestDetectSchemaDrift_EmptySamples(t *testing.T) {
	scan := &scanner.ScanResult{
		FieldRefs: []scanner.FieldRef{
			{Collection: "users", Field: "name", File: "app.go", Line: 10},
		},
	}

	findings := DetectSchemaDrift(scan, nil)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for nil samples, got %d", len(findings))
	}

	findings = DetectSchemaDrift(scan, []mongoinspect.FieldSampleResult{})
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty samples, got %d", len(findings))
	}
}

func TestDetectSchemaDrift_EmptyScan(t *testing.T) {
	scan := &scanner.ScanResult{}
	samples := []mongoinspect.FieldSampleResult{
		{
			Database:   "mydb",
			Collection: "users",
			SampleSize: 100,
			Fields: []mongoinspect.FieldFrequency{
				{Path: "_id", Count: 100, Types: map[string]int64{"objectId": 100}},
				{Path: "legacyField", Count: 95, Types: map[string]int64{"string": 95}},
			},
		},
	}

	findings := DetectSchemaDrift(scan, samples)
	// With no code refs, legacyField should still be flagged as undocumented.
	found := false
	for _, f := range findings {
		if f.Type == FindingUndocumentedField {
			found = true
		}
	}
	if !found {
		t.Error("expected UNDOCUMENTED_FIELD finding even with empty scan")
	}
}

func TestDetectSchemaDrift_WriteRefs(t *testing.T) {
	scan := &scanner.ScanResult{
		WriteRefs: []scanner.WriteRef{
			{Collection: "orders", Field: "shippingAddress", File: "order.go", Line: 20},
		},
	}
	samples := []mongoinspect.FieldSampleResult{
		{
			Database:   "mydb",
			Collection: "orders",
			SampleSize: 100,
			Fields: []mongoinspect.FieldFrequency{
				{Path: "_id", Count: 100, Types: map[string]int64{"objectId": 100}},
				{Path: "total", Count: 100, Types: map[string]int64{"double": 100}},
			},
		},
	}

	findings := DetectSchemaDrift(scan, samples)
	found := false
	for _, f := range findings {
		if f.Type == FindingMissingField && f.Collection == "orders" {
			found = true
		}
	}
	if !found {
		t.Error("expected MISSING_FIELD finding for WriteRef 'shippingAddress'")
	}
}
