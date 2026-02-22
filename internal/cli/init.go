package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create starter .mongospectre.yml and .mongospectreignore in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getwd: %w", err)
			}

			wrote := 0
			for _, f := range initFiles {
				path := filepath.Join(cwd, f.name)
				if _, err := os.Stat(path); err == nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "skip: %s already exists\n", f.name)
					continue
				}
				if err := os.WriteFile(path, []byte(f.content), 0o600); err != nil {
					return fmt.Errorf("write %s: %w", f.name, err)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", f.name)
				wrote++
			}

			if wrote == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Nothing to do — all config files already exist.")
			}
			return nil
		},
	}
	return cmd
}

type initFile struct {
	name    string
	content string
}

var initFiles = []initFile{
	{
		name: ".mongospectre.yml",
		content: `# mongospectre configuration
# See: https://github.com/ppiankov/mongospectre

# MongoDB connection URI (overridden by --uri flag or MONGODB_URI env var)
# uri: mongodb://localhost:27017

# Restrict audit to a specific database (default: all non-system databases)
# database: myapp

thresholds:
  oversized_docs: 1000000
  index_usage_days: 30

exclude:
  databases: []
  collections: []

defaults:
  format: text
  verbose: false
  timeout: 30s

# Optional watch notifications (used by: mongospectre watch --notify)
# notifications:
#   - type: slack
#     webhook_url: ${SLACK_WEBHOOK_URL}
#     on: [new_high, new_medium]
#   - type: webhook
#     url: https://alerts.example.com/mongospectre
#     method: POST
#     headers:
#       Authorization: "Bearer ${ALERT_TOKEN}"
#     on: [new_high]
#   - type: email
#     smtp_host: smtp.gmail.com
#     smtp_port: 587
#     from: alerts@example.com
#     to: ["team@example.com"]
#     smtp_username: ${SMTP_USERNAME}
#     smtp_password: ${SMTP_PASSWORD}
#     on: [new_high, resolved]
`,
	},
	{
		name: ".mongospectreignore",
		content: `# mongospectre ignore rules
# Format: TYPE db.collection[.index]
#   TYPE  — finding type (e.g. UNUSED_INDEX) or * for any
#   db    — database name or * for any
#   collection — collection name (supports trailing * glob)
#   index — optional index name
#
# Examples:
# UNUSED_INDEX app.users.idx_old
# * *.audit_logs
# MISSING_TTL app.settings
`,
	},
}
