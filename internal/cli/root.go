package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/ppiankov/mongospectre/internal/config"
	"github.com/spf13/cobra"
)

var (
	version string
	uri     string
	verbose bool
	timeout time.Duration
	cfg     config.Config
)

func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "mongospectre",
		Short: "MongoDB collection and index auditor",
		Long:  "Scans codebases for collection/field references, compares with live MongoDB schema and statistics, detects drift.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Load config file.
			cwd, _ := os.Getwd()
			var err error
			cfg, err = config.Load(cwd)
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}

			// Apply config defaults where CLI flags were not explicitly set.
			if !cmd.Flags().Changed("uri") && uri == "" {
				uri = os.Getenv("MONGODB_URI")
				if uri == "" {
					uri = cfg.URI
				}
			}
			if !cmd.Flags().Changed("verbose") && cfg.Defaults.Verbose {
				verbose = true
			}
			if !cmd.Flags().Changed("timeout") {
				timeout = cfg.TimeoutDuration()
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&uri, "uri", "", "MongoDB connection URI (env: MONGODB_URI)")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	root.PersistentFlags().DurationVar(&timeout, "timeout", 30*time.Second, "operation timeout")

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newAuditCmd())
	root.AddCommand(newCheckCmd())
	root.AddCommand(newCompareCmd())

	return root
}

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("mongospectre " + version)
		},
	}
}

// Execute runs the root command.
func Execute(v string) error {
	version = v
	return newRootCmd(v).Execute()
}
