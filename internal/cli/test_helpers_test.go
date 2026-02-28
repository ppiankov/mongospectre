package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ppiankov/mongospectre/internal/atlas"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

type fakeInspector struct {
	serverInfo       mongoinspect.ServerInfo
	serverInfoErr    error
	inspectResult    []mongoinspect.CollectionInfo
	inspectByDB      map[string][]mongoinspect.CollectionInfo
	inspectErr       error
	inspectHook      func(string)
	listDatabasesRes []mongoinspect.DatabaseInfo
	listDatabasesErr error
	inspectUsersRes  map[string][]mongoinspect.UserInfo
	inspectUsersErr  map[string]error
	profilerRes      []mongoinspect.ProfileEntry
	profilerErr      error
	validatorsRes    []mongoinspect.ValidatorInfo
	validatorsErr    error
	shardingRes      mongoinspect.ShardingInfo
	shardingErr      error
	sampleDocsRes    []mongoinspect.FieldSampleResult
	sampleDocsErr    error
	securityRes      mongoinspect.SecurityInfo
	securityErr      error
	replsetRes       mongoinspect.ReplicaSetInfo
	replsetErr       error
	closeErr         error

	inspectCalls           []string
	listDatabasesCalls     []string
	inspectUsersCalls      []string
	profilerCalls          []profilerCall
	sampleDocsCalls        []sampleDocsCall
	inspectShardingCalls   int
	inspectSecurityCalls   int
	inspectReplicaSetCalls int
	closeCalls             int
}

type profilerCall struct {
	database string
	limit    int64
}

type sampleDocsCall struct {
	database   string
	sampleSize int64
}

type fakeAtlasClient struct {
	clusterRes          atlas.Cluster
	clusterErr          error
	suggestionsRes      []atlas.SuggestedIndex
	suggestionsErr      error
	alertsRes           []atlas.Alert
	alertsErr           error
	versionsRes         []string
	versionsErr         error
	projectsRes         []atlas.Project
	projectsErr         error
	clustersRes         []atlas.Cluster
	clustersErr         error
	resolveProjectIDRes string
	resolveProjectIDErr error
	databaseUsersRes    []atlas.DatabaseUser
	databaseUsersErr    error
	accessLogsRes       []atlas.AccessLogEntry
	accessLogsErr       error

	getClusterCalls     []string
	suggestedIndexCalls []string
	alertCalls          []string
	versionCalls        []string
	listProjectsCalls   int
	listClustersCalls   []string
	resolveProjectCalls []string
	databaseUsersCalls  []string
	accessLogsCalls     []string
}

func (f *fakeInspector) Close(context.Context) error {
	f.closeCalls++
	return f.closeErr
}

func (f *fakeInspector) GetServerVersion(context.Context) (mongoinspect.ServerInfo, error) {
	if f.serverInfoErr != nil {
		return mongoinspect.ServerInfo{}, f.serverInfoErr
	}
	return f.serverInfo, nil
}

func (f *fakeInspector) ReadProfiler(_ context.Context, database string, limit int64) ([]mongoinspect.ProfileEntry, error) {
	f.profilerCalls = append(f.profilerCalls, profilerCall{database: database, limit: limit})
	if f.profilerErr != nil {
		return nil, f.profilerErr
	}
	return append([]mongoinspect.ProfileEntry(nil), f.profilerRes...), nil
}

func (f *fakeInspector) GetValidators(context.Context, string) ([]mongoinspect.ValidatorInfo, error) {
	if f.validatorsErr != nil {
		return nil, f.validatorsErr
	}
	return append([]mongoinspect.ValidatorInfo(nil), f.validatorsRes...), nil
}

func (f *fakeInspector) Inspect(_ context.Context, database string) ([]mongoinspect.CollectionInfo, error) {
	f.inspectCalls = append(f.inspectCalls, database)
	if f.inspectHook != nil {
		f.inspectHook(database)
	}
	if f.inspectErr != nil {
		return nil, f.inspectErr
	}
	if f.inspectByDB != nil {
		if res, ok := f.inspectByDB[database]; ok {
			return append([]mongoinspect.CollectionInfo(nil), res...), nil
		}
	}
	return append([]mongoinspect.CollectionInfo(nil), f.inspectResult...), nil
}

func (f *fakeInspector) InspectUsers(_ context.Context, dbName string) ([]mongoinspect.UserInfo, error) {
	f.inspectUsersCalls = append(f.inspectUsersCalls, dbName)
	if err, ok := f.inspectUsersErr[dbName]; ok {
		return nil, err
	}
	if users, ok := f.inspectUsersRes[dbName]; ok {
		return append([]mongoinspect.UserInfo(nil), users...), nil
	}
	return nil, nil
}

func (f *fakeInspector) InspectSharding(context.Context) (mongoinspect.ShardingInfo, error) {
	f.inspectShardingCalls++
	if f.shardingErr != nil {
		return mongoinspect.ShardingInfo{}, f.shardingErr
	}
	return f.shardingRes, nil
}

func (f *fakeInspector) InspectSecurity(context.Context) (mongoinspect.SecurityInfo, error) {
	f.inspectSecurityCalls++
	if f.securityErr != nil {
		return mongoinspect.SecurityInfo{}, f.securityErr
	}
	return f.securityRes, nil
}

func (f *fakeInspector) InspectReplicaSet(context.Context) (mongoinspect.ReplicaSetInfo, error) {
	f.inspectReplicaSetCalls++
	if f.replsetErr != nil {
		return mongoinspect.ReplicaSetInfo{}, f.replsetErr
	}
	return f.replsetRes, nil
}

