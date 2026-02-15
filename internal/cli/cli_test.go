package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	"github.com/spf13/cobra"
)

var testBuildInfo = BuildInfo{
	Version:   "1.2.3",
	Commit:    "abc1234",
	Date:      "2026-02-15T00:00:00Z",
	GoVersion: "go1.25.0",
}

func TestVersionCommand(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := out.String()
	if !strings.Contains(output, "mongospectre 1.2.3") {
		t.Errorf("version output = %q", output)
	}
	if !strings.Contains(output, "abc1234") {
		t.Errorf("missing commit in version output: %q", output)
	}
	if !strings.Contains(output, "2026-02-15") {
		t.Errorf("missing date in version output: %q", output)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var info BuildInfo
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if info.Version != "1.2.3" {
		t.Errorf("version = %s, want 1.2.3", info.Version)
	}
	if info.Commit != "abc1234" {
		t.Errorf("commit = %s, want abc1234", info.Commit)
	}
}

func TestRootHelp(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, sub := range []string{"audit", "check", "compare", "version", "watch"} {
		if !strings.Contains(help, sub) {
			t.Errorf("help missing subcommand %q", sub)
		}
	}
}

func silentCmd(args ...string) *cobra.Command {
	cmd := newRootCmd(testBuildInfo)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd
}

func TestAuditMissingURI(t *testing.T) {
	uri = ""
	cmd := silentCmd("audit")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing URI")
	}
	if !strings.Contains(err.Error(), "--uri is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckMissingURI(t *testing.T) {
	uri = ""
	cmd := silentCmd("check", "--repo", "/tmp")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing URI")
	}
	if !strings.Contains(err.Error(), "--uri is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckMissingRepo(t *testing.T) {
	uri = "mongodb://localhost"
	cmd := silentCmd("check", "--uri", "mongodb://localhost")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
	if !strings.Contains(err.Error(), "--repo is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCompareMissingSource(t *testing.T) {
	cmd := silentCmd("compare", "--target", "mongodb://target")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if !strings.Contains(err.Error(), "--source is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCompareMissingTarget(t *testing.T) {
	cmd := silentCmd("compare", "--source", "mongodb://source")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing target")
	}
	if !strings.Contains(err.Error(), "--target is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWatchMissingURI(t *testing.T) {
	uri = ""
	cmd := silentCmd("watch")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing URI")
	}
	if !strings.Contains(err.Error(), "--uri is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAuditConnectionError(t *testing.T) {
	uri = ""
	cmd := silentCmd("audit", "--uri", "mongodb://localhost:1", "--timeout", "500ms")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connect") && !strings.Contains(err.Error(), "ping") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckConnectionError(t *testing.T) {
	dir := t.TempDir()
	uri = ""
	cmd := silentCmd("check", "--uri", "mongodb://localhost:1", "--repo", dir, "--timeout", "500ms")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestCompareConnectionError(t *testing.T) {
	uri = ""
	cmd := silentCmd("compare", "--source", "mongodb://localhost:1", "--target", "mongodb://localhost:2", "--timeout", "500ms")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestAuditFormatFlag(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"audit", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, f := range []string{"sarif", "spectrehub"} {
		if !strings.Contains(help, f) {
			t.Errorf("audit --help should mention %s format", f)
		}
	}
}

func TestCheckFormatFlag(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"check", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, f := range []string{"sarif", "spectrehub"} {
		if !strings.Contains(help, f) {
			t.Errorf("check --help should mention %s format", f)
		}
	}
}

func TestAuditHelpFlags(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"audit", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, flag := range []string{"--database", "--format", "--no-ignore", "--baseline"} {
		if !strings.Contains(help, flag) {
			t.Errorf("audit --help missing %s", flag)
		}
	}
}

func TestCheckHelpFlags(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"check", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, flag := range []string{"--repo", "--database", "--format", "--fail-on-missing", "--no-ignore", "--baseline"} {
		if !strings.Contains(help, flag) {
			t.Errorf("check --help missing %s", flag)
		}
	}
}

func TestCompareHelpFlags(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"compare", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, flag := range []string{"--source", "--target", "--source-db", "--target-db", "--format"} {
		if !strings.Contains(help, flag) {
			t.Errorf("compare --help missing %s", flag)
		}
	}
}

func TestWatchHelpFlags(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"watch", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, flag := range []string{"--interval", "--format", "--exit-on-new", "--no-ignore"} {
		if !strings.Contains(help, flag) {
			t.Errorf("watch --help missing %s", flag)
		}
	}
}

func TestExecute_Version(t *testing.T) {
	err := Execute("test-version", "none", "unknown")
	if err != nil {
		t.Errorf("Execute returned unexpected error: %v", err)
	}
}

func TestRootPersistentFlags(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, flag := range []string{"--uri", "--verbose", "--timeout"} {
		if !strings.Contains(help, flag) {
			t.Errorf("root --help missing %s", flag)
		}
	}
}

func TestWriteCompareTextEmpty(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	writeCompareText(cmd, nil)
	if !strings.Contains(out.String(), "No differences found") {
		t.Errorf("expected 'No differences found', got %q", out.String())
	}
}

func TestWriteCompareTextWithFindings(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	findings := []analyzer.CompareFinding{
		{
			Type:       "MISSING_IN_TARGET",
			Database:   "testdb",
			Collection: "users",
			Severity:   analyzer.SeverityHigh,
			Message:    "collection missing in target",
		},
		{
			Type:       "INDEX_DRIFT",
			Database:   "testdb",
			Collection: "orders",
			Index:      "status_1",
			Severity:   analyzer.SeverityMedium,
			Message:    "index exists in source but not target",
		},
		{
			Type:       "EXTRA_COLLECTION",
			Database:   "testdb",
			Collection: "temp",
			Severity:   analyzer.SeverityLow,
			Message:    "extra collection in target",
		},
		{
			Type:       "SCHEMA_INFO",
			Database:   "testdb",
			Collection: "logs",
			Severity:   analyzer.SeverityInfo,
			Message:    "informational note",
		},
	}
	writeCompareText(cmd, findings)

	output := out.String()
	if !strings.Contains(output, "[HIGH]") {
		t.Error("missing HIGH severity label")
	}
	if !strings.Contains(output, "[MEDIUM]") {
		t.Error("missing MEDIUM severity label")
	}
	if !strings.Contains(output, "[LOW]") {
		t.Error("missing LOW severity label")
	}
	if !strings.Contains(output, "[INFO]") {
		t.Error("missing INFO severity label")
	}
	if !strings.Contains(output, "testdb.orders.status_1") {
		t.Error("missing index in location")
	}
	if !strings.Contains(output, "4 differences found") {
		t.Error("missing count")
	}
}

func TestEmitJSON(t *testing.T) {
	var out bytes.Buffer
	w := &watcher{cmd: &cobra.Command{}}
	w.cmd.SetOut(&out)

	event := &watchEvent{
		Timestamp: "2026-01-01T00:00:00Z",
		Type:      "full",
		Summary:   watchSummary{Total: 5, New: 2, Resolved: 1},
	}
	w.emitJSON(&out, event)

	var parsed watchEvent
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.Type != "full" {
		t.Errorf("type = %s, want full", parsed.Type)
	}
	if parsed.Summary.Total != 5 {
		t.Errorf("total = %d, want 5", parsed.Summary.Total)
	}
}

func TestCheckScansThenFailsConnect(t *testing.T) {
	// check command scans repo first, then fails at MongoDB connect.
	// This exercises the scanner path in check.go.
	dir := t.TempDir()
	// Write a Go file with a collection reference so scanner finds something.
	goFile := dir + "/main.go"
	os.WriteFile(goFile, []byte(`package main
import "go.mongodb.org/mongo-driver/v2/mongo"
func f(db *mongo.Database) { db.Collection("users") }
`), 0o644)

	uri = ""
	cmd := silentCmd("check", "--uri", "mongodb://localhost:1", "--repo", dir, "--timeout", "500ms")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection error after scanning repo")
	}
	// The error should be from connect, not from scanning — means scanner ran.
	if !strings.Contains(err.Error(), "connect") && !strings.Contains(err.Error(), "ping") {
		t.Errorf("expected connect error, got: %v", err)
	}
}

func TestCheckScansThenFailsConnectVerbose(t *testing.T) {
	dir := t.TempDir()
	goFile := dir + "/main.go"
	os.WriteFile(goFile, []byte(`package main
import "go.mongodb.org/mongo-driver/v2/mongo"
func f(db *mongo.Database) { db.Collection("orders") }
`), 0o644)

	uri = ""
	verbose = true
	defer func() { verbose = false }()
	cmd := silentCmd("check", "--uri", "mongodb://localhost:1", "--repo", dir, "--timeout", "500ms", "--verbose")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestAuditVerboseConnectionError(t *testing.T) {
	uri = ""
	verbose = true
	defer func() { verbose = false }()
	cmd := silentCmd("audit", "--uri", "mongodb://localhost:1", "--timeout", "500ms", "--verbose")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestCompareVerboseConnectionError(t *testing.T) {
	uri = ""
	verbose = true
	defer func() { verbose = false }()
	cmd := silentCmd("compare", "--source", "mongodb://localhost:1", "--target", "mongodb://localhost:2", "--timeout", "500ms", "--verbose")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestWatchConnectionError(t *testing.T) {
	// Watch retries on connection errors, so we need to cancel context.
	uri = ""
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := silentCmd("watch", "--uri", "mongodb://localhost:1", "--timeout", "500ms", "--interval", "100ms")
	cmd.SetContext(ctx)
	// Watch exits cleanly on context cancel (returns nil), so we just verify
	// it doesn't panic and runs the watcher path.
	_ = cmd.Execute()
}

func TestRootConfigLoading(t *testing.T) {
	// PersistentPreRunE loads config. Run audit (which will fail at URI check)
	// but the config loading path should execute.
	uri = ""
	cmd := silentCmd("audit")
	_ = cmd.Execute()
	// Just verify it didn't panic — config loading ran.
}

func TestRootEnvURI(t *testing.T) {
	// Set MONGODB_URI env, verify it's picked up.
	uri = ""
	t.Setenv("MONGODB_URI", "mongodb://env-uri:27017")
	cmd := silentCmd("audit", "--timeout", "500ms")
	err := cmd.Execute()
	// Should fail on connect, not on "uri required", proving env was picked up.
	if err != nil && strings.Contains(err.Error(), "--uri is required") {
		t.Error("env MONGODB_URI should have been picked up")
	}
}

func TestWatchConnectionErrorJSON(t *testing.T) {
	// Watch in JSON mode: exercises the shutdown JSON event emission path.
	uri = ""
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := silentCmd("watch", "--uri", "mongodb://localhost:1", "--timeout", "500ms", "--interval", "100ms", "--format", "json")
	cmd.SetContext(ctx)
	var out bytes.Buffer
	cmd.SetOut(&out)
	_ = cmd.Execute()
	// Shutdown event should be emitted as JSON.
	output := out.String()
	if output == "" {
		t.Error("expected JSON shutdown event")
	}
	if output != "" && !strings.Contains(output, "shutdown") {
		t.Errorf("expected shutdown event, got %q", output)
	}
}

func TestWatchRunTextShutdownSummary(t *testing.T) {
	// Exercises the watcher.run() shutdown summary path with text format.
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	w := &watcher{
		uri:      "mongodb://localhost:1",
		interval: 100 * time.Millisecond,
		format:   "text",
		cmd:      cmd,
	}
	timeout = 500 * time.Millisecond
	_ = w.run(ctx)
	errOut := stderr.String()
	if !strings.Contains(errOut, "Watch mode: auditing every") {
		t.Errorf("missing watch mode message in stderr: %q", errOut)
	}
	if !strings.Contains(errOut, "Watch summary:") {
		t.Errorf("missing shutdown summary in stderr: %q", errOut)
	}
}

func TestWatchRunJSONShutdownEvent(t *testing.T) {
	// Exercises the watcher.run() JSON format shutdown path directly.
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	w := &watcher{
		uri:      "mongodb://localhost:1",
		interval: 100 * time.Millisecond,
		format:   "json",
		cmd:      cmd,
	}
	timeout = 500 * time.Millisecond
	_ = w.run(ctx)
	output := stdout.String()
	if !strings.Contains(output, `"type":"shutdown"`) {
		t.Errorf("expected JSON shutdown event, got %q", output)
	}
}

func TestWatchRunNoIgnore(t *testing.T) {
	// Exercises the noIgnore flag path in runAudit.
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	w := &watcher{
		uri:      "mongodb://localhost:1",
		interval: 100 * time.Millisecond,
		format:   "text",
		noIgnore: true,
		cmd:      cmd,
	}
	timeout = 500 * time.Millisecond
	_ = w.run(ctx)
}

func TestConfigFileDefaults(t *testing.T) {
	// Create a .mongospectre.yml with verbose and timeout defaults
	// to exercise the config fallback paths in PersistentPreRunE.
	dir := t.TempDir()
	cfgFile := dir + "/.mongospectre.yml"
	os.WriteFile(cfgFile, []byte("uri: mongodb://config-uri:27017\ndefaults:\n  verbose: true\n  timeout: 10s\n"), 0o644)

	// Save and restore cwd since config.Load uses it.
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	uri = ""
	verbose = false
	cmd := silentCmd("audit", "--timeout", "500ms")
	err := cmd.Execute()
	// Config URI should be picked up — error should be connect, not "uri required".
	if err != nil && strings.Contains(err.Error(), "--uri is required") {
		t.Error("config file URI should have been picked up")
	}
}

func TestConfigFileVerboseDefault(t *testing.T) {
	// Config sets verbose: true, verify it's applied when flag not set.
	dir := t.TempDir()
	cfgFile := dir + "/.mongospectre.yml"
	os.WriteFile(cfgFile, []byte("defaults:\n  verbose: true\n"), 0o644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	uri = ""
	verbose = false
	cmd := silentCmd("audit")
	_ = cmd.Execute()
	// After PersistentPreRunE, verbose should be true from config.
	if !verbose {
		t.Error("config verbose default should have been applied")
	}
	verbose = false
}

func TestConfigFileTimeoutDefault(t *testing.T) {
	// Config sets timeout, verify it's applied when flag not set.
	dir := t.TempDir()
	cfgFile := dir + "/.mongospectre.yml"
	os.WriteFile(cfgFile, []byte("defaults:\n  timeout: 45s\n"), 0o644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	uri = ""
	cmd := silentCmd("audit")
	_ = cmd.Execute()
	if timeout != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", timeout)
	}
}

func TestWatchExitOnNewFlag(t *testing.T) {
	// Exercises the exitOnNew watcher field (connection will fail before it matters,
	// but the field is initialized in the watcher struct).
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	w := &watcher{
		uri:       "mongodb://localhost:1",
		interval:  100 * time.Millisecond,
		format:    "text",
		exitOnNew: true,
		cmd:       cmd,
	}
	timeout = 500 * time.Millisecond
	_ = w.run(ctx)
}

func TestWatchVerboseConnectionError(t *testing.T) {
	// Watch with verbose in text mode: exercises verbose error logging.
	uri = ""
	verbose = true
	defer func() { verbose = false }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := silentCmd("watch", "--uri", "mongodb://localhost:1", "--timeout", "500ms", "--interval", "100ms", "--verbose")
	cmd.SetContext(ctx)
	_ = cmd.Execute()
}

func TestWatchDatabaseFlag(t *testing.T) {
	// Exercises the database field in the watcher.
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	w := &watcher{
		uri:      "mongodb://localhost:1",
		database: "testdb",
		interval: 100 * time.Millisecond,
		format:   "text",
		cmd:      cmd,
	}
	timeout = 500 * time.Millisecond
	_ = w.run(ctx)
}
