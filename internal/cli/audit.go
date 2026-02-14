package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/spf13/cobra"
)

func newAuditCmd() *cobra.Command {
	var (
		database string
		format   string
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit MongoDB cluster for unused collections, indexes, and drift",
		RunE: func(cmd *cobra.Command, args []string) error {
			if uri == "" {
				return fmt.Errorf("--uri is required")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			inspector, err := mongoinspect.NewInspector(ctx, mongoinspect.Config{
				URI:      uri,
				Database: database,
			})
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			defer inspector.Close(ctx)

			info, err := inspector.GetServerVersion(ctx)
			if err != nil {
				return fmt.Errorf("server info: %w", err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Connected to MongoDB %s\n", info.Version)

			collections, err := inspector.Inspect(ctx, database)
			if err != nil {
				return fmt.Errorf("inspect: %w", err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Inspected %d collections\n", len(collections))

			findings := analyzer.Audit(collections)
			report := reporter.NewReport(findings)

			if err := reporter.Write(cmd.OutOrStdout(), report, reporter.Format(format)); err != nil {
				return fmt.Errorf("write report: %w", err)
			}

			code := analyzer.ExitCode(report.MaxSeverity)
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&database, "database", "", "specific database to audit (default: all non-system)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")

	return cmd
}
