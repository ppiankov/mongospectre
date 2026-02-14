package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIgnoreRule(t *testing.T) {
	tests := []struct {
		line string
		ok   bool
		want IgnoreRule
	}{
		{"UNUSED_INDEX app.users.idx_old", true, IgnoreRule{Type: "UNUSED_INDEX", Database: "app", Collection: "users", Index: "idx_old"}},
		{"* app.audit_logs", true, IgnoreRule{Type: "*", Database: "app", Collection: "audit_logs"}},
		{"MISSING_TTL app.settings", true, IgnoreRule{Type: "MISSING_TTL", Database: "app", Collection: "settings"}},
		{"UNUSED_COLLECTION migrations", true, IgnoreRule{Type: "UNUSED_COLLECTION", Database: "*", Collection: "migrations"}},
		{"", false, IgnoreRule{}},
		{"ONLY_TYPE", false, IgnoreRule{}},
	}
	for _, tt := range tests {
		rule, ok := parseIgnoreRule(tt.line)
		if ok != tt.ok {
			t.Errorf("parseIgnoreRule(%q) ok = %v, want %v", tt.line, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if rule != tt.want {
			t.Errorf("parseIgnoreRule(%q) = %+v, want %+v", tt.line, rule, tt.want)
		}
	}
}

func TestIgnoreRule_Matches(t *testing.T) {
	finding := Finding{
		Type:       FindingUnusedIndex,
		Database:   "app",
		Collection: "users",
		Index:      "idx_old",
	}

	tests := []struct {
		name string
		rule IgnoreRule
		want bool
	}{
		{"exact match", IgnoreRule{Type: "UNUSED_INDEX", Database: "app", Collection: "users", Index: "idx_old"}, true},
		{"wildcard type", IgnoreRule{Type: "*", Database: "app", Collection: "users"}, true},
		{"wildcard db", IgnoreRule{Type: "UNUSED_INDEX", Database: "*", Collection: "users"}, true},
		{"wrong type", IgnoreRule{Type: "MISSING_TTL", Database: "app", Collection: "users"}, false},
		{"wrong collection", IgnoreRule{Type: "UNUSED_INDEX", Database: "app", Collection: "orders"}, false},
		{"wrong index", IgnoreRule{Type: "UNUSED_INDEX", Database: "app", Collection: "users", Index: "idx_new"}, false},
		{"collection glob", IgnoreRule{Type: "*", Database: "app", Collection: "user*"}, true},
		{"db glob", IgnoreRule{Type: "*", Database: "app*", Collection: "*"}, true},
	}
	for _, tt := range tests {
		got := tt.rule.Matches(&finding)
		if got != tt.want {
			t.Errorf("%s: Matches = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIgnoreList_Filter(t *testing.T) {
	findings := []Finding{
		{Type: FindingUnusedIndex, Database: "app", Collection: "users", Index: "idx_old"},
		{Type: FindingMissingTTL, Database: "app", Collection: "sessions"},
		{Type: FindingUnusedCollection, Database: "app", Collection: "legacy"},
	}

	il := IgnoreList{Rules: []IgnoreRule{
		{Type: "UNUSED_INDEX", Database: "app", Collection: "users", Index: "idx_old"},
		{Type: "*", Database: "app", Collection: "legacy"},
	}}

	filtered, suppressed := il.Filter(findings)
	if suppressed != 2 {
		t.Errorf("suppressed = %d, want 2", suppressed)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered = %d, want 1", len(filtered))
	}
	if filtered[0].Type != FindingMissingTTL {
		t.Errorf("expected MISSING_TTL, got %s", filtered[0].Type)
	}
}

func TestIgnoreList_Filter_Empty(t *testing.T) {
	findings := []Finding{
		{Type: FindingUnusedIndex, Database: "app", Collection: "users"},
	}
	il := IgnoreList{}
	filtered, suppressed := il.Filter(findings)
	if suppressed != 0 {
		t.Errorf("suppressed = %d, want 0", suppressed)
	}
	if len(filtered) != 1 {
		t.Errorf("filtered = %d, want 1", len(filtered))
	}
}

func TestLoadIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	content := `# Comment line
UNUSED_INDEX app.users.idx_old

* app.audit_logs
MISSING_TTL app.settings
`
	if err := os.WriteFile(filepath.Join(dir, ".mongospectreignore"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	il, err := LoadIgnoreFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(il.Rules) != 3 {
		t.Fatalf("got %d rules, want 3", len(il.Rules))
	}
}

func TestLoadIgnoreFile_Missing(t *testing.T) {
	il, err := LoadIgnoreFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(il.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(il.Rules))
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, value string
		want           bool
	}{
		{"*", "anything", true},
		{"app", "app", true},
		{"app", "other", false},
		{"app*", "app_staging", true},
		{"app*", "other", false},
		{"user*", "users", true},
	}
	for _, tt := range tests {
		got := matchGlob(tt.pattern, tt.value)
		if got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
		}
	}
}
