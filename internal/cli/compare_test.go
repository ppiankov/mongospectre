package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

func TestCompareDatabaseScopeAndJSON(t *testing.T) {
	source := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "srcdb", Name: "users", Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	target := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "tgtdb", Name: "users", Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}

	var cfgs []mongoinspect.Config
	call := 0
	stubNewInspector(t, func(_ context.Context, cfg mongoinspect.Config) (inspector, error) {
		cfgs = append(cfgs, cfg)
		call++
		switch call {
		case 1:
			return source, nil
		case 2:
			return target, nil
		default:
			t.Fatalf("unexpected inspector creation #%d", call)
			return nil, nil
		}
	})

	stdout, _, err := execCLI(t, "compare",
		"--source", "mongodb://source",
		"--target", "mongodb://target",
		"--source-db", "srcdb",
		"--target-db", "tgtdb",
		"--format", "json",
		"--timeout", "1s",
	)
	if err != nil {
		t.Fatalf("compare returned error: %v", err)
	}

	if len(cfgs) != 2 {
		t.Fatalf("expected two inspector configs, got %d", len(cfgs))
	}
	if cfgs[0].Database != "srcdb" || cfgs[1].Database != "tgtdb" {
		t.Fatalf("unexpected inspector config databases: %#v", cfgs)
	}
	if len(source.inspectCalls) != 1 || source.inspectCalls[0] != "srcdb" {
		t.Fatalf("source Inspect called with %v, want [srcdb]", source.inspectCalls)
	}
	if len(target.inspectCalls) != 1 || target.inspectCalls[0] != "tgtdb" {
		t.Fatalf("target Inspect called with %v, want [tgtdb]", target.inspectCalls)
	}

	var findings []analyzer.CompareFinding
	if err := json.Unmarshal([]byte(stdout), &findings); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %d", len(findings))
	}
}

func TestCompareExitCodeHigh(t *testing.T) {
	source := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "users", Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}
	target := &fakeInspector{}

	call := 0
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		call++
		if call == 1 {
			return source, nil
		}
		return target, nil
	})

	_, stderr, err := execCLI(t, "compare", "--source", "mongodb://source", "--target", "mongodb://target", "--timeout", "1s")
	requireExitCode(t, err, 2)
	if !strings.Contains(stderr, "Exit 2: high-severity findings detected") {
		t.Fatalf("expected high-severity exit hint, got: %q", stderr)
	}
}

func TestCompareExitCodeMedium(t *testing.T) {
	source := &fakeInspector{}
	target := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "users", Indexes: []mongoinspect.IndexInfo{{Name: "_id_"}}},
		},
	}

	call := 0
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		call++
		if call == 1 {
			return source, nil
		}
		return target, nil
	})

	_, stderr, err := execCLI(t, "compare", "--source", "mongodb://source", "--target", "mongodb://target", "--timeout", "1s")
	requireExitCode(t, err, 1)
	if !strings.Contains(stderr, "Exit 1: medium-severity findings detected") {
		t.Fatalf("expected medium-severity exit hint, got: %q", stderr)
	}
}

func TestCompareInspectTargetError(t *testing.T) {
	source := &fakeInspector{
		inspectResult: []mongoinspect.CollectionInfo{{Database: "app", Name: "users"}},
	}
	target := &fakeInspector{
		inspectErr: errors.New("target inspect failed"),
	}

	call := 0
	stubNewInspector(t, func(context.Context, mongoinspect.Config) (inspector, error) {
		call++
		if call == 1 {
			return source, nil
		}
		return target, nil
	})

	_, _, err := execCLI(t, "compare", "--source", "mongodb://source", "--target", "mongodb://target", "--timeout", "1s")
	if err == nil {
		t.Fatal("expected inspect target error")
	}
	if !strings.Contains(err.Error(), "inspect target") {
		t.Fatalf("unexpected error: %v", err)
	}
}
