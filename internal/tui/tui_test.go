package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

func TestMatchesFilter(t *testing.T) {
	f := analyzer.Finding{
		Type:       analyzer.FindingMissingIndex,
		Severity:   analyzer.SeverityHigh,
		Database:   "app",
		Collection: "users",
		Message:    "collection has 20000 documents but only the _id index",
	}

	tests := []struct {
		query string
		want  bool
	}{
		{query: "", want: true},
		{query: "high users", want: true},
		{query: "missing_index", want: true},
		{query: "orders", want: false},
	}

	for _, tc := range tests {
		if got := matchesFilter(&f, tc.query); got != tc.want {
			t.Fatalf("matchesFilter(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestSortEntriesBySeverity(t *testing.T) {
	entries := []findingEntry{
		{id: 1, finding: analyzer.Finding{Type: analyzer.FindingUnusedCollection, Severity: analyzer.SeverityMedium, Collection: "users"}},
		{id: 2, finding: analyzer.Finding{Type: analyzer.FindingMissingCollection, Severity: analyzer.SeverityHigh, Collection: "orders"}},
		{id: 3, finding: analyzer.Finding{Type: analyzer.FindingSuggestIndex, Severity: analyzer.SeverityInfo, Collection: "audit"}},
	}

	sortEntries(entries, sortBySeverity)
	if entries[0].finding.Severity != analyzer.SeverityHigh {
		t.Fatalf("first severity = %s, want high", entries[0].finding.Severity)
	}
	if entries[2].finding.Severity != analyzer.SeverityInfo {
		t.Fatalf("last severity = %s, want info", entries[2].finding.Severity)
	}
}

func TestRenderDetailIncludesContext(t *testing.T) {
	ttl := int32(3600)
	f := analyzer.Finding{
		Type:       analyzer.FindingUnusedIndex,
		Severity:   analyzer.SeverityMedium,
		Database:   "app",
		Collection: "users",
		Index:      "email_1",
		Message:    "index \"email_1\" has never been used",
	}

	input := Input{
		Report: reporter.NewReport([]analyzer.Finding{f}),
		Collections: []mongoinspect.CollectionInfo{
			{
				Database: "app",
				Name:     "users",
				DocCount: 100,
				Indexes: []mongoinspect.IndexInfo{
					{Name: "_id_"},
					{
						Name: "email_1",
						Key: []mongoinspect.KeyField{
							{Field: "email", Direction: 1},
						},
						TTL: &ttl,
					},
				},
			},
		},
		Scan: &scanner.ScanResult{
			Refs: []scanner.CollectionRef{
				{Collection: "users", File: "api/users.go", Line: 42, Pattern: scanner.PatternDriverCall},
			},
		},
	}

	m := newModel(&input)
	detail := m.renderDetail(&f)

	if !strings.Contains(detail, "Index definition: {email:1}") {
		t.Fatalf("detail missing index definition: %q", detail)
	}
	if !strings.Contains(detail, "api/users.go") {
		t.Fatalf("detail missing code ref file: %q", detail)
	}
	if !strings.Contains(detail, "Suggested Fix") {
		t.Fatalf("detail missing suggested fix section: %q", detail)
	}
}

func TestExportFilteredWritesJSON(t *testing.T) {
	f := analyzer.Finding{
		Type:       analyzer.FindingUnusedCollection,
		Severity:   analyzer.SeverityMedium,
		Collection: "users",
		Message:    "collection has 0 documents",
	}
	input := Input{
		Report: reporter.NewReport([]analyzer.Finding{f}),
	}

	m := newModel(&input)

	tmp := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	path, err := m.exportFiltered()
	if err != nil {
		t.Fatalf("exportFiltered: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, path))
	if err != nil {
		t.Fatalf("read export: %v", err)
	}

	var out reporter.Report
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid exported JSON: %v", err)
	}
	if len(out.Findings) != 1 {
		t.Fatalf("exported findings = %d, want 1", len(out.Findings))
	}
}
