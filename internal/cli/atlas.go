package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	"github.com/ppiankov/mongospectre/internal/atlas"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
	"github.com/spf13/cobra"
)

type atlasOptions struct {
	PublicKey  string
	PrivateKey string
	ProjectID  string
	Cluster    string
}

func resolveAtlasOptions(opts atlasOptions) atlasOptions {
	resolved := atlasOptions{
		PublicKey:  strings.TrimSpace(opts.PublicKey),
		PrivateKey: strings.TrimSpace(opts.PrivateKey),
		ProjectID:  strings.TrimSpace(opts.ProjectID),
		Cluster:    strings.TrimSpace(opts.Cluster),
	}

	if resolved.PublicKey == "" {
		resolved.PublicKey = strings.TrimSpace(os.Getenv("ATLAS_PUBLIC_KEY"))
	}
	if resolved.PrivateKey == "" {
		resolved.PrivateKey = strings.TrimSpace(os.Getenv("ATLAS_PRIVATE_KEY"))
	}
	if resolved.ProjectID == "" {
		resolved.ProjectID = strings.TrimSpace(os.Getenv("ATLAS_PROJECT_ID"))
	}
	if resolved.Cluster == "" {
		resolved.Cluster = strings.TrimSpace(os.Getenv("ATLAS_CLUSTER"))
	}
	return resolved
}

func collectAtlasFindings(
	ctx context.Context,
	cmd *cobra.Command,
	opts atlasOptions,
	mongoURI string,
	collections []mongoinspect.CollectionInfo,
) []analyzer.Finding {
	resolved := resolveAtlasOptions(opts)

	hasPublic := resolved.PublicKey != ""
	hasPrivate := resolved.PrivateKey != ""
	switch {
	case !hasPublic && !hasPrivate:
		if verbose {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Atlas integration skipped: no Atlas API credentials provided.")
		}
		return nil
	case !hasPublic || !hasPrivate:
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: atlas integration skipped: both --atlas-public-key and --atlas-private-key are required")
		return nil
	}

	if resolved.Cluster == "" {
		resolved.Cluster = deriveAtlasClusterName(mongoURI)
	}

	atlasClient, err := newAtlasClient(atlas.Config{
		PublicKey:  resolved.PublicKey,
		PrivateKey: resolved.PrivateKey,
	})
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas integration skipped: %v\n", err)
		return nil
	}

	projectID := resolved.ProjectID
	clusterName := resolved.Cluster
	projectID, clusterName = resolveAtlasTarget(ctx, cmd, atlasClient, projectID, clusterName)
	if projectID == "" || clusterName == "" {
		return nil
	}

	cluster := atlas.Cluster{Name: clusterName}
	if c, err := atlasClient.GetCluster(ctx, projectID, clusterName); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas cluster metadata unavailable: %v\n", err)
	} else {
		cluster = c
	}

	suggestions, err := atlasClient.ListSuggestedIndexes(ctx, projectID, clusterName)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas index suggestions unavailable: %v\n", err)
	}

	var scanRes *scanner.ScanResult
	if len(suggestions) > 0 {
		if scan, ok := scanRepoForAtlas(cmd); ok {
			scanRes = &scan
		}
	}

	alerts, err := atlasClient.ListAlerts(ctx, projectID)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas alerts unavailable: %v\n", err)
	}

	versions, err := atlasClient.ListMongoDBVersions(ctx, projectID)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas mongoDB versions unavailable: %v\n", err)
	}

	findings := analyzer.AuditAtlas(&analyzer.AtlasAuditInput{
		ProjectID:         projectID,
		Cluster:           cluster,
		SuggestedIndexes:  suggestions,
		Alerts:            alerts,
		AvailableVersions: versions,
		Collections:       collections,
		Scan:              scanRes,
	})

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Atlas enrichment: project=%s cluster=%s findings=%d\n", projectID, clusterName, len(findings))
	}
	return findings
}

func resolveAtlasTarget(
	ctx context.Context,
	cmd *cobra.Command,
	client atlasClient,
	projectID, clusterName string,
) (string, string) {
	if projectID == "" && clusterName != "" {
		resolvedProject, err := client.ResolveProjectIDByCluster(ctx, clusterName)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas project auto-discovery failed: %v\n", err)
		} else {
			projectID = resolvedProject
		}
	}

	if clusterName == "" && projectID != "" {
		clusters, err := client.ListClusters(ctx, projectID)
		switch {
		case err != nil:
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas cluster auto-discovery failed: %v\n", err)
		case len(clusters) == 1:
			clusterName = clusters[0].Name
		case len(clusters) > 1:
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: atlas cluster auto-discovery is ambiguous; set --atlas-cluster")
		}
	}

	if projectID == "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: atlas integration skipped: missing Atlas project (set --atlas-project or ATLAS_PROJECT_ID)")
		return "", ""
	}
	if clusterName == "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: atlas integration skipped: missing Atlas cluster (set --atlas-cluster)")
		return "", ""
	}
	return projectID, clusterName
}

