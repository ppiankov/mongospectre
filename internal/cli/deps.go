package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/ppiankov/mongospectre/internal/atlas"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

type inspector interface {
	Close(ctx context.Context) error
	GetServerVersion(ctx context.Context) (mongoinspect.ServerInfo, error)
	ReadProfiler(ctx context.Context, database string, limit int64) ([]mongoinspect.ProfileEntry, error)
	GetValidators(ctx context.Context, database string) ([]mongoinspect.ValidatorInfo, error)
	Inspect(ctx context.Context, database string) ([]mongoinspect.CollectionInfo, error)
	InspectSharding(ctx context.Context) (mongoinspect.ShardingInfo, error)
	InspectUsers(ctx context.Context, dbName string) ([]mongoinspect.UserInfo, error)
	ListDatabases(ctx context.Context, database string) ([]mongoinspect.DatabaseInfo, error)
	SampleDocuments(ctx context.Context, database string, sampleSize int64) ([]mongoinspect.FieldSampleResult, error)
}

type atlasClient interface {
	GetCluster(ctx context.Context, projectID, clusterName string) (atlas.Cluster, error)
	ListAlerts(ctx context.Context, projectID string) ([]atlas.Alert, error)
	ListMongoDBVersions(ctx context.Context, projectID string) ([]string, error)
	ListProjects(ctx context.Context) ([]atlas.Project, error)
	ListSuggestedIndexes(ctx context.Context, projectID, clusterName string) ([]atlas.SuggestedIndex, error)
	ListClusters(ctx context.Context, projectID string) ([]atlas.Cluster, error)
	ResolveProjectIDByCluster(ctx context.Context, clusterName string) (string, error)
	ListDatabaseUsers(ctx context.Context, projectID string) ([]atlas.DatabaseUser, error)
	ListAccessLogs(ctx context.Context, projectID, clusterName string) ([]atlas.AccessLogEntry, error)
}

var (
	newInspector = func(ctx context.Context, cfg mongoinspect.Config) (inspector, error) {
		return mongoinspect.NewInspector(ctx, cfg)
	}
	newAtlasClient = func(cfg atlas.Config) (atlasClient, error) {
		return atlas.NewClient(cfg)
	}
	scanRepo = scanner.Scan
)

func validateFormat(format string, allowed ...string) error {
	for _, v := range allowed {
		if format == v {
			return nil
		}
	}
	return fmt.Errorf("invalid --format %q (allowed: %s)", format, strings.Join(allowed, ", "))
}
