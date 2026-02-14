package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiffBaseline_NewFindings(t *testing.T) {
	current := []Finding{
		{Type: FindingUnusedIndex, Database: "app", Collection: "users", Index: "idx_old"},
	}
	var baseline []Finding

	result := DiffBaseline(current, baseline)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Status != StatusNew {
		t.Errorf("expected new, got %s", result[0].Status)
	}
}

func TestDiffBaseline_ResolvedFindings(t *testing.T) {
	var current []Finding
	baseline := []Finding{
		{Type: FindingUnusedIndex, Database: "app", Collection: "users", Index: "idx_old"},
	}

	result := DiffBaseline(current, baseline)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Status != StatusResolved {
		t.Errorf("expected resolved, got %s", result[0].Status)
	}
}

func TestDiffBaseline_UnchangedFindings(t *testing.T) {
	finding := Finding{Type: FindingUnusedIndex, Database: "app", Collection: "users", Index: "idx_old"}
	current := []Finding{finding}
	baseline := []Finding{finding}

	result := DiffBaseline(current, baseline)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Status != StatusUnchanged {
		t.Errorf("expected unchanged, got %s", result[0].Status)
	}
}

func TestDiffBaseline_Mixed(t *testing.T) {
	current := []Finding{
		{Type: FindingUnusedIndex, Database: "app", Collection: "users", Index: "idx_old", Message: "unused"},
		{Type: FindingMissingIndex, Database: "app", Collection: "orders", Message: "new finding"},
	}
	baseline := []Finding{
		{Type: FindingUnusedIndex, Database: "app", Collection: "users", Index: "idx_old", Message: "unused"},
		{Type: FindingMissingTTL, Database: "app", Collection: "sessions", Message: "resolved finding"},
	}

	result := DiffBaseline(current, baseline)

	counts := map[BaselineStatus]int{}
	for _, r := range result {
		counts[r.Status]++
	}
	if counts[StatusNew] != 1 {
		t.Errorf("new = %d, want 1", counts[StatusNew])
	}
	if counts[StatusUnchanged] != 1 {
		t.Errorf("unchanged = %d, want 1", counts[StatusUnchanged])
	}
	if counts[StatusResolved] != 1 {
		t.Errorf("resolved = %d, want 1", counts[StatusResolved])
	}
}

func TestLoadBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	content := `{"findings":[{"type":"UNUSED_INDEX","severity":"medium","database":"app","collection":"users","index":"idx_old","message":"test"}]}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := LoadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingUnusedIndex {
		t.Errorf("type = %s", findings[0].Type)
	}
}

func TestLoadBaseline_Missing(t *testing.T) {
	_, err := LoadBaseline("/nonexistent/file.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadBaseline_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBaseline(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFindingKey(t *testing.T) {
	f1 := Finding{Type: FindingUnusedIndex, Database: "app", Collection: "users", Index: "idx_old"}
	f2 := Finding{Type: FindingUnusedIndex, Database: "app", Collection: "users", Index: "idx_old", Message: "different message"}
	if findingKey(f1) != findingKey(f2) {
		t.Error("same finding with different message should have same key")
	}

	f3 := Finding{Type: FindingMissingIndex, Database: "app", Collection: "users"}
	if findingKey(f1) == findingKey(f3) {
		t.Error("different types should have different keys")
	}
}
