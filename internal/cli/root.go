package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
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

// BuildInfo holds version and build metadata.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"goVersion"`
}

func newRootCmd(info BuildInfo) *cobra.Command {
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

	root.AddCommand(newVersionCmd(info))
	root.AddCommand(newAuditCmd())
	root.AddCommand(newCheckCmd())
	root.AddCommand(newCompareCmd())
	root.AddCommand(newWatchCmd())
	root.AddCommand(newInitCmd())

	return root
}

func newVersionCmd(info BuildInfo) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			if jsonOutput {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(info)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "mongospectre %s (commit: %s, built: %s, go: %s)\n",
					info.Version, info.Commit, info.Date, info.GoVersion)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output version as JSON")

	return cmd
}

// Execute runs the root command.
func Execute(v, commit, date string) error {
	version = v
	info := BuildInfo{
		Version:   v,
		Commit:    commit,
		Date:      date,
		GoVersion: runtime.Version(),
	}
	return newRootCmd(info).Execute()
}
