package cli

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

func TestCheckDatabaseScopeAndExitCodeZero(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{
			Collections:  []string{"users"},
			Refs:         []scanner.CollectionRef{{Collection: "users"}},
			FilesScanned: 1,
		}, nil
	})

	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "users", DocCount: 25, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}

	var gotCfg mongoinspect.Config
	stubNewInspector(t, func(_ context.Context, cfg mongoinspect.Config) (inspector, error) {
		gotCfg = cfg
		return fake, nil
	})

	stdout, _, err := execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "--database", "app", "--format", "json", "--timeout", "1s")
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}

	if gotCfg.Database != "app" {
		t.Fatalf("inspector config database = %q, want app", gotCfg.Database)
	}
	if len(fake.inspectCalls) != 1 || fake.inspectCalls[0] != "app" {
		t.Fatalf("Inspect called with %v, want [app]", fake.inspectCalls)
	}

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid report JSON: %v", err)
	}
	if report.Metadata.Database != "app" {
		t.Fatalf("report database = %q, want app", report.Metadata.Database)
	}
}

func TestCheckJSONIncludesScanAndCollections(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{
			RepoPath:    "repo",
			Collections: []string{"users"},
			Refs: []scanner.CollectionRef{
				{Collection: "users", File: "main.go", Line: 10, Pattern: scanner.PatternDriverCall},
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
				DocCount: 25,
				Indexes:  []mongoinspect.IndexInfo{{Name: "_id_"}},
			},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	stdout, _, err := execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "--database", "app", "--format", "json", "--timeout", "1s")
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid report JSON: %v", err)
	}
	if report.Scan == nil {
		t.Fatal("expected scan payload in JSON report")
	}
	if len(report.Scan.Refs) != 1 || report.Scan.Refs[0].File != "main.go" || report.Scan.Refs[0].Line != 10 {
		t.Fatalf("unexpected scan refs: %+v", report.Scan.Refs)
	}
	if len(report.Collections) != 1 || report.Collections[0].Name != "users" || report.Collections[0].Database != "app" {
		t.Fatalf("unexpected collections payload: %+v", report.Collections)
	}
}

func TestCheckScanError(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{}, errors.New("boom")
	})

	_, _, err := execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "--timeout", "1s")
	if err == nil {
		t.Fatal("expected scan error")
	}
	if !strings.Contains(err.Error(), "scan repo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckExitCodes(t *testing.T) {
	tests := []struct {
		name        string
		scan        scanner.ScanResult
		collections []mongoinspect.CollectionInfo
		wantCode    int
	}{
		{
			name: "medium severity",
			scan: scanner.ScanResult{},
			collections: []mongoinspect.CollectionInfo{
				{Database: "app", Name: "empty", DocCount: 0, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
			},
			wantCode: 1,
		},
		{
			name: "high severity",
			scan: scanner.ScanResult{
				Collections: []string{"missing_collection"},
				Refs:        []scanner.CollectionRef{{Collection: "missing_collection"}},
			},
			collections: nil,
			wantCode:    2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stubScanRepo(t, func(string) (scanner.ScanResult, error) {
				return tc.scan, nil
			})
			fake := &fakeInspector{
				serverInfo:    mongoinspect.ServerInfo{Version: "7.0.0"},
				inspectResult: tc.collections,
			}
			stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
				return fake, nil
			})

			_, stderr, err := execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "--timeout", "1s")
			requireExitCode(t, err, tc.wantCode)
			if !strings.Contains(stderr, "Exit ") {
				t.Fatalf("expected exit code hint in stderr, got: %q", stderr)
			}
		})
	}
}

func TestCheckFailOnMissingReturnsExitTwo(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{
			Collections: []string{"missing_collection"},
			Refs:        []scanner.CollectionRef{{Collection: "missing_collection"}},
		}, nil
	})
	fake := &fakeInspector{
		serverInfo:    mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: nil,
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	_, stderr, err := execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "--fail-on-missing", "--timeout", "1s")
	requireExitCode(t, err, 2)
	if strings.Contains(stderr, "Exit 2:") {
		t.Fatalf("did not expect generic exit hint for fail-on-missing path, got: %q", stderr)
	}
}

func TestCheckBaselineLoadError(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{}, nil
	})
	fake := &fakeInspector{
		serverInfo:    mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: nil,
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	missingBaseline := filepath.Join(t.TempDir(), "missing-baseline.json")
	_, _, err := execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "--baseline", missingBaseline, "--timeout", "1s")
	if err == nil {
		t.Fatal("expected baseline load error")
	}
	if !strings.Contains(err.Error(), "load baseline") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckEmptyCollectionsHint(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{}, nil
	})
	fake := &fakeInspector{
		serverInfo:    mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: nil,
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	_, stderr, err := execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "--timeout", "1s")
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	if !strings.Contains(stderr, "Hint: no collections found") {
		t.Fatalf("expected empty-collections hint, got: %q", stderr)
	}
}

