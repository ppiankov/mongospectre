package analyzer

import (
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

func TestCompare_MissingInTarget(t *testing.T) {
	source := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
		collInfo("orders", "app", 200),
	}
	target := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
	}

	findings := Compare(source, target)

	var missing []CompareFinding
	for _, f := range findings {
		if f.Type == CompareMissingInTarget {
			missing = append(missing, f)
		}
	}
	if len(missing) != 1 {
		t.Fatalf("expected 1 MISSING_IN_TARGET, got %d", len(missing))
	}
	if missing[0].Collection != "orders" {
		t.Errorf("expected orders, got %s", missing[0].Collection)
	}
}

func TestCompare_MissingInSource(t *testing.T) {
	source := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
	}
	target := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
		collInfo("sessions", "app", 50),
	}

	findings := Compare(source, target)

	var missing []CompareFinding
	for _, f := range findings {
		if f.Type == CompareMissingInSource {
			missing = append(missing, f)
		}
	}
	if len(missing) != 1 {
		t.Fatalf("expected 1 MISSING_IN_SOURCE, got %d", len(missing))
	}
	if missing[0].Collection != "sessions" {
		t.Errorf("expected sessions, got %s", missing[0].Collection)
	}
}

func TestCompare_IndexDrift_MissingInTarget(t *testing.T) {
	source := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100,
			mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
			idx("email_1", kf("email"), 0),
		),
	}
	target := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100,
			mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
		),
	}

	findings := Compare(source, target)

	var drift []CompareFinding
	for _, f := range findings {
		if f.Type == CompareIndexDrift {
			drift = append(drift, f)
		}
	}
	if len(drift) != 1 {
		t.Fatalf("expected 1 INDEX_DRIFT, got %d", len(drift))
	}
	if drift[0].Index != "email_1" {
		t.Errorf("expected email_1, got %s", drift[0].Index)
	}
}

func TestCompare_IndexDrift_DifferentKeys(t *testing.T) {
	source := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100,
			mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
			idx("status_1", kf("status"), 0),
		),
	}
	target := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100,
			mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
			mongoinspect.IndexInfo{Name: "status_1", Key: kf("status", "-created_at")},
		),
	}

	findings := Compare(source, target)

	var drift []CompareFinding
	for _, f := range findings {
		if f.Type == CompareIndexDrift && f.Index == "status_1" {
			drift = append(drift, f)
		}
	}
	if len(drift) != 1 {
		t.Fatalf("expected 1 INDEX_DRIFT for key mismatch, got %d", len(drift))
	}
	if drift[0].Severity != SeverityHigh {
		t.Errorf("expected high severity for key mismatch, got %s", drift[0].Severity)
	}
}

func TestCompare_Identical(t *testing.T) {
	coll := collInfo("users", "app", 100,
		mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
		idx("email_1", kf("email"), 0),
	)
	source := []mongoinspect.CollectionInfo{coll}
	target := []mongoinspect.CollectionInfo{coll}

	findings := Compare(source, target)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for identical clusters, got %d: %v", len(findings), findings)
	}
}

func TestCompare_Empty(t *testing.T) {
	findings := Compare(nil, nil)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestFormatKeyFields(t *testing.T) {
	keys := kf("status", "-created_at")
	got := formatKeyFields(keys)
	want := "{status:1, created_at:-1}"
	if got != want {
		t.Errorf("formatKeyFields = %q, want %q", got, want)
	}
}
