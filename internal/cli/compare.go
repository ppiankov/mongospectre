package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/spf13/cobra"
)

func newCompareCmd() *cobra.Command {
	var (
		sourceURI string
		targetURI string
		sourceDB  string
		targetDB  string
		format    string
	)

	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare schemas across two MongoDB clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			if sourceURI == "" {
				return fmt.Errorf("--source is required")
			}
			if targetURI == "" {
				return fmt.Errorf("--target is required")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			// Connect to source.
			if verbose {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to source %s...\n", sourceURI)
			}
			sourceInspector, err := mongoinspect.NewInspector(ctx, mongoinspect.Config{
				URI:      sourceURI,
				Database: sourceDB,
			})
			if err != nil {
				return fmt.Errorf("connect source: %w", err)
			}
			defer func() { _ = sourceInspector.Close(ctx) }()

			sourceColls, err := sourceInspector.Inspect(ctx, sourceDB)
			if err != nil {
				return fmt.Errorf("inspect source: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Source: %d collections\n", len(sourceColls))

			// Connect to target.
			if verbose {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to target %s...\n", targetURI)
			}
			targetInspector, err := mongoinspect.NewInspector(ctx, mongoinspect.Config{
				URI:      targetURI,
				Database: targetDB,
			})
			if err != nil {
				return fmt.Errorf("connect target: %w", err)
			}
			defer func() { _ = targetInspector.Close(ctx) }()

			targetColls, err := targetInspector.Inspect(ctx, targetDB)
			if err != nil {
				return fmt.Errorf("inspect target: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Target: %d collections\n", len(targetColls))

			// Compare.
			findings := analyzer.Compare(sourceColls, targetColls)

			switch format {
			case "json":
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(findings); err != nil {
					return fmt.Errorf("write json: %w", err)
				}
			default:
				writeCompareText(cmd, findings)
			}

			// Exit code based on severity.
			maxSev := analyzer.SeverityInfo
			sevOrder := map[analyzer.Severity]int{
				analyzer.SeverityInfo:   0,
				analyzer.SeverityLow:    1,
				analyzer.SeverityMedium: 2,
				analyzer.SeverityHigh:   3,
			}
			for i := range findings {
				if sevOrder[findings[i].Severity] > sevOrder[maxSev] {
					maxSev = findings[i].Severity
				}
			}
			code := analyzer.ExitCode(maxSev)
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceURI, "source", "", "source MongoDB connection URI")
	cmd.Flags().StringVar(&targetURI, "target", "", "target MongoDB connection URI")
	cmd.Flags().StringVar(&sourceDB, "source-db", "", "specific database in source (default: all)")
	cmd.Flags().StringVar(&targetDB, "target-db", "", "specific database in target (default: all)")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "output format: text or json")

	return cmd
}

func writeCompareText(cmd *cobra.Command, findings []analyzer.CompareFinding) {
	if len(findings) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No differences found.")
		return
	}

	sevLabel := map[analyzer.Severity]string{
		analyzer.SeverityHigh:   "HIGH",
		analyzer.SeverityMedium: "MEDIUM",
		analyzer.SeverityLow:    "LOW",
		analyzer.SeverityInfo:   "INFO",
	}

	for i := range findings {
		loc := findings[i].Database + "." + findings[i].Collection
		if findings[i].Index != "" {
			loc += "." + findings[i].Index
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s (%s)\n",
			sevLabel[findings[i].Severity], findings[i].Type, findings[i].Message, loc)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n%d differences found\n", len(findings))
}
