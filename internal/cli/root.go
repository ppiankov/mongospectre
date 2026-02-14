package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	version string
	uri     string
	verbose bool
	timeout time.Duration
)

func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "mongospectre",
		Short: "MongoDB collection and index auditor",
		Long:  "Scans codebases for collection/field references, compares with live MongoDB schema and statistics, detects drift.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if uri == "" {
				uri = os.Getenv("MONGODB_URI")
			}
		},
	}

	root.PersistentFlags().StringVar(&uri, "uri", "", "MongoDB connection URI (env: MONGODB_URI)")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	root.PersistentFlags().DurationVar(&timeout, "timeout", 30*time.Second, "operation timeout")

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newAuditCmd())
	root.AddCommand(newCheckCmd())

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
