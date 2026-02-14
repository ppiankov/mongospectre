package cli

import (
	"context"
	"fmt"
	"time"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/spf13/cobra"
)

func newAuditCmd() *cobra.Command {
	var database string

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
			fmt.Fprintf(cmd.OutOrStdout(), "Connected to MongoDB %s\n", info.Version)

			collections, err := inspector.Inspect(ctx, database)
			if err != nil {
				return fmt.Errorf("inspect: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Found %d collections\n", len(collections))
			for _, c := range collections {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s.%s (%d docs, %d indexes)\n",
					c.Database, c.Name, c.DocCount, len(c.Indexes))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&database, "database", "", "specific database to audit (default: all non-system)")

	return cmd
}
