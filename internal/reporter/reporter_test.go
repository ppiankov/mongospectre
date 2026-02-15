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
	r.Metadata.Version = "1.0.0"
	r.Metadata.Command = "audit"
	r.Metadata.MongoDBVersion = "7.0.1"
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "mongospectre 1.0.0 | audit | MongoDB 7.0.1") {
		t.Error("missing header line")
	}
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
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No findings.") {
		t.Error("expected 'No findings.' for empty report")
	}
}

func TestNewReport_AllSeverities(t *testing.T) {
	findings := []analyzer.Finding{
		{Severity: analyzer.SeverityHigh},
		{Severity: analyzer.SeverityMedium},
		{Severity: analyzer.SeverityLow},
		{Severity: analyzer.SeverityInfo},
	}
	r := NewReport(findings)
	if r.Summary.Total != 4 {
		t.Errorf("total = %d, want 4", r.Summary.Total)
	}
	if r.Summary.High != 1 || r.Summary.Medium != 1 || r.Summary.Low != 1 || r.Summary.Info != 1 {
		t.Errorf("summary = %+v, want 1 each", r.Summary)
	}
}

func TestWriteText_AllSeverities(t *testing.T) {
	findings := []analyzer.Finding{
		{Type: analyzer.FindingMissingIndex, Severity: analyzer.SeverityHigh, Database: "db", Collection: "c", Message: "high"},
		{Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityMedium, Database: "db", Collection: "c", Message: "med"},
		{Type: analyzer.FindingMissingTTL, Severity: analyzer.SeverityLow, Database: "db", Collection: "c", Message: "low"},
		{Type: analyzer.FindingOK, Severity: analyzer.SeverityInfo, Database: "db", Collection: "c", Message: "info"},
	}
	r := NewReport(findings)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, label := range []string{"[HIGH]", "[MEDIUM]", "[LOW]", "[INFO]"} {
		if !strings.Contains(out, label) {
			t.Errorf("missing %s in output", label)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	r := NewReport(testFindings)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatJSON); err != nil {
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

func TestWriteSARIF(t *testing.T) {
	r := NewReport(testFindings)
	r.Metadata.Version = "0.2.0"
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatSARIF); err != nil {
		t.Fatal(err)
	}

	var log sarifLog
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}
	if log.Version != "2.1.0" {
		t.Errorf("SARIF version = %s, want 2.1.0", log.Version)
	}
	if len(log.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(log.Runs))
	}
	run := log.Runs[0]
	if run.Tool.Driver.Name != "mongospectre" {
		t.Errorf("tool name = %s", run.Tool.Driver.Name)
	}
	if run.Tool.Driver.Version != "0.2.0" {
		t.Errorf("tool version = %s", run.Tool.Driver.Version)
	}
	if len(run.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(run.Results))
	}

	// Check severity mapping.
	for _, r := range run.Results {
		switch r.RuleID {
		case "UNUSED_INDEX":
			if r.Level != "warning" {
				t.Errorf("UNUSED_INDEX level = %s, want warning", r.Level)
			}
		case "MISSING_INDEX":
			if r.Level != "error" {
				t.Errorf("MISSING_INDEX level = %s, want error", r.Level)
			}
		}
	}
}

func TestWriteSARIF_Empty(t *testing.T) {
	r := NewReport(nil)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatSARIF); err != nil {
		t.Fatal(err)
	}
	var log sarifLog
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}
	if len(log.Runs[0].Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(log.Runs[0].Results))
	}
}

func TestWriteBaselineDiff(t *testing.T) {
	diff := []analyzer.BaselineFinding{
		{Finding: analyzer.Finding{Type: analyzer.FindingMissingIndex, Severity: analyzer.SeverityHigh, Message: "new one"}, Status: analyzer.StatusNew},
		{Finding: analyzer.Finding{Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityMedium, Message: "gone"}, Status: analyzer.StatusResolved},
		{Finding: analyzer.Finding{Type: analyzer.FindingMissingTTL, Severity: analyzer.SeverityLow, Message: "same"}, Status: analyzer.StatusUnchanged},
	}
	var buf bytes.Buffer
	WriteBaselineDiff(&buf, diff)
	out := buf.String()
	if !strings.Contains(out, "+ [new]") {
		t.Error("missing new marker")
	}
	if !strings.Contains(out, "- [resolved]") {
		t.Error("missing resolved marker")
	}
	if !strings.Contains(out, "1 new, 1 resolved, 1 unchanged") {
		t.Error("missing summary line")
	}
}