func deriveAtlasClusterName(mongoURI string) string {
	parsed, err := url.Parse(mongoURI)
	if err != nil {
		return ""
	}

	host := parsed.Host
	if host == "" {
		return ""
	}

	firstHost := strings.Split(host, ",")[0]
	firstHost = strings.TrimSpace(firstHost)
	if firstHost == "" {
		return ""
	}

	if strings.Contains(firstHost, "@") {
		parts := strings.SplitN(firstHost, "@", 2)
		firstHost = parts[1]
	}

	hostOnly := firstHost
	if idx := strings.Index(hostOnly, ":"); idx >= 0 {
		hostOnly = hostOnly[:idx]
	}
	label := strings.Split(hostOnly, ".")[0]
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}

	if strings.Contains(label, "-shard-") {
		return strings.SplitN(label, "-shard-", 2)[0]
	}
	return label
}

// collectAtlasUsers fetches database users via the Atlas Admin API.
// Returns nil if Atlas credentials are not available or the API call fails.
func collectAtlasUsers(
	ctx context.Context,
	cmd *cobra.Command,
	opts atlasOptions,
	mongoURI string,
) []atlas.DatabaseUser {
	resolved := resolveAtlasOptions(opts)

	if resolved.PublicKey == "" || resolved.PrivateKey == "" {
		if verbose {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Atlas user audit skipped: no Atlas API credentials provided.")
		}
		return nil
	}

	if resolved.Cluster == "" {
		resolved.Cluster = deriveAtlasClusterName(mongoURI)
	}

	atlasClient, err := newAtlasClient(atlas.Config{
		PublicKey:  resolved.PublicKey,
		PrivateKey: resolved.PrivateKey,
	})
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas user audit skipped: %v\n", err)
		return nil
	}

	projectID := resolved.ProjectID
	clusterName := resolved.Cluster
	projectID, _ = resolveAtlasTarget(ctx, cmd, atlasClient, projectID, clusterName)
	if projectID == "" {
		return nil
	}

	users, err := atlasClient.ListDatabaseUsers(ctx, projectID)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas user audit failed: %v\n", err)
		return nil
	}
	return users
}

// atlasUsersToUserInfo converts Atlas API database users to the internal
// UserInfo type so they flow through the standard AuditUsers analyzer.
func atlasUsersToUserInfo(users []atlas.DatabaseUser) []mongoinspect.UserInfo {
	result := make([]mongoinspect.UserInfo, 0, len(users))
	for _, u := range users {
		info := mongoinspect.UserInfo{
			Username: u.Username,
			Database: u.DatabaseName,
		}
		for _, r := range u.Roles {
			info.Roles = append(info.Roles, mongoinspect.UserRole{
				Role: r.RoleName,
				DB:   r.DatabaseName,
			})
		}
		result = append(result, info)
	}
	return result
}

// collectAccessLogs fetches authentication history via the Atlas Admin API.
// Returns nil if Atlas credentials are not available, cluster name is unknown,
// or the API call fails (graceful degradation for M0/M2/M5 and insufficient roles).
func collectAccessLogs(
	ctx context.Context,
	cmd *cobra.Command,
	opts atlasOptions,
	mongoURI string,
) []atlas.AccessLogEntry {
	resolved := resolveAtlasOptions(opts)

	if resolved.PublicKey == "" || resolved.PrivateKey == "" {
		return nil
	}

	if resolved.Cluster == "" {
		resolved.Cluster = deriveAtlasClusterName(mongoURI)
	}

	client, err := newAtlasClient(atlas.Config{
		PublicKey:  resolved.PublicKey,
		PrivateKey: resolved.PrivateKey,
	})
	if err != nil {
		return nil
	}

	projectID := resolved.ProjectID
	clusterName := resolved.Cluster
	projectID, clusterName = resolveAtlasTarget(ctx, cmd, client, projectID, clusterName)
	if projectID == "" || clusterName == "" {
		return nil
	}

	logs, err := client.ListAccessLogs(ctx, projectID, clusterName)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: access log analysis skipped: %v\n", err)
		return nil
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Fetched %d access log entries\n", len(logs))
	}
	return logs
}

func scanRepoForAtlas(cmd *cobra.Command) (scanner.ScanResult, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas code correlation skipped: %v\n", err)
		return scanner.ScanResult{}, false
	}

	scan, err := scanRepo(cwd)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: atlas code correlation skipped: %v\n", err)
		return scanner.ScanResult{}, false
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Atlas code correlation scanned %d files (%d field refs)\n", scan.FilesScanned, len(scan.FieldRefs))
	}
	return scan, true
}
