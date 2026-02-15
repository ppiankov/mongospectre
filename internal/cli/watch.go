package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/spf13/cobra"
)

func newWatchCmd() *cobra.Command {
	var (
		database  string
		interval  time.Duration
		format    string
		exitOnNew bool
		noIgnore  bool
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Continuously monitor a MongoDB cluster and report drift",
		Long:  "Runs audit on a configurable interval, compares each run against the previous, and prints only new/resolved findings.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if uri == "" {
				return fmt.Errorf("--uri is required (or set MONGODB_URI)")
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			// Handle SIGINT/SIGTERM for clean shutdown.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				cancel()
			}()

			w := &watcher{
				uri:       uri,
				database:  database,
				interval:  interval,
				format:    format,
				exitOnNew: exitOnNew,
				noIgnore:  noIgnore,
				cmd:       cmd,
			}
			return w.run(ctx)
		},
	}

	cmd.Flags().StringVar(&database, "database", "", "specific database to watch (default: all non-system)")
	cmd.Flags().DurationVar(&interval, "interval", 5*time.Minute, "time between audit runs")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "output format: text or json (NDJSON)")
	cmd.Flags().BoolVar(&exitOnNew, "exit-on-new", false, "exit with code 2 on first new high-severity finding")
	cmd.Flags().BoolVar(&noIgnore, "no-ignore", false, "bypass .mongospectreignore file")

	return cmd
}

type watcher struct {
	uri       string
	database  string
	interval  time.Duration
	format    string
	exitOnNew bool
	noIgnore  bool
	cmd       *cobra.Command
}

// watchEvent is a single NDJSON event emitted in JSON format.
type watchEvent struct {
	Timestamp string                     `json:"timestamp"`
	Type      string                     `json:"type"` // "full", "diff", "shutdown"
	Findings  []analyzer.Finding         `json:"findings,omitempty"`
	Diff      []analyzer.BaselineFinding `json:"diff,omitempty"`
	Summary   watchSummary               `json:"summary"`
}

type watchSummary struct {
	Total    int `json:"total"`
	New      int `json:"new"`
	Resolved int `json:"resolved"`
}

func (w *watcher) run(ctx context.Context) error {
	stderr := w.cmd.ErrOrStderr()
	stdout := w.cmd.OutOrStdout()

	_, _ = fmt.Fprintf(stderr, "Watch mode: auditing every %s\n", w.interval)

	var baseline []analyzer.Finding
	runCount := 0
	totalNew := 0
	totalResolved := 0

	for {
		findings, err := w.runAudit(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			_, _ = fmt.Fprintf(stderr, "[%s] audit error: %v\n", time.Now().UTC().Format(time.RFC3339), err)
			goto wait
		}

		runCount++

		if baseline == nil {
			// First run: print full results.
			baseline = findings
			if w.format == "json" {
				w.emitJSON(stdout, &watchEvent{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Type:      "full",
					Findings:  findings,
					Summary:   watchSummary{Total: len(findings)},
				})
			} else {
				_, _ = fmt.Fprintf(stderr, "[%s] Initial audit: %d findings\n",
					time.Now().UTC().Format(time.RFC3339), len(findings))
				report := reporter.NewReport(findings)
				_ = reporter.Write(stdout, &report, reporter.FormatText)
			}
		} else {
			// Subsequent runs: diff against baseline.
			diff := analyzer.DiffBaseline(findings, baseline)
			var newCount, resolvedCount int
			for _, d := range diff {
				switch d.Status {
				case analyzer.StatusNew:
					newCount++
				case analyzer.StatusResolved:
					resolvedCount++
				}
			}
			totalNew += newCount
			totalResolved += resolvedCount

			if newCount > 0 || resolvedCount > 0 {
				if w.format == "json" {
					w.emitJSON(stdout, &watchEvent{
						Timestamp: time.Now().UTC().Format(time.RFC3339),
						Type:      "diff",
						Diff:      diff,
						Summary:   watchSummary{Total: len(findings), New: newCount, Resolved: resolvedCount},
					})
				} else {
					_, _ = fmt.Fprintf(stdout, "[%s]\n", time.Now().UTC().Format(time.RFC3339))
					reporter.WriteBaselineDiff(stdout, diff)
				}

				// Check exit-on-new for high severity.
				if w.exitOnNew && newCount > 0 {
					for _, d := range diff {
						if d.Status == analyzer.StatusNew && d.Severity == analyzer.SeverityHigh {
							_, _ = fmt.Fprintf(stderr, "New high-severity finding detected, exiting\n")
							os.Exit(2)
						}
					}
				}
			} else if verbose {
				_, _ = fmt.Fprintf(stderr, "[%s] no changes (%d findings)\n",
					time.Now().UTC().Format(time.RFC3339), len(findings))
			}

			baseline = findings
		}

	wait:
		select {
		case <-ctx.Done():
			goto shutdown
		case <-time.After(w.interval):
			continue
		}
	}

shutdown:
	_, _ = fmt.Fprintf(stderr, "\nWatch summary: %d runs, %d new findings, %d resolved\n",
		runCount, totalNew, totalResolved)
	if w.format == "json" {
		w.emitJSON(stdout, &watchEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Type:      "shutdown",
			Summary:   watchSummary{Total: len(baseline), New: totalNew, Resolved: totalResolved},
		})
	}
	return nil
}

func (w *watcher) runAudit(ctx context.Context) ([]analyzer.Finding, error) {
	auditCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	inspector, err := mongoinspect.NewInspector(auditCtx, mongoinspect.Config{
		URI:      w.uri,
		Database: w.database,
	})
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = inspector.Close(auditCtx) }()

	collections, err := inspector.Inspect(auditCtx, w.database)
	if err != nil {
		return nil, fmt.Errorf("inspect: %w", err)
	}

	findings := analyzer.Audit(collections)

	if !w.noIgnore {
		cwd, _ := os.Getwd()
		il, ilErr := analyzer.LoadIgnoreFile(cwd)
		if ilErr == nil {
			findings, _ = il.Filter(findings)
		}
	}

	return findings, nil
}

func (w *watcher) emitJSON(stdout interface{ Write([]byte) (int, error) }, event *watchEvent) {
	data, _ := json.Marshal(event)
	_, _ = stdout.Write(data)
	_, _ = stdout.Write([]byte("\n"))
}