func TestWriteSpectreHub(t *testing.T) {
	r := NewReport(testFindings)
	r.Metadata.Version = "0.2.0"
	r.Metadata.URIHash = "sha256:abc123"
	r.Metadata.Database = "app"
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatSpectreHub); err != nil {
		t.Fatal(err)
	}

	var env SpectreHubEnvelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if env.Schema != "spectre/v1" {
		t.Errorf("schema = %s, want spectre/v1", env.Schema)
	}
	if env.Tool != "mongospectre" {
		t.Errorf("tool = %s, want mongospectre", env.Tool)
	}
	if env.Version != "0.2.0" {
		t.Errorf("version = %s, want 0.2.0", env.Version)
	}
	if env.Target.Type != "mongodb" {
		t.Errorf("target.type = %s, want mongodb", env.Target.Type)
	}
	if env.Target.URIHash != "sha256:abc123" {
		t.Errorf("target.uri_hash = %s", env.Target.URIHash)
	}
	if len(env.Findings) != 2 {
		t.Fatalf("findings = %d, want 2", len(env.Findings))
	}
	if env.Summary.Total != 2 || env.Summary.High != 1 || env.Summary.Medium != 1 {
		t.Errorf("summary = %+v", env.Summary)
	}
}

func TestWriteSpectreHub_Empty(t *testing.T) {
	r := NewReport(nil)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatSpectreHub); err != nil {
		t.Fatal(err)
	}
	var env SpectreHubEnvelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(env.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(env.Findings))
	}
	// Findings array should be [] not null.
	if !strings.Contains(buf.String(), `"findings": []`) {
		t.Error("findings should be empty array, not null")
	}
}

func TestHashURI(t *testing.T) {
	// Credentials should be stripped.
	h1 := HashURI("mongodb://user:pass@localhost:27017/mydb")
	h2 := HashURI("mongodb://other:secret@localhost:27017/mydb")
	if h1 != h2 {
		t.Errorf("URIs with different credentials should produce same hash\n  h1=%s\n  h2=%s", h1, h2)
	}
	if !strings.HasPrefix(h1, "sha256:") {
		t.Errorf("hash should start with sha256:, got %s", h1)
	}

	// Different hosts should produce different hashes.
	h3 := HashURI("mongodb://localhost:27018/mydb")
	if h1 == h3 {
		t.Error("different hosts should produce different hashes")
	}
}

func TestWriteText_HeaderWithDatabase(t *testing.T) {
	r := NewReport(nil)
	r.Metadata.Command = "audit"
	r.Metadata.Database = "mydb"
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "db=mydb") {
		t.Error("missing database in header")
	}
	if !strings.Contains(out, "No findings.") {
		t.Error("missing 'No findings.' for empty report")
	}
}

func TestWriteText_NoHeader(t *testing.T) {
	// When metadata.Command is empty, no header is printed.
	r := NewReport(nil)
	var buf bytes.Buffer
	if err := Write(&buf, &r, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "mongospectre") {
		t.Error("should not print header when command is empty")
	}
}

func TestExitCodeHint(t *testing.T) {
	if h := ExitCodeHint(0); h != "" {
		t.Errorf("hint for 0 should be empty, got %q", h)
	}
	if h := ExitCodeHint(1); !strings.Contains(h, "medium") {
		t.Errorf("hint for 1 should mention medium, got %q", h)
	}
	if h := ExitCodeHint(2); !strings.Contains(h, "high") {
		t.Errorf("hint for 2 should mention high, got %q", h)
	}
}

func TestSeverityToSARIFLevel(t *testing.T) {
	tests := []struct {
		severity analyzer.Severity
		want     string
	}{
		{analyzer.SeverityHigh, "error"},
		{analyzer.SeverityMedium, "warning"},
		{analyzer.SeverityLow, "note"},
		{analyzer.SeverityInfo, "none"},
	}
	for _, tt := range tests {
		got := severityToSARIFLevel(tt.severity)
		if got != tt.want {
			t.Errorf("severityToSARIFLevel(%s) = %s, want %s", tt.severity, got, tt.want)
		}
	}
}
