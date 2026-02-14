package cli

import (
	"context"
	"fmt"
	"os"
	"time"

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
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Compare code repo collection references against live MongoDB",
		RunE: func(cmd *cobra.Command, args []string) error {
			if uri == "" {
				return fmt.Errorf("--uri is required")
			}
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
			defer cancel()

			// Scan code repo
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Scanning repo %s...\n", repo)
			scan, err := scanner.Scan(repo)
			if err != nil {
				return fmt.Errorf("scan repo: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Found %d collection references in %d files\n",
				len(scan.Refs), scan.FilesScanned)

			// Connect to MongoDB
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
			report := reporter.NewReport(findings)

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
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.Flags().BoolVar(&failOnMissing, "fail-on-missing", false, "exit 2 if any MISSING_COLLECTION found")

	return cmd
}
