package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/ppiankov/mongospectre/internal/scanner"
	"github.com/spf13/cobra"
)

func stubInteractiveHooks(
	t *testing.T,
	tty bool,
	supported bool,
	launchFn func(*reporter.Report, []mongoinspect.CollectionInfo, *scanner.ScanResult) error,
) {
	t.Helper()
	prevTTY := commandHasTTY
	prevSupported := terminalSupportsInteractive
	prevLaunch := launchInteractiveUI
	commandHasTTY = func(*cobra.Command) bool { return tty }
	terminalSupportsInteractive = func() bool { return supported }
	launchInteractiveUI = launchFn
	t.Cleanup(func() {
		commandHasTTY = prevTTY
		terminalSupportsInteractive = prevSupported
		launchInteractiveUI = prevLaunch
	})
}

func TestDecideInteractive(t *testing.T) {
	tests := []struct {
		name      string
		cfg       interactiveConfig
		tty       bool
		supported bool
		wantRun   bool
		wantPart  string
	}{
		{
			name: "force interactive",
			cfg: interactiveConfig{
				force:    true,
				format:   "text",
				findings: 0,
			},
			tty:       true,
			supported: true,
			wantRun:   true,
		},
		{
			name: "auto threshold",
			cfg: interactiveConfig{
				format:   "text",
				findings: 21,
			},
			tty:       true,
			supported: true,
			wantRun:   true,
		},
		{
			name: "no tty fallback",
			cfg: interactiveConfig{
				force:    true,
				format:   "text",
				findings: 100,
			},
			tty:       false,
			supported: true,
			wantRun:   false,
			wantPart:  "not a terminal",
		},
		{
			name: "json format",
			cfg: interactiveConfig{
				force:    true,
				format:   "json",
				findings: 100,
			},
			tty:       true,
			supported: true,
			wantRun:   false,
			wantPart:  "--interactive requires --format text",
		},
		{
			name: "explicitly disabled",
			cfg: interactiveConfig{
				force:    true,
				disable:  true,
				format:   "text",
				findings: 100,
			},
			tty:       true,
			supported: true,
			wantRun:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decideInteractive(tc.cfg, tc.tty, tc.supported)
			if got.run != tc.wantRun {
				t.Fatalf("run = %v, want %v", got.run, tc.wantRun)
			}
			if tc.wantPart != "" && !strings.Contains(got.reason, tc.wantPart) {
				t.Fatalf("reason = %q, want to contain %q", got.reason, tc.wantPart)
			}
		})
	}
}

func TestAuditInteractiveFlagLaunchesTUI(t *testing.T) {
	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{
				Database: "app",
				Name:     "users",
				DocCount: 10,
				Indexes:  []mongoinspect.IndexInfo{{Name: "_id_"}},
			},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	var launched bool
	stubInteractiveHooks(t, true, true, func(_ *reporter.Report, _ []mongoinspect.CollectionInfo, _ *scanner.ScanResult) error {
		launched = true
		return nil
	})

	stdout, _, err := execCLI(t, "audit", "--uri", "mongodb://stub", "-i", "--timeout", "1s")
	if err != nil {
		t.Fatalf("audit returned error: %v", err)
	}
	if !launched {
		t.Fatal("expected interactive UI to be launched")
	}
	if stdout != "" {
		t.Fatalf("expected no text report when TUI launched, got: %q", stdout)
	}
}

func TestAuditInteractiveFallsBackWhenNonTTY(t *testing.T) {
	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{
				Database: "app",
				Name:     "users",
				DocCount: 10,
				Indexes:  []mongoinspect.IndexInfo{{Name: "_id_"}},
			},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	launched := false
	stubInteractiveHooks(t, false, true, func(_ *reporter.Report, _ []mongoinspect.CollectionInfo, _ *scanner.ScanResult) error {
		launched = true
		return nil
	})

	stdout, stderr, err := execCLI(t, "audit", "--uri", "mongodb://stub", "-i", "--timeout", "1s")
	if err != nil {
		t.Fatalf("audit returned error: %v", err)
	}
	if launched {
		t.Fatal("did not expect interactive UI launch in non-TTY mode")
	}
	if !strings.Contains(stderr, "interactive mode skipped") {
		t.Fatalf("expected skip warning in stderr, got: %q", stderr)
	}
	if !strings.Contains(stdout, "No findings.") {
		t.Fatalf("expected text fallback output, got: %q", stdout)
	}
}

func TestAuditAutoInteractiveForLargeFindingSet(t *testing.T) {
	collections := make([]mongoinspect.CollectionInfo, 0, 21)
	for i := 0; i < 21; i++ {
		collections = append(collections, mongoinspect.CollectionInfo{
			Database: "app",
			Name:     fmt.Sprintf("c%02d", i),
			DocCount: 0,
			Indexes:  []mongoinspect.IndexInfo{{Name: "_id_"}},
		})
	}
	fake := &fakeInspector{
		serverInfo:    mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: collections,
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	launched := false
	stubInteractiveHooks(t, true, true, func(_ *reporter.Report, _ []mongoinspect.CollectionInfo, _ *scanner.ScanResult) error {
		launched = true
		return nil
	})

	_, _, err := execCLI(t, "audit", "--uri", "mongodb://stub", "--timeout", "1s")
	requireExitCode(t, err, 1)
	if !launched {
		t.Fatal("expected auto interactive UI launch for >20 findings")
	}
}

func TestCheckInteractiveLaunchReceivesScan(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{
			Collections: []string{"users"},
			Refs: []scanner.CollectionRef{
				{Collection: "users", File: "repo/main.go", Line: 10, Pattern: scanner.PatternDriverCall},
			},
			FilesScanned: 1,
		}, nil
	})

	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{
				Database: "app",
				Name:     "users",
				DocCount: 10,
				Indexes:  []mongoinspect.IndexInfo{{Name: "_id_"}},
			},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	var scanSeen *scanner.ScanResult
	stubInteractiveHooks(t, true, true, func(_ *reporter.Report, _ []mongoinspect.CollectionInfo, scan *scanner.ScanResult) error {
		scanSeen = scan
		return nil
	})

	_, _, err := execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "-i", "--timeout", "1s")
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	if scanSeen == nil {
		t.Fatal("expected scan result to be passed to interactive UI")
	}
	if len(scanSeen.Refs) != 1 {
		t.Fatalf("scan refs = %d, want 1", len(scanSeen.Refs))
	}
}

func TestInteractiveFlagConflict(t *testing.T) {
	_, _, err := execCLI(t, "audit", "--uri", "mongodb://stub", "-i", "--no-interactive")
	if err == nil {
		t.Fatal("expected interactive flag conflict error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _, err = execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "-i", "--no-interactive")
	if err == nil {
		t.Fatal("expected interactive flag conflict error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}
