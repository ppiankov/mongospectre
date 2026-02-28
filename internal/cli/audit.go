package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	"github.com/ppiankov/mongospectre/internal/atlas"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/spf13/cobra"
)

func newAuditCmd() *cobra.Command {
	var (
		database        string
		format          string
		noIgnore        bool
		baseline        string
		auditUsers      bool
		sharding        bool
		atlasPublicKey  string
		atlasPrivateKey string
		atlasProject    string
		atlasCluster    string
		interactive     bool
		noInteractive   bool
		lintURI         bool
		security        bool
		replset         bool
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit MongoDB cluster for unused collections, indexes, and drift",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(format, "text", "json", "sarif", "spectrehub"); err != nil {
				return err
			}
			if interactive && noInteractive {
				return fmt.Errorf("--interactive and --no-interactive are mutually exclusive")
			}
			if uri == "" {
				return fmt.Errorf("--uri is required (or set MONGODB_URI)")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			if verbose {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to %s (timeout %s)...\n", uri, timeout)
			}

			inspector, err := newInspector(ctx, mongoinspect.Config{
				URI:      uri,
				Database: database,
			})
			if err != nil {
				return err
			}
			defer func() { _ = inspector.Close(ctx) }()

			info, err := inspector.GetServerVersion(ctx)
			if err != nil {
				return fmt.Errorf("server info: %w", err)
			}
			host := reporter.HostFromURI(uri)
			if host != "" {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connected to MongoDB %s at %s\n", info.Version, host)
			} else {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connected to MongoDB %s\n", info.Version)
			}

			collections, err := inspector.Inspect(ctx, database)
			if err != nil {
				return fmt.Errorf("inspect: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Inspected %d collections\n", len(collections))

			if len(collections) == 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Hint: no collections found. Check that the URI points to a database with data, or use --database to specify one.\n")
			}

			if verbose {
				for _, c := range collections {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s.%s (%d docs, %d indexes)\n",
						c.Database, c.Name, c.DocCount, len(c.Indexes))
				}
			}

			var findings []analyzer.Finding

			// URI linting: static analysis before connecting.
			if lintURI {
				findings = append(findings, analyzer.LintURI(uri)...)
			}

			findings = append(findings, analyzer.Audit(collections)...)

			if auditUsers {
				var allUsers []mongoinspect.UserInfo
				var userErrors int

				// Query admin database for cluster-level users.
				adminUsers, adminErr := inspector.InspectUsers(ctx, "admin")
				if adminErr != nil {
					if verbose {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not list admin users: %v\n", adminErr)
					}
					userErrors++
				} else {
					allUsers = append(allUsers, adminUsers...)
				}

				// Query each application database.
				dbs, dbsErr := inspector.ListDatabases(ctx, database)
				if dbsErr == nil {
					for _, db := range dbs {
						dbUsers, dbErr := inspector.InspectUsers(ctx, db.Name)
						if dbErr != nil {
							if verbose {
								_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not list users on %s: %v\n", db.Name, dbErr)
							}
							userErrors++
							continue
						}
						allUsers = append(allUsers, dbUsers...)
					}
				}

				// Atlas API fallback: when native usersInfo fails, try Atlas Admin API.
				var atlasUsers []atlas.DatabaseUser
				if len(allUsers) == 0 && userErrors > 0 {
					atlasUsers = collectAtlasUsers(ctx, cmd, atlasOptions{
						PublicKey:  atlasPublicKey,
						PrivateKey: atlasPrivateKey,
						ProjectID:  atlasProject,
						Cluster:    atlasCluster,
					}, uri)
					if len(atlasUsers) > 0 {
						allUsers = append(allUsers, atlasUsersToUserInfo(atlasUsers)...)
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Fetched %d users via Atlas API\n", len(atlasUsers))
					}
				}

				if len(allUsers) == 0 && userErrors > 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"WARNING: --audit-users produced no results (%d databases denied access).\n"+
							"  Native MongoDB usersInfo requires userAdmin or userAdminAnyDatabase role.\n"+
							"  On Atlas, use --atlas-public-key and --atlas-private-key to audit users via Atlas API.\n"+
							"  See: docs/troubleshooting.md\n", userErrors)
				} else if len(atlasUsers) == 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Inspected %d users\n", len(allUsers))
				}

				userFindings := analyzer.AuditUsers(allUsers)
				findings = append(findings, userFindings...)

				// Atlas-specific user findings (scope analysis).
				if len(atlasUsers) > 0 {
					findings = append(findings, analyzer.AuditAtlasUsers(atlasUsers)...)

					// Atlas access log analysis: detect inactive users.
					accessLogs := collectAccessLogs(ctx, cmd, atlasOptions{
						PublicKey:  atlasPublicKey,
						PrivateKey: atlasPrivateKey,
						ProjectID:  atlasProject,
						Cluster:    atlasCluster,
					}, uri)
					if accessLogs != nil {
						inactiveFindings := analyzer.DetectInactiveUsers(atlasUsers, accessLogs)
						findings = append(findings, inactiveFindings...)
					}
				}
			}

			if sharding {
				shardingInfo, shardingErr := inspector.InspectSharding(ctx)
				switch {
				case shardingErr != nil:
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: sharding analysis skipped: %v\n", shardingErr)
				case !shardingInfo.Enabled:
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Sharding analysis skipped: deployment is not sharded.")
				default:
					if verbose {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Inspected sharding metadata for %d collections across %d shards\n",
							len(shardingInfo.Collections), len(shardingInfo.Shards))
					}
					findings = append(findings, analyzer.AuditSharding(collections, shardingInfo)...)
				}
			}

			if security {
				if isAtlasURI(uri) {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Security audit skipped: Atlas manages server security configuration.")
				} else {
					secInfo, secErr := inspector.InspectSecurity(ctx)
					if secErr != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: security audit skipped: %v\n", secErr)
					} else {
						findings = append(findings, analyzer.AuditSecurity(secInfo)...)
					}
				}
			}

			if replset {
				if isAtlasURI(uri) {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Replica set audit skipped: Atlas manages replica set topology.")
				} else {
					rsInfo, rsErr := inspector.InspectReplicaSet(ctx)
					switch {
					case rsErr != nil:
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: replica set audit skipped: %v\n", rsErr)
					case rsInfo.Name != "":
						findings = append(findings, analyzer.AuditReplicaSet(rsInfo)...)
					default:
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Replica set audit skipped: standalone deployment.")
					}
				}
			}

			atlasFindings := collectAtlasFindings(ctx, cmd, atlasOptions{
				PublicKey:  atlasPublicKey,
				PrivateKey: atlasPrivateKey,
				ProjectID:  atlasProject,
				Cluster:    atlasCluster,
			}, uri, collections)
			findings = append(findings, atlasFindings...)

			// Baseline: load collections for growth detection, then diff findings.
			var baselineFindings []analyzer.Finding
			if baseline != "" {
				var baselineCollections []mongoinspect.CollectionInfo
				var baselineTime time.Time
				var blErr error
				baselineFindings, baselineCollections, baselineTime, blErr = analyzer.LoadBaselineWithCollections(baseline)
				if blErr != nil {
					return fmt.Errorf("load baseline: %w", blErr)
				}
				if len(baselineCollections) > 0 && len(collections) > 0 {
					elapsed := time.Since(baselineTime)
					if !baselineTime.IsZero() {
						findings = append(findings, analyzer.DetectGrowth(collections, baselineCollections, elapsed)...)
					}
				}
			}

			// Apply ignore file.
			if !noIgnore {
				cwd, _ := os.Getwd()
				il, ilErr := analyzer.LoadIgnoreFile(cwd)
				if ilErr != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", ilErr)
				}
				var suppressed int
				findings, suppressed = il.Filter(findings)
				if verbose && suppressed > 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Suppressed %d findings via .mongospectreignore\n", suppressed)
				}
			}

			// Baseline diff display.
			if baseline != "" {
				diff := analyzer.DiffBaseline(findings, baselineFindings)
				reporter.WriteBaselineDiff(cmd.OutOrStdout(), diff)
			}

			report := reporter.NewReport(findings)
			report.Metadata = reporter.Metadata{
				Version:        version,
				Command:        "audit",
				Host:           host,
				Database:       database,
				MongoDBVersion: info.Version,
				URIHash:        reporter.HashURI(uri),
			}
			report.Collections = collections

			renderedInteractive, err := maybeRenderInteractive(cmd, &report, collections, nil, interactiveConfig{
				force:    interactive,
				disable:  noInteractive,
				format:   format,
				findings: len(findings),
			})
			if err != nil {
				return err
			}
			if !renderedInteractive {
				if err := reporter.Write(cmd.OutOrStdout(), &report, reporter.Format(format)); err != nil {
					return fmt.Errorf("write report: %w", err)
				}
			}

			code := analyzer.ExitCode(report.MaxSeverity)
			if code != 0 {
				if hint := reporter.ExitCodeHint(code); hint != "" {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), hint)
				}
				return &ExitError{Code: code}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&database, "database", "", "specific database to audit (default: all non-system)")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "output format: text, json, sarif, or spectrehub")
	cmd.Flags().BoolVar(&noIgnore, "no-ignore", false, "bypass .mongospectreignore file")
	cmd.Flags().StringVar(&baseline, "baseline", "", "path to previous JSON report for diff comparison")
	cmd.Flags().BoolVar(&auditUsers, "audit-users", false, "audit MongoDB user configurations (requires userAdmin role)")
	cmd.Flags().BoolVar(&sharding, "sharding", false, "run sharding metadata analysis (requires access to config database)")
	cmd.Flags().StringVar(&atlasPublicKey, "atlas-public-key", "", "MongoDB Atlas API public key (env: ATLAS_PUBLIC_KEY)")
	cmd.Flags().StringVar(&atlasPrivateKey, "atlas-private-key", "", "MongoDB Atlas API private key (env: ATLAS_PRIVATE_KEY)")
	cmd.Flags().StringVar(&atlasProject, "atlas-project", "", "MongoDB Atlas project/group ID (env: ATLAS_PROJECT_ID)")
	cmd.Flags().StringVar(&atlasCluster, "atlas-cluster", "", "MongoDB Atlas cluster name (auto-derived from URI if possible)")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "launch interactive terminal UI (text format only)")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "force non-interactive output")
	cmd.Flags().BoolVar(&lintURI, "lint-uri", true, "lint MongoDB URI for common misconfigurations")
	cmd.Flags().BoolVar(&security, "security", false, "audit server security configuration (requires admin access)")
	cmd.Flags().BoolVar(&replset, "replset", false, "audit replica set configuration (requires admin access)")

	return cmd
}

// isAtlasURI returns true if the URI hostname indicates a MongoDB Atlas cluster.
func isAtlasURI(rawURI string) bool {
	host := reporter.HostFromURI(rawURI)
	return strings.Contains(strings.ToLower(host), ".mongodb.net")
}
