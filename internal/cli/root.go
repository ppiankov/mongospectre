package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var uri string

func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "mongospectre",
		Short: "MongoDB collection and index auditor",
		Long:  "Scans codebases for collection/field references, compares with live MongoDB schema and statistics, detects drift.",
	}

	root.PersistentFlags().StringVar(&uri, "uri", "", "MongoDB connection URI")

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newAuditCmd())

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
func Execute(version string) error {
	return newRootCmd(version).Execute()
}
