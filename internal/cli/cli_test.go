package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionCommand(t *testing.T) {
	cmd := newRootCmd("1.2.3")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "mongospectre 1.2.3") {
		t.Errorf("version output = %q", out.String())
	}
}

func TestRootHelp(t *testing.T) {
	cmd := newRootCmd("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, sub := range []string{"audit", "check", "compare", "version"} {
		if !strings.Contains(help, sub) {
			t.Errorf("help missing subcommand %q", sub)
		}
	}
}

func silentCmd(args ...string) *cobra.Command {
	cmd := newRootCmd("test")
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
	cmd := newRootCmd("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"audit", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	if !strings.Contains(help, "sarif") {
		t.Error("audit --help should mention sarif format")
	}
}

func TestCheckFormatFlag(t *testing.T) {
	cmd := newRootCmd("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"check", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	if !strings.Contains(help, "sarif") {
		t.Error("check --help should mention sarif format")
	}
}

func TestAuditHelpFlags(t *testing.T) {
	cmd := newRootCmd("test")
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
	cmd := newRootCmd("test")
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
	cmd := newRootCmd("test")
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

func TestExecute_Version(t *testing.T) {
	// Execute runs the root command with the given version.
	// We can't fully test without capturing stdout, but we can
	// verify it doesn't panic.
	err := Execute("test-version")
	// Execute with no args just shows help, which isn't an error.
	if err != nil {
		t.Errorf("Execute returned unexpected error: %v", err)
	}
}

func TestRootPersistentFlags(t *testing.T) {
	cmd := newRootCmd("test")
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
