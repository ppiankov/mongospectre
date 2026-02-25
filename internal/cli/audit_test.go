package cli

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	"github.com/ppiankov/mongospectre/internal/atlas"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

func TestAuditDatabaseScopeAndExitCodeZero(t *testing.T) {
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

	var gotCfg mongoinspect.Config
	stubNewInspector(t, func(_ context.Context, cfg mongoinspect.Config) (inspector, error) {
		gotCfg = cfg
		return fake, nil
	})

	stdout, _, err := execCLI(t, "audit", "--uri", "mongodb://stub", "--database", "app", "--format", "json", "--timeout", "1s")
	if err != nil {
		t.Fatalf("audit returned error: %v", err)
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

func TestAuditExitCodes(t *testing.T) {
	tests := []struct {
		name        string
		collections []mongoinspect.CollectionInfo
		wantCode    int
	}{
		{
			name: "medium severity",
			collections: []mongoinspect.CollectionInfo{
				{Database: "app", Name: "empty", DocCount: 0, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
			},
			wantCode: 1,
		},
		{
			name: "high severity",
			collections: []mongoinspect.CollectionInfo{
				{Database: "app", Name: "orders", DocCount: 20000, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
			},
			wantCode: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeInspector{
				serverInfo:    mongoinspect.ServerInfo{Version: "7.0.0"},
				inspectResult: tc.collections,
			}
			stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
				return fake, nil
			})

			_, stderr, err := execCLI(t, "audit", "--uri", "mongodb://stub", "--timeout", "1s")
			requireExitCode(t, err, tc.wantCode)
			if !strings.Contains(stderr, "Exit ") {
				t.Fatalf("expected exit code hint in stderr, got: %q", stderr)
			}
		})
	}
}

func TestAuditBaselineLoadError(t *testing.T) {
	fake := &fakeInspector{
		serverInfo:    mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{{Database: "app", Name: "users", DocCount: 1}},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	missingBaseline := filepath.Join(t.TempDir(), "missing-baseline.json")
	_, _, err := execCLI(t, "audit", "--uri", "mongodb://stub", "--baseline", missingBaseline, "--timeout", "1s")
	if err == nil {
		t.Fatal("expected baseline load error")
	}
	if !strings.Contains(err.Error(), "load baseline") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditUsersWarningsPath(t *testing.T) {
	fake := &fakeInspector{
		serverInfo:       mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult:    []mongoinspect.CollectionInfo{{Database: "app", Name: "users", DocCount: 1}},
		listDatabasesRes: []mongoinspect.DatabaseInfo{{Name: "app"}},
		inspectUsersRes:  map[string][]mongoinspect.UserInfo{"app": nil},
		inspectUsersErr:  map[string]error{"admin": errors.New("permission denied")},
		listDatabasesErr: nil,
		serverInfoErr:    nil,
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	_, stderr, err := execCLI(t, "audit", "--uri", "mongodb://stub", "--audit-users", "--verbose", "--timeout", "1s")
	if err != nil {
		t.Fatalf("audit returned error: %v", err)
	}

	if !strings.Contains(stderr, "warning: could not list admin users") {
		t.Fatalf("expected admin users warning, got: %q", stderr)
	}
	if len(fake.listDatabasesCalls) != 1 {
		t.Fatalf("expected ListDatabases to be called once, got %d", len(fake.listDatabasesCalls))
	}
	if got := strings.Join(fake.inspectUsersCalls, ","); !strings.Contains(got, "admin") || !strings.Contains(got, "app") {
		t.Fatalf("expected InspectUsers calls for admin and app, got: %v", fake.inspectUsersCalls)
	}
}

func TestAuditEmptyCollectionsHint(t *testing.T) {
	fake := &fakeInspector{
		serverInfo:    mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: nil,
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	_, stderr, err := execCLI(t, "audit", "--uri", "mongodb://stub", "--timeout", "1s")
	if err != nil {
		t.Fatalf("audit returned error: %v", err)
	}
	if !strings.Contains(stderr, "Hint: no collections found") {
		t.Fatalf("expected empty-collections hint, got: %q", stderr)
	}
}

func TestAuditShardingFlagReportsFindings(t *testing.T) {
	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "events", DocCount: 100, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
		shardingRes: mongoinspect.ShardingInfo{
			Enabled: true,
			Shards:  []string{"shardA", "shardB"},
			Collections: []mongoinspect.ShardedCollectionInfo{
				{
					Namespace:  "app.events",
					Database:   "app",
					Collection: "events",
					Key:        []mongoinspect.KeyField{{Field: "_id", Direction: 1}},
					ChunkDistribution: map[string]int64{
						"shardA": 9,
						"shardB": 1,
					},
					ChunkCount:  10,
					JumboChunks: 1,
				},
			},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	stdout, _, err := execCLI(t, "audit", "--uri", "mongodb://stub", "--sharding", "--format", "json", "--timeout", "1s")
	requireExitCode(t, err, 2)

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid report JSON: %v", err)
	}
	assertHasType(t, report.Findings, analyzer.FindingMonotonicShardKey)
	assertHasType(t, report.Findings, analyzer.FindingUnbalancedChunks)
	assertHasType(t, report.Findings, analyzer.FindingJumboChunks)

	if fake.inspectShardingCalls != 1 {
		t.Fatalf("expected InspectSharding to be called once, got %d", fake.inspectShardingCalls)
	}
}

func TestAuditShardingFlagNonShardedDeployment(t *testing.T) {
	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "users", DocCount: 10, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
		shardingRes: mongoinspect.ShardingInfo{
			Enabled: false,
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	_, stderr, err := execCLI(t, "audit", "--uri", "mongodb://stub", "--sharding", "--timeout", "1s")
	if err != nil {
		t.Fatalf("audit returned error: %v", err)
	}
	if !strings.Contains(stderr, "deployment is not sharded") {
		t.Fatalf("expected non-sharded skip message, got: %q", stderr)
	}
	if fake.inspectShardingCalls != 1 {
		t.Fatalf("expected InspectSharding to be called once, got %d", fake.inspectShardingCalls)
	}
}

func assertHasType(t *testing.T, findings []analyzer.Finding, want analyzer.FindingType) {
	t.Helper()
	for _, finding := range findings {
		if finding.Type == want {
			return
		}
	}
	t.Fatalf("missing finding type %s", want)
}

func TestAuditAtlas_NoKeysGracefulSkip(t *testing.T) {
	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "users", DocCount: 10, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})
	stubNewAtlasClient(t, func(atlas.Config) (atlasClient, error) {
		t.Fatal("atlas client must not be created when no keys are provided")
		return nil, nil
	})

	stdout, _, err := execCLI(t, "audit", "--uri", "mongodb://stub", "--format", "json", "--timeout", "1s")
	if err != nil {
		t.Fatalf("audit returned error: %v", err)
	}
	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid report JSON: %v", err)
	}
	for _, finding := range report.Findings {
		if finding.Type == analyzer.FindingAtlasIndexSuggestion {
			t.Fatalf("unexpected atlas finding without keys: %+v", finding)
		}
	}
}

func TestAuditAtlas_IncludesCorrelatedIndexSuggestions(t *testing.T) {
	fakeInspector := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "orders", DocCount: 20000, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fakeInspector, nil
	})

	fakeAtlas := &fakeAtlasClient{
		clusterRes: atlas.Cluster{
			Name:             "Cluster0",
			MongoDBVersion:   "7.0.5",
			InstanceSizeName: "M30",
		},
		suggestionsRes: []atlas.SuggestedIndex{{
			Namespace:   "app.orders",
			IndexFields: []string{"status", "createdAt"},
		}},
	}
	stubNewAtlasClient(t, func(atlas.Config) (atlasClient, error) {
		return fakeAtlas, nil
	})
	stubScanRepo(t, func(string) (scanner.ScanResult, error) {
		return scanner.ScanResult{
			Collections: []string{"orders"},
			FieldRefs: []scanner.FieldRef{
				{Collection: "orders", Field: "status", File: "repo/orders.go", Line: 7},
			},
		}, nil
	})

	stdout, _, err := execCLI(t,
		"audit",
		"--uri", "mongodb://localhost:27017/testdb",
		"--atlas-public-key", "pub",
		"--atlas-private-key", "priv",
		"--atlas-project", "proj1",
		"--atlas-cluster", "Cluster0",
		"--format", "json",
		"--timeout", "1s",
	)
	requireExitCode(t, err, 2)

	var report reporter.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid report JSON: %v", err)
	}
	assertHasType(t, report.Findings, analyzer.FindingAtlasIndexSuggestion)

	var atlasSuggestion *analyzer.Finding
	for i := range report.Findings {
		if report.Findings[i].Type == analyzer.FindingAtlasIndexSuggestion {
			atlasSuggestion = &report.Findings[i]
			break
		}
	}
	if atlasSuggestion == nil {
		t.Fatal("expected atlas index suggestion finding")
	}
	if !strings.Contains(atlasSuggestion.Message, "matching queried code fields: status") {
		t.Fatalf("expected code correlation in message, got: %q", atlasSuggestion.Message)
	}
	if len(fakeAtlas.suggestedIndexCalls) != 1 {
		t.Fatalf("expected ListSuggestedIndexes call, got %d", len(fakeAtlas.suggestedIndexCalls))
	}
}

