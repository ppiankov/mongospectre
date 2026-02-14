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

func TestDiff_UnindexedQuery(t *testing.T) {
	scan := scanner.ScanResult{
		Refs:        []scanner.CollectionRef{{Collection: "users", File: "app.go", Line: 1}},
		Collections: []string{"users"},
		FieldRefs: []scanner.FieldRef{
			{Collection: "users", Field: "email", File: "app.go", Line: 10},
			{Collection: "users", Field: "status", File: "app.go", Line: 15},
			{Collection: "users", Field: "_id", File: "app.go", Line: 20}, // should be skipped
		},
	}
	colls := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100,
			mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
			idx("status_1", kf("status"), 50), // status is indexed
		),
	}

	findings := Diff(scan, colls)

	var unindexed []Finding
	for _, f := range findings {
		if f.Type == FindingUnindexedQuery {
			unindexed = append(unindexed, f)
		}
	}
	if len(unindexed) != 1 {
		t.Fatalf("expected 1 UNINDEXED_QUERY, got %d: %v", len(unindexed), unindexed)
	}
	if unindexed[0].Collection != "users" {
		t.Errorf("expected users, got %s", unindexed[0].Collection)
	}
	if unindexed[0].Severity != SeverityMedium {
		t.Errorf("expected medium severity, got %s", unindexed[0].Severity)
	}
}

func TestDiff_UnindexedQuery_AllIndexed(t *testing.T) {
	scan := scanner.ScanResult{
		Refs:        []scanner.CollectionRef{{Collection: "users", File: "app.go", Line: 1}},
		Collections: []string{"users"},
		FieldRefs: []scanner.FieldRef{
			{Collection: "users", Field: "email", File: "app.go", Line: 10},
		},
	}
	colls := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 100,
			mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
			idx("email_1", kf("email"), 50),
		),
	}

	findings := Diff(scan, colls)

	for _, f := range findings {
		if f.Type == FindingUnindexedQuery {
			t.Errorf("unexpected UNINDEXED_QUERY: %s", f.Message)
		}
	}
}

func TestDiff_SuggestIndex(t *testing.T) {
	scan := scanner.ScanResult{
		Refs:        []scanner.CollectionRef{{Collection: "orders", File: "app.go", Line: 1}},
		Collections: []string{"orders"},
		FieldRefs: []scanner.FieldRef{
			{Collection: "orders", Field: "customer_id", File: "app.go", Line: 10},
			{Collection: "orders", Field: "status", File: "app.go", Line: 15},
		},
	}
	colls := []mongoinspect.CollectionInfo{
		collInfo("orders", "app", 50000, // above suggestMinDocs
			mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
		),
	}

	findings := Diff(scan, colls)

	var suggestions []Finding
	for _, f := range findings {
		if f.Type == FindingSuggestIndex {
			suggestions = append(suggestions, f)
		}
	}
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 SUGGEST_INDEX, got %d: %v", len(suggestions), suggestions)
	}
}

func TestDiff_SuggestIndex_SkipsSmallCollections(t *testing.T) {
	scan := scanner.ScanResult{
		Refs:        []scanner.CollectionRef{{Collection: "small", File: "app.go", Line: 1}},
		Collections: []string{"small"},
		FieldRefs: []scanner.FieldRef{
			{Collection: "small", Field: "status", File: "app.go", Line: 10},
		},
	}
	colls := []mongoinspect.CollectionInfo{
		collInfo("small", "app", 50, // below suggestMinDocs
			mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
		),
	}

	findings := Diff(scan, colls)

	for _, f := range findings {
		if f.Type == FindingSuggestIndex {
			t.Error("should not suggest indexes for small collections")
		}
	}
}

func TestDiff_SuggestIndex_SkipsIndexedFields(t *testing.T) {
	scan := scanner.ScanResult{
		Refs:        []scanner.CollectionRef{{Collection: "users", File: "app.go", Line: 1}},
		Collections: []string{"users"},
		FieldRefs: []scanner.FieldRef{
			{Collection: "users", Field: "email", File: "app.go", Line: 10},
		},
	}
	colls := []mongoinspect.CollectionInfo{
		collInfo("users", "app", 5000,
			mongoinspect.IndexInfo{Name: "_id_", Key: kf("_id")},
			idx("email_1", kf("email"), 50),
		),
	}

	findings := Diff(scan, colls)

	for _, f := range findings {
		if f.Type == FindingSuggestIndex {
			t.Errorf("should not suggest index for already-indexed field: %s", f.Message)
		}
	}
}

func TestDiff_UnindexedQuery_MissingCollection(t *testing.T) {
	// Field refs on a collection that doesn't exist in DB should not produce
	// UNINDEXED_QUERY (already reported as MISSING_COLLECTION).
	scan := scanner.ScanResult{
		Refs:        []scanner.CollectionRef{{Collection: "orders", File: "app.go", Line: 1}},
		Collections: []string{"orders"},
		FieldRefs: []scanner.FieldRef{
			{Collection: "orders", Field: "status", File: "app.go", Line: 10},
		},
	}
	var colls []mongoinspect.CollectionInfo

	findings := Diff(scan, colls)

	for _, f := range findings {
		if f.Type == FindingUnindexedQuery {
			t.Error("should not report UNINDEXED_QUERY for missing collection")
		}
	}
}
