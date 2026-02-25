package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	var (
		repo          string
		database      string
		format        string
		failOnMissing bool
		profile       bool
		profileLimit  int
		noIgnore      bool
		baseline      string
		interactive   bool
		noInteractive bool
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Compare code repo collection references against live MongoDB",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(format, "text", "json", "sarif", "spectrehub"); err != nil {
				return err
			}
			if profileLimit <= 0 {
				return fmt.Errorf("--profile-limit must be greater than 0")
			}
			if interactive && noInteractive {
				return fmt.Errorf("--interactive and --no-interactive are mutually exclusive")
			}
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
			scan, err := scanRepo(repo)
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

			inspector, err := newInspector(ctx, mongoinspect.Config{
				URI:      uri,
				Database: database,
			})
			if err != nil {
				return err
			}
			defer func() { _ = inspector.Close(ctx) }()

			info, err := inspector.GetServerVersion(ctx)
			if err != nil {
				return fmt.Errorf("server info: %w", err)
			}
			host := reporter.HostFromURI(uri)
			if host != "" {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connected to MongoDB %s at %s\n", info.Version, host)
			} else {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connected to MongoDB %s\n", info.Version)
			}

			collections, err := inspector.Inspect(ctx, database)
			if err != nil {
				return fmt.Errorf("inspect: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Inspected %d collections\n", len(collections))

			if len(collections) == 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Hint: no collections found. Check that the URI points to a database with data, or use --database to specify one.\n")
			}

			validators, err := inspector.GetValidators(ctx, database)
			if err != nil {
				return fmt.Errorf("validators: %w", err)
			}
			collections = mergeCollectionValidators(collections, validators)

			// Run diff
			findings := analyzer.Diff(&scan, collections)
			if profile {
				entries, profileErr := inspector.ReadProfiler(ctx, database, int64(profileLimit))
				if profileErr != nil {
					return fmt.Errorf("read profiler: %w", profileErr)
				}
				if len(entries) == 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"Hint: no profiler entries found in system.profile. Profiler may be disabled; enable with db.setProfilingLevel(1) and rerun with --profile.\n")
				} else {
					findings = append(findings, analyzer.CorrelateProfiler(&scan, entries)...)
				}
			}

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
				Host:           host,
				Database:       database,
				MongoDBVersion: info.Version,
				RepoPath:       repo,
				URIHash:        reporter.HashURI(uri),
			}
			scanCopy := scan
			report.Scan = &scanCopy
			report.Collections = collections

			renderedInteractive, err := maybeRenderInteractive(cmd, &report, collections, &scan, interactiveConfig{
				force:    interactive,
				disable:  noInteractive,
				format:   format,
				findings: len(findings),
			})
			if err != nil {
				return err
			}
			if !renderedInteractive {
				if err := reporter.Write(cmd.OutOrStdout(), &report, reporter.Format(format)); err != nil {
					return fmt.Errorf("write report: %w", err)
				}
			}

			if failOnMissing {
				for _, f := range findings {
					if f.Type == analyzer.FindingMissingCollection {
						return &ExitError{Code: 2}
					}
				}
			}

			code := analyzer.ExitCode(report.MaxSeverity)
			if code != 0 {
				if hint := reporter.ExitCodeHint(code); hint != "" {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), hint)
				}
				return &ExitError{Code: code}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "path to code repository to scan")
	cmd.Flags().StringVar(&database, "database", "", "specific database to check (default: all non-system)")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "output format: text, json, sarif, or spectrehub")
	cmd.Flags().BoolVar(&failOnMissing, "fail-on-missing", false, "exit 2 if any MISSING_COLLECTION found")
	cmd.Flags().BoolVar(&profile, "profile", false, "read system.profile and correlate slow queries to source locations")
	cmd.Flags().IntVar(&profileLimit, "profile-limit", 1000, "maximum number of profiler entries to read")
	cmd.Flags().BoolVar(&noIgnore, "no-ignore", false, "bypass .mongospectreignore file")
	cmd.Flags().StringVar(&baseline, "baseline", "", "path to previous JSON report for diff comparison")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "launch interactive terminal UI (text format only)")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "force non-interactive output")

	return cmd
}

func mergeCollectionValidators(collections []mongoinspect.CollectionInfo, validators []mongoinspect.ValidatorInfo) []mongoinspect.CollectionInfo {
	validatorByCollection := make(map[string]mongoinspect.ValidatorInfo, len(validators))
	for _, v := range validators {
		key := strings.ToLower(v.Database + "." + v.Collection)
		validatorByCollection[key] = v
	}

	for i := range collections {
		key := strings.ToLower(collections[i].Database + "." + collections[i].Name)
		if v, ok := validatorByCollection[key]; ok {
			vCopy := v
			collections[i].Validator = &vCopy
		}
	}
	return collections
}