func TestAuditAtlas_MissingOneKeySkipsWithWarning(t *testing.T) {
	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "users", DocCount: 10, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})
	stubNewAtlasClient(t, func(atlas.Config) (atlasClient, error) {
		t.Fatal("atlas client must not be created when one key is missing")
		return nil, nil
	})

	_, stderr, err := execCLI(t, "audit", "--uri", "mongodb://stub", "--atlas-public-key", "pub", "--timeout", "1s")
	if err != nil {
		t.Fatalf("audit returned error: %v", err)
	}
	if !strings.Contains(stderr, "atlas integration skipped") {
		t.Fatalf("expected atlas skip warning, got: %q", stderr)
	}
}

func TestAtlasUsersToUserInfo(t *testing.T) {
	atlasUsers := []atlas.DatabaseUser{
		{
			Username:     "admin",
			DatabaseName: "admin",
			Roles: []atlas.DatabaseUserRole{
				{RoleName: "readWriteAnyDatabase", DatabaseName: "admin"},
				{RoleName: "dbAdminAnyDatabase", DatabaseName: "admin"},
			},
			Scopes: []atlas.DatabaseUserScope{
				{Name: "Cluster0", Type: "CLUSTER"},
			},
		},
		{
			Username:     "appuser",
			DatabaseName: "admin",
			Roles: []atlas.DatabaseUserRole{
				{RoleName: "readWrite", DatabaseName: "myapp"},
			},
		},
	}

	result := atlasUsersToUserInfo(atlasUsers)

	if len(result) != 2 {
		t.Fatalf("expected 2 users, got %d", len(result))
	}
	if result[0].Username != "admin" {
		t.Errorf("expected username 'admin', got %q", result[0].Username)
	}
	if result[0].Database != "admin" {
		t.Errorf("expected database 'admin', got %q", result[0].Database)
	}
	if len(result[0].Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(result[0].Roles))
	}
	if result[0].Roles[0].Role != "readWriteAnyDatabase" {
		t.Errorf("expected role 'readWriteAnyDatabase', got %q", result[0].Roles[0].Role)
	}
	if result[0].Roles[0].DB != "admin" {
		t.Errorf("expected db 'admin', got %q", result[0].Roles[0].DB)
	}
	if result[1].Username != "appuser" {
		t.Errorf("expected username 'appuser', got %q", result[1].Username)
	}
	if len(result[1].Roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(result[1].Roles))
	}
}