func (f *fakeInspector) SampleDocuments(_ context.Context, database string, sampleSize int64) ([]mongoinspect.FieldSampleResult, error) {
	f.sampleDocsCalls = append(f.sampleDocsCalls, sampleDocsCall{database: database, sampleSize: sampleSize})
	if f.sampleDocsErr != nil {
		return nil, f.sampleDocsErr
	}
	return append([]mongoinspect.FieldSampleResult(nil), f.sampleDocsRes...), nil
}

func (f *fakeInspector) ListDatabases(_ context.Context, database string) ([]mongoinspect.DatabaseInfo, error) {
	f.listDatabasesCalls = append(f.listDatabasesCalls, database)
	if f.listDatabasesErr != nil {
		return nil, f.listDatabasesErr
	}
	return append([]mongoinspect.DatabaseInfo(nil), f.listDatabasesRes...), nil
}

func (f *fakeAtlasClient) GetCluster(_ context.Context, projectID, clusterName string) (atlas.Cluster, error) {
	f.getClusterCalls = append(f.getClusterCalls, projectID+"/"+clusterName)
	if f.clusterErr != nil {
		return atlas.Cluster{}, f.clusterErr
	}
	return f.clusterRes, nil
}

func (f *fakeAtlasClient) ListAlerts(_ context.Context, projectID string) ([]atlas.Alert, error) {
	f.alertCalls = append(f.alertCalls, projectID)
	if f.alertsErr != nil {
		return nil, f.alertsErr
	}
	return append([]atlas.Alert(nil), f.alertsRes...), nil
}

func (f *fakeAtlasClient) ListMongoDBVersions(_ context.Context, projectID string) ([]string, error) {
	f.versionCalls = append(f.versionCalls, projectID)
	if f.versionsErr != nil {
		return nil, f.versionsErr
	}
	return append([]string(nil), f.versionsRes...), nil
}

func (f *fakeAtlasClient) ListProjects(context.Context) ([]atlas.Project, error) {
	f.listProjectsCalls++
	if f.projectsErr != nil {
		return nil, f.projectsErr
	}
	return append([]atlas.Project(nil), f.projectsRes...), nil
}

func (f *fakeAtlasClient) ListSuggestedIndexes(_ context.Context, projectID, clusterName string) ([]atlas.SuggestedIndex, error) {
	f.suggestedIndexCalls = append(f.suggestedIndexCalls, projectID+"/"+clusterName)
	if f.suggestionsErr != nil {
		return nil, f.suggestionsErr
	}
	return append([]atlas.SuggestedIndex(nil), f.suggestionsRes...), nil
}

func (f *fakeAtlasClient) ListClusters(_ context.Context, projectID string) ([]atlas.Cluster, error) {
	f.listClustersCalls = append(f.listClustersCalls, projectID)
	if f.clustersErr != nil {
		return nil, f.clustersErr
	}
	return append([]atlas.Cluster(nil), f.clustersRes...), nil
}

func (f *fakeAtlasClient) ResolveProjectIDByCluster(_ context.Context, clusterName string) (string, error) {
	f.resolveProjectCalls = append(f.resolveProjectCalls, clusterName)
	if f.resolveProjectIDErr != nil {
		return "", f.resolveProjectIDErr
	}
	return f.resolveProjectIDRes, nil
}

func (f *fakeAtlasClient) ListDatabaseUsers(_ context.Context, projectID string) ([]atlas.DatabaseUser, error) {
	f.databaseUsersCalls = append(f.databaseUsersCalls, projectID)
	if f.databaseUsersErr != nil {
		return nil, f.databaseUsersErr
	}
	return append([]atlas.DatabaseUser(nil), f.databaseUsersRes...), nil
}

func (f *fakeAtlasClient) ListAccessLogs(_ context.Context, projectID, clusterName string) ([]atlas.AccessLogEntry, error) {
	f.accessLogsCalls = append(f.accessLogsCalls, projectID+"/"+clusterName)
	if f.accessLogsErr != nil {
		return nil, f.accessLogsErr
	}
	return append([]atlas.AccessLogEntry(nil), f.accessLogsRes...), nil
}

func stubNewInspector(t *testing.T, fn func(context.Context, mongoinspect.Config) (inspector, error)) {
	t.Helper()
	orig := newInspector
	newInspector = fn
	t.Cleanup(func() {
		newInspector = orig
	})
}

func stubScanRepo(t *testing.T, fn func(string) (scanner.ScanResult, error)) {
	t.Helper()
	orig := scanRepo
	scanRepo = fn
	t.Cleanup(func() {
		scanRepo = orig
	})
}

func stubNewAtlasClient(t *testing.T, fn func(atlas.Config) (atlasClient, error)) {
	t.Helper()
	orig := newAtlasClient
	newAtlasClient = fn
	t.Cleanup(func() {
		newAtlasClient = orig
	})
}

func execCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	prevURI := uri
	prevVerbose := verbose
	prevTimeout := timeout
	prevVersion := version
	t.Cleanup(func() {
		uri = prevURI
		verbose = prevVerbose
		timeout = prevTimeout
		version = prevVersion
	})
	uri = ""
	verbose = false
	timeout = 30 * time.Second
	version = "test-version"

	cmd := newRootCmd(testBuildInfo)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func requireExitCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected ExitError(%d), got nil", want)
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError(%d), got %T (%v)", want, err, err)
	}
	if exitErr.Code != want {
		t.Fatalf("exit code = %d, want %d", exitErr.Code, want)
	}
}
