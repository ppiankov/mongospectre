package analyzer

import (
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

func scanResult(collections ...string) scanner.ScanResult {
	var refs []scanner.CollectionRef
	for _, c := range collections {
		refs = append(refs, scanner.CollectionRef{Collection: c, File: "app.go", Line: 1})
	}
	return scanner.ScanResult{
		Refs:        refs,
		Collections: collections,
	}
}

func collInfo(name, db string, docCount int64, indexes ...mongoinspect.IndexInfo) mongoinspect.CollectionInfo {
	return mongoinspect.CollectionInfo{
		Name:     name,
		Database: db,
		DocCount: docCount,
		Indexes:  indexes,
	}
}

func TestDiff_MissingCollection(t *testing.T) {
	scan := scanResult("users", "orders")
	colls := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
	}

	findings := Diff(scan, colls)

	var missing []Finding
	for _, f := range findings {
		if f.Type == FindingMissingCollection {
			missing = append(missing, f)
		}
	}
	if len(missing) != 1 {
		t.Fatalf("expected 1 MISSING_COLLECTION, got %d", len(missing))
	}
	if missing[0].Collection != "orders" {
		t.Errorf("expected orders, got %s", missing[0].Collection)
	}
	if missing[0].Severity != SeverityHigh {
		t.Errorf("expected high severity, got %s", missing[0].Severity)
	}
}

func TestDiff_UnusedCollection(t *testing.T) {
	scan := scanResult("users")
	colls := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
		collInfo("legacy", "app", 0),
	}

	findings := Diff(scan, colls)

	var unused []Finding
	for _, f := range findings {
		if f.Type == FindingUnusedCollection {
			unused = append(unused, f)
		}
	}
	if len(unused) != 1 {
		t.Fatalf("expected 1 UNUSED_COLLECTION, got %d", len(unused))
	}
	if unused[0].Collection != "legacy" {
		t.Errorf("expected legacy, got %s", unused[0].Collection)
	}
}

func TestDiff_UnusedCollection_SkipsNonEmpty(t *testing.T) {
	scan := scanResult("users")
	colls := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
		collInfo("active_but_unreferenced", "app", 5000),
	}

	findings := Diff(scan, colls)

	for _, f := range findings {
		if f.Type == FindingUnusedCollection && f.Collection == "active_but_unreferenced" {
			t.Error("should not flag non-empty unreferenced collection as UNUSED_COLLECTION")
		}
	}
}

func TestDiff_OrphanedIndex(t *testing.T) {
	scan := scanResult("users")
	colls := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
		collInfo("legacy", "app", 500,
			mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
			idx("old_idx", kf("status"), 0),
		),
	}

	findings := Diff(scan, colls)

	var orphaned []Finding
	for _, f := range findings {
		if f.Type == FindingOrphanedIndex {
			orphaned = append(orphaned, f)
		}
	}
	if len(orphaned) != 1 {
		t.Fatalf("expected 1 ORPHANED_INDEX, got %d", len(orphaned))
	}
	if orphaned[0].Index != "old_idx" {
		t.Errorf("expected old_idx, got %s", orphaned[0].Index)
	}
}

func TestDiff_OK(t *testing.T) {
	scan := scanResult("users", "orders")
	colls := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
		collInfo("orders", "app", 200),
	}

	findings := Diff(scan, colls)

	var ok []Finding
	for _, f := range findings {
		if f.Type == FindingOK {
			ok = append(ok, f)
		}
	}
	if len(ok) != 2 {
		t.Fatalf("expected 2 OK, got %d", len(ok))
	}
}

func TestDiff_CaseInsensitive(t *testing.T) {
	scan := scanResult("Users")
	colls := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
	}

	findings := Diff(scan, colls)

	for _, f := range findings {
		if f.Type == FindingMissingCollection {
			t.Error("should match case-insensitively, but got MISSING_COLLECTION")
		}
	}

	var ok []Finding
	for _, f := range findings {
		if f.Type == FindingOK {
			ok = append(ok, f)
		}
	}
	if len(ok) != 1 {
		t.Errorf("expected 1 OK for case-insensitive match, got %d", len(ok))
	}
}

func TestDiff_EmptyCode(t *testing.T) {
	scan := scanResult()
	colls := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100),
	}

	findings := Diff(scan, colls)

	// No MISSING_COLLECTION (nothing referenced in code)
	for _, f := range findings {
		if f.Type == FindingMissingCollection {
			t.Error("unexpected MISSING_COLLECTION with empty code scan")
		}
	}
}

func TestDiff_EmptyDB(t *testing.T) {
	scan := scanResult("users", "orders")
	var colls []mongoinspect.CollectionInfo

	findings := Diff(scan, colls)

	var missing []Finding
	for _, f := range findings {
		if f.Type == FindingMissingCollection {
			missing = append(missing, f)
		}
	}
	if len(missing) != 2 {
		t.Errorf("expected 2 MISSING_COLLECTION, got %d", len(missing))
	}
}