func TestAuditUsers_AtlasFallback(t *testing.T) {
	// Native usersInfo fails, but Atlas API succeeds.
	fake := &fakeInspector{
		serverInfo: mongoinspect.ServerInfo{Version: "7.0.0"},
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "users", DocCount: 10, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
		inspectUsersErr: map[string]error{
			"admin": errors.New("command usersInfo not permitted"),
			"app":   errors.New("command usersInfo not permitted"),
		},
		listDatabasesRes: []mongoinspect.DatabaseInfo{
			{Name: "app"},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	fakeAtlas := &fakeAtlasClient{
		resolveProjectIDRes: "proj123",
		databaseUsersRes: []atlas.DatabaseUser{
			{
				Username:     "atlasAdmin",
				DatabaseName: "admin",
				Roles: []atlas.DatabaseUserRole{
					{RoleName: "root", DatabaseName: "admin"},
				},
			},
			{
				Username:     "atlasApp",
				DatabaseName: "admin",
				Roles: []atlas.DatabaseUserRole{
					{RoleName: "readWrite", DatabaseName: "app"},
				},
			},
		},
	}
	stubNewAtlasClient(t, func(atlas.Config) (atlasClient, error) {
		return fakeAtlas, nil
	})

	stdout, stderr, err := execCLI(t,
		"audit",
		"--uri", "mongodb://stub",
		"--audit-users",
		"--atlas-public-key", "pub",
		"--atlas-private-key", "priv",
		"--atlas-project", "proj123",
		"--atlas-cluster", "Cluster0",
		"--format", "json",
		"--timeout", "1s",
	)
	requireExitCode(t, err, 1)

	if !strings.Contains(stderr, "Fetched 2 users via Atlas API") {
		t.Errorf("expected Atlas API success message, got stderr: %q", stderr)
	}

	var report reporter.Report
	if jsonErr := json.Unmarshal([]byte(stdout), &report); jsonErr != nil {
		t.Fatalf("invalid report JSON: %v", jsonErr)
	}

	// Should have OVERPRIVILEGED_USER for root user.
	assertHasType(t, report.Findings, analyzer.FindingOverprivilegedUser)
	// Should have ATLAS_USER_NO_SCOPE for both users (no scopes).
	assertHasType(t, report.Findings, analyzer.FindingAtlasUserNoScope)
}
