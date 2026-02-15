package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/spf13/cobra"
)

func newAuditCmd() *cobra.Command {
	var (
		database   string
		format     string
		noIgnore   bool
		baseline   string
		auditUsers bool
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit MongoDB cluster for unused collections, indexes, and drift",
		RunE: func(cmd *cobra.Command, args []string) error {
			if uri == "" {
				return fmt.Errorf("--uri is required (or set MONGODB_URI)")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			if verbose {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to %s (timeout %s)...\n", uri, timeout)
			}

			inspector, err := mongoinspect.NewInspector(ctx, mongoinspect.Config{
				URI:      uri,
				Database: database,
			})
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			defer func() { _ = inspector.Close(ctx) }()

			info, err := inspector.GetServerVersion(ctx)
			if err != nil {
				return fmt.Errorf("server info: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connected to MongoDB %s\n", info.Version)

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

			findings := analyzer.Audit(collections)

			if auditUsers {
				var allUsers []mongoinspect.UserInfo

				// Query admin database for cluster-level users.
				adminUsers, adminErr := inspector.InspectUsers(ctx, "admin")
				if adminErr != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not list admin users: %v\n", adminErr)
				} else {
					allUsers = append(allUsers, adminUsers...)
				}

				// Query each application database.
				dbs, dbsErr := inspector.ListDatabases(ctx, database)
				if dbsErr == nil {
					for _, db := range dbs {
						dbUsers, dbErr := inspector.InspectUsers(ctx, db.Name)
						if dbErr != nil {
							_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not list users on %s: %v\n", db.Name, dbErr)
							continue
						}
						allUsers = append(allUsers, dbUsers...)
					}
				}

				if verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Inspected %d users\n", len(allUsers))
				}

				userFindings := analyzer.AuditUsers(allUsers)
				findings = append(findings, userFindings...)
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

			// Baseline comparison.
			if baseline != "" {
				baselineFindings, blErr := analyzer.LoadBaseline(baseline)
				if blErr != nil {
					return fmt.Errorf("load baseline: %w", blErr)
				}
				diff := analyzer.DiffBaseline(findings, baselineFindings)
				reporter.WriteBaselineDiff(cmd.OutOrStdout(), diff)
			}

			report := reporter.NewReport(findings)
			report.Metadata = reporter.Metadata{
				Version:        version,
				Command:        "audit",
				Database:       database,
				MongoDBVersion: info.Version,
				URIHash:        reporter.HashURI(uri),
			}

			if err := reporter.Write(cmd.OutOrStdout(), &report, reporter.Format(format)); err != nil {
				return fmt.Errorf("write report: %w", err)
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

	return cmd
}
