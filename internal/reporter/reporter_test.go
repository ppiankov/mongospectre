package reporter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ppiankov/mongospectre/internal/analyzer"
)

var testFindings = []analyzer.Finding{
	{
		Type:       analyzer.FindingUnusedIndex,
		Severity:   analyzer.SeverityMedium,
		Database:   "app",
		Collection: "users",
		Index:      "old_idx",
		Message:    `index "old_idx" has never been used`,
	},
	{
		Type:       analyzer.FindingMissingIndex,
		Severity:   analyzer.SeverityHigh,
		Database:   "app",
		Collection: "orders",
		Message:    "collection has 100000 documents but only the _id index",
	},
}

func TestNewReport_Summary(t *testing.T) {
	r := NewReport(testFindings)
	if r.Summary.Total != 2 {
		t.Errorf("total = %d, want 2", r.Summary.Total)
	}
	if r.Summary.High != 1 {
		t.Errorf("high = %d, want 1", r.Summary.High)
	}
	if r.Summary.Medium != 1 {
		t.Errorf("medium = %d, want 1", r.Summary.Medium)
	}
	if r.MaxSeverity != analyzer.SeverityHigh {
		t.Errorf("maxSeverity = %s, want high", r.MaxSeverity)
	}
}

func TestNewReport_Empty(t *testing.T) {
	r := NewReport(nil)
	if r.Summary.Total != 0 {
		t.Errorf("total = %d, want 0", r.Summary.Total)
	}
	if r.MaxSeverity != analyzer.SeverityInfo {
		t.Errorf("maxSeverity = %s, want info", r.MaxSeverity)
	}
}

func TestWriteText(t *testing.T) {
	r := NewReport(testFindings)
	var buf bytes.Buffer
	if err := Write(&buf, r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "[HIGH]") {
		t.Error("missing [HIGH] label")
	}
	if !strings.Contains(out, "[MEDIUM]") {
		t.Error("missing [MEDIUM] label")
	}
	if !strings.Contains(out, "app.users.old_idx") {
		t.Error("missing collection.index location")
	}
	if !strings.Contains(out, "Summary:") {
		t.Error("missing summary line")
	}
}

func TestWriteText_Empty(t *testing.T) {
	r := NewReport(nil)
	var buf bytes.Buffer
	if err := Write(&buf, r, FormatText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No findings.") {
		t.Error("expected 'No findings.' for empty report")
	}
}

func TestWriteJSON(t *testing.T) {
	r := NewReport(testFindings)
	var buf bytes.Buffer
	if err := Write(&buf, r, FormatJSON); err != nil {
		t.Fatal(err)
	}

	var decoded Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(decoded.Findings) != 2 {
		t.Errorf("findings count = %d, want 2", len(decoded.Findings))
	}
	if decoded.MaxSeverity != analyzer.SeverityHigh {
		t.Errorf("maxSeverity = %s, want high", decoded.MaxSeverity)
	}
}