func TestCheckVerboseScannerOutput(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{
			Collections:  []string{"users"},
			Refs:         []scanner.CollectionRef{{Collection: "users"}},
			FilesScanned: 2,
			FilesSkipped: 1,
		}, nil
	})
	fake := &fakeInspector{
		serverInfo:    mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{{Database: "app", Name: "users", DocCount: 10, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}}},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	_, stderr, err := execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "--verbose", "--timeout", "1s")
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	if !strings.Contains(stderr, "collection: users") {
		t.Fatalf("expected verbose collection listing, got: %q", stderr)
	}
	if !strings.Contains(stderr, "skipped 1 unreadable files") {
		t.Fatalf("expected verbose skipped-files line, got: %q", stderr)
	}
}

func TestCheckReportsValidatorDrift(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{
			Collections: []string{"users"},
			Refs:        []scanner.CollectionRef{{Collection: "users"}},
			WriteRefs: []scanner.WriteRef{
				{Collection: "users", Field: "email", ValueType: scanner.ValueTypeString},
			},
			FilesScanned: 1,
		}, nil
	})
	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "users", DocCount: 25, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	stdout, _, err := execCLI(t, "check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "--format", "json", "--timeout", "1s")
	requireExitCode(t, err, 1)

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid report JSON: %v", err)
	}

	for _, f := range report.Findings {
		if f.Type == analyzer.FindingValidatorMissing {
			return
		}
	}
	t.Fatal("expected VALIDATOR_MISSING in check report")
}

func TestCheckProfileCorrelatesSlowQueries(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{
			Collections: []string{"users"},
			Refs: []scanner.CollectionRef{
				{Collection: "users", File: "app/models/user.go", Line: 15},
			},
			FieldRefs: []scanner.FieldRef{
				{Collection: "users", Field: "status", File: "app/models/user.go", Line: 15},
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
				DocCount: 25,
				Indexes:  []mongoinspect.IndexInfo{{Name: "_id_"}, {Name: "status_1", Key: []mongoinspect.KeyField{{Field: "status", Direction: 1}}}},
			},
		},
		profilerRes: []mongoinspect.ProfileEntry{
			{
				Database:       "app",
				Collection:     "users",
				FilterFields:   []string{"status"},
				DurationMillis: 850,
				PlanSummary:    "COLLSCAN",
			},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	stdout, _, err := execCLI(
		t,
		"check",
		"--uri", "mongodb://stub",
		"--repo", t.TempDir(),
		"--database", "app",
		"--profile",
		"--profile-limit", "77",
		"--format", "json",
		"--timeout", "1s",
	)
	requireExitCode(t, err, 2)

	if len(fake.profilerCalls) != 1 {
		t.Fatalf("expected one profiler call, got %d", len(fake.profilerCalls))
	}
	if fake.profilerCalls[0].database != "app" || fake.profilerCalls[0].limit != 77 {
		t.Fatalf("unexpected profiler call: %+v", fake.profilerCalls[0])
	}

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid report JSON: %v", err)
	}

	var slowFound, collscanFound bool
	for _, finding := range report.Findings {
		switch finding.Type {
		case analyzer.FindingSlowQuerySource:
			slowFound = strings.Contains(finding.Message, "app/models/user.go:15")
		case analyzer.FindingCollectionScanSource:
			collscanFound = strings.Contains(finding.Message, "app/models/user.go:15")
		}
	}
	if !slowFound {
		t.Fatal("expected SLOW_QUERY_SOURCE finding linked to source location")
	}
	if !collscanFound {
		t.Fatal("expected COLLECTION_SCAN_SOURCE finding linked to source location")
	}
}

func TestCheckProfileGracefulSkipWhenDisabledOrEmpty(t *testing.T) {
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{
			Collections:  []string{"users"},
			Refs:         []scanner.CollectionRef{{Collection: "users", File: "app/main.go", Line: 10}},
			FilesScanned: 1,
		}, nil
	})
	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "users", DocCount: 25, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
		profilerRes: nil,
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	_, stderr, err := execCLI(
		t,
		"check",
		"--uri", "mongodb://stub",
		"--repo", t.TempDir(),
		"--database", "app",
		"--profile",
		"--format", "json",
		"--timeout", "1s",
	)
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	if !strings.Contains(stderr, "no profiler entries found in system.profile") {
		t.Fatalf("expected profiler skip hint, got: %q", stderr)
	}
	if len(fake.profilerCalls) != 1 {
		t.Fatalf("expected one profiler call, got %d", len(fake.profilerCalls))
	}
}

func TestCheckProfileLimitValidation(t *testing.T) {
	_, _, err := execCLI(
		t,
		"check",
		"--uri", "mongodb://stub",
		"--repo", t.TempDir(),
		"--profile-limit", "0",
		"--timeout", "1s",
	)
	if err == nil {
		t.Fatal("expected profile-limit validation error")
	}
	if !strings.Contains(err.Error(), "--profile-limit must be greater than 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}
