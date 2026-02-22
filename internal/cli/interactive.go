package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/ppiankov/mongospectre/internal/scanner"
	"github.com/ppiankov/mongospectre/internal/tui"
	"github.com/spf13/cobra"
)

const interactiveAutoThreshold = 20

type interactiveConfig struct {
	force    bool
	disable  bool
	format   string
	findings int
}

type interactiveDecision struct {
	run    bool
	reason string
}

var (
	launchInteractiveUI = func(report *reporter.Report, collections []mongoinspect.CollectionInfo, scan *scanner.ScanResult) error {
		return tui.Run(&tui.Input{
			Report:      *report,
			Collections: collections,
			Scan:        scan,
		})
	}
	commandHasTTY = func(cmd *cobra.Command) bool {
		return isTTYWriter(cmd.OutOrStdout()) && isTTYFile(os.Stdin)
	}
	terminalSupportsInteractive = func() bool {
		termName := strings.TrimSpace(strings.ToLower(os.Getenv("TERM")))
		return termName != "" && termName != "dumb"
	}
)

func maybeRenderInteractive(cmd *cobra.Command, report *reporter.Report, collections []mongoinspect.CollectionInfo, scan *scanner.ScanResult, cfg interactiveConfig) (bool, error) {
	decision := decideInteractive(cfg, commandHasTTY(cmd), terminalSupportsInteractive())
	if !decision.run {
		if cfg.force && decision.reason != "" {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "interactive mode skipped: %s\n", decision.reason)
		}
		return false, nil
	}

	if err := launchInteractiveUI(report, collections, scan); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: interactive UI unavailable (%v), falling back to text output\n", err)
		return false, nil
	}
	return true, nil
}

func decideInteractive(cfg interactiveConfig, tty bool, supported bool) interactiveDecision {
	if cfg.disable {
		return interactiveDecision{run: false}
	}

	if cfg.format != "text" {
		return interactiveDecision{
			run:    false,
			reason: "--interactive requires --format text",
		}
	}

	shouldRun := cfg.force || cfg.findings > interactiveAutoThreshold
	if !shouldRun {
		return interactiveDecision{run: false}
	}

	if !tty {
		return interactiveDecision{
			run:    false,
			reason: "stdout is not a terminal",
		}
	}

	if !supported {
		return interactiveDecision{
			run:    false,
			reason: "terminal does not support TUI mode",
		}
	}

	return interactiveDecision{run: true}
}

func isTTYWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isTTYFile(f)
}

func isTTYFile(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
