package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/notify"
	"github.com/spf13/cobra"
)

type fakeWatchNotifier struct {
	mu     sync.Mutex
	events []notify.Event
	err    error
}

func (f *fakeWatchNotifier) Notify(_ context.Context, events []notify.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, events...)
	return f.err
}

func TestWatcherRunExitOnNewHigh(t *testing.T) {
	prevTimeout := timeout
	t.Cleanup(func() { timeout = prevTimeout })
	timeout = time.Second

	first := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "baseline_medium", DocCount: 0, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	second := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "orders", DocCount: 20000, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}

	call := 0
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		call++
		if call == 1 {
			return first, nil
		}
		return second, nil
	})

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	w := &watcher{
		uri:       "mongodb://stub",
		interval:  10 * time.Millisecond,
		format:    "text",
		exitOnNew: true,
		cmd:       cmd,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := w.run(ctx)
	requireExitCode(t, err, 2)
	if !strings.Contains(stderr.String(), "New high-severity finding detected, exiting") {
		t.Fatalf("expected exit-on-new log, got: %q", stderr.String())
	}
}

func TestWatcherRunJSONEmitsFullDiffShutdown(t *testing.T) {
	prevTimeout := timeout
	t.Cleanup(func() { timeout = prevTimeout })
	timeout = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	first := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "baseline_one", DocCount: 0, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	second := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "baseline_two", DocCount: 0, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
		inspectHook: func(string) {
			cancel()
		},
	}

	call := 0
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		call++
		if call == 1 {
			return first, nil
		}
		return second, nil
	})

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	w := &watcher{
		uri:      "mongodb://stub",
		interval: 10 * time.Millisecond,
		format:   "json",
		cmd:      cmd,
	}

	if err := w.run(ctx); err != nil {
		t.Fatalf("watch run returned error: %v", err)
	}

	out := stdout.String()
	for _, eventType := range []string{`"type":"full"`, `"type":"diff"`, `"type":"shutdown"`} {
		if !strings.Contains(out, eventType) {
			t.Fatalf("missing %s in output: %q", eventType, out)
		}
	}
	if !strings.Contains(stderr.String(), "Watch summary:") {
		t.Fatalf("expected watch summary on stderr, got: %q", stderr.String())
	}
}

func TestWatcherRunVerboseNoChanges(t *testing.T) {
	prevTimeout := timeout
	t.Cleanup(func() { timeout = prevTimeout })
	timeout = time.Second
	verbose = true
	defer func() { verbose = false }()

	ctx, cancel := context.WithCancel(context.Background())
	first := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "same_collection", DocCount: 0, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	second := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "same_collection", DocCount: 0, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
		inspectHook: func(string) {
			cancel()
		},
	}

	call := 0
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		call++
		if call == 1 {
			return first, nil
		}
		return second, nil
	})

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	w := &watcher{
		uri:      "mongodb://stub",
		interval: 10 * time.Millisecond,
		format:   "text",
		cmd:      cmd,
	}

	if err := w.run(ctx); err != nil {
		t.Fatalf("watch run returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "no changes") {
		t.Fatalf("expected verbose no-changes heartbeat, got: %q", stderr.String())
	}
}

func TestWatcherRunAuditRespectsIgnoreFile(t *testing.T) {
	prevTimeout := timeout
	t.Cleanup(func() { timeout = prevTimeout })
	timeout = time.Second

	dir := t.TempDir()
	ignorePath := filepath.Join(dir, ".mongospectreignore")
	if err := os.WriteFile(ignorePath, []byte("UNUSED_COLLECTION app.*\n"), 0o644); err != nil {
		t.Fatalf("write ignore file: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	fake := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "empty", DocCount: 0, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		return fake, nil
	})

	w := &watcher{
		uri:      "mongodb://stub",
		database: "app",
		format:   "text",
		cmd:      &cobra.Command{},
	}

	findings, err := w.runAudit(context.Background())
	if err != nil {
		t.Fatalf("runAudit returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected findings to be suppressed by ignore file, got %d", len(findings))
	}

	w.noIgnore = true
	findings, err = w.runAudit(context.Background())
	if err != nil {
		t.Fatalf("runAudit returned error with --no-ignore: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings when noIgnore=true")
	}
}

func TestWatcherRunSendsNotificationsOnDiff(t *testing.T) {
	prevTimeout := timeout
	t.Cleanup(func() { timeout = prevTimeout })
	timeout = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	first := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "baseline", DocCount: 0, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	second := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "orders", DocCount: 20000, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
		inspectHook: func(string) {
			cancel()
		},
	}

	call := 0
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		call++
		if call == 1 {
			return first, nil
		}
		return second, nil
	})

	fakeNotifier := &fakeWatchNotifier{}
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	w := &watcher{
		uri:      "mongodb://stub",
		interval: 10 * time.Millisecond,
		format:   "text",
		notifier: fakeNotifier,
		cmd:      cmd,
	}

	if err := w.run(ctx); err != nil {
		t.Fatalf("watch run returned error: %v", err)
	}

	fakeNotifier.mu.Lock()
	defer fakeNotifier.mu.Unlock()
	if len(fakeNotifier.events) == 0 {
		t.Fatal("expected notification events on diff")
	}
	foundNewHigh := false
	for _, event := range fakeNotifier.events {
		if event.Type == notify.EventNewHigh {
			foundNewHigh = true
			break
		}
	}
	if !foundNewHigh {
		t.Fatalf("expected at least one %s event, got: %+v", notify.EventNewHigh, fakeNotifier.events)
	}
}

func TestWatcherRunNotificationErrorIsNonFatal(t *testing.T) {
	prevTimeout := timeout
	t.Cleanup(func() { timeout = prevTimeout })
	timeout = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	first := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "baseline", DocCount: 0, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	second := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "orders", DocCount: 20000, Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
		inspectHook: func(string) {
			cancel()
		},
	}

	call := 0
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		call++
		if call == 1 {
			return first, nil
		}
		return second, nil
	})

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	w := &watcher{
		uri:      "mongodb://stub",
		interval: 10 * time.Millisecond,
		format:   "text",
		notifier: &fakeWatchNotifier{err: context.DeadlineExceeded},
		cmd:      cmd,
	}

	if err := w.run(ctx); err != nil {
		t.Fatalf("watch run returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "notification error") {
		t.Fatalf("expected notification error log, got: %q", stderr.String())
	}
}
