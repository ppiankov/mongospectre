package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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
