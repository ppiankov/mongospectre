package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/ppiankov/mongospectre/internal/scanner"
	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	var (
		repo          string
		database      string
		format        string
		failOnMissing bool
		noIgnore      bool
		baseline      string
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Compare code repo collection references against live MongoDB",
		RunE: func(cmd *cobra.Command, args []string) error {
			if uri == "" {
				return fmt.Errorf("--uri is required (or set MONGODB_URI)")
			}
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			// Scan code repo
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Scanning repo %s...\n", repo)
			scan, err := scanner.Scan(repo)
			if err != nil {
				return fmt.Errorf("scan repo: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Found %d collection references in %d files\n",
				len(scan.Refs), scan.FilesScanned)

			if verbose {
				for _, c := range scan.Collections {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  collection: %s\n", c)
				}
				if scan.FilesSkipped > 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  skipped %d unreadable files\n", scan.FilesSkipped)
				}
			}

			// Connect to MongoDB
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

			// Run diff
			findings := analyzer.Diff(scan, collections)

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
				Command:        "check",
				Database:       database,
				MongoDBVersion: info.Version,
				RepoPath:       repo,
			}

			if err := reporter.Write(cmd.OutOrStdout(), report, reporter.Format(format)); err != nil {
				return fmt.Errorf("write report: %w", err)
			}

			if failOnMissing {
				for _, f := range findings {
					if f.Type == analyzer.FindingMissingCollection {
						os.Exit(2)
					}
				}
			}

			code := analyzer.ExitCode(report.MaxSeverity)
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "path to code repository to scan")
	cmd.Flags().StringVar(&database, "database", "", "specific database to check (default: all non-system)")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "output format: text or json")
	cmd.Flags().BoolVar(&failOnMissing, "fail-on-missing", false, "exit 2 if any MISSING_COLLECTION found")
	cmd.Flags().BoolVar(&noIgnore, "no-ignore", false, "bypass .mongospectreignore file")
	cmd.Flags().StringVar(&baseline, "baseline", "", "path to previous JSON report for diff comparison")

	return cmd
}
