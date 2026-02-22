package cli

import (
	"strings"
	"testing"
)

func TestExitErrorError(t *testing.T) {
	err := (&ExitError{Code: 2}).Error()
	if err != "exit status 2" {
		t.Fatalf("error() = %q, want %q", err, "exit status 2")
	}
}

func TestFormatValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "audit invalid format",
			args: []string{"audit", "--uri", "mongodb://stub", "--format", "xml"},
		},
		{
			name: "check invalid format",
			args: []string{"check", "--uri", "mongodb://stub", "--repo", t.TempDir(), "--format", "xml"},
		},
		{
			name: "compare invalid format",
			args: []string{"compare", "--source", "mongodb://source", "--target", "mongodb://target", "--format", "xml"},
		},
		{
			name: "watch invalid format",
			args: []string{"watch", "--uri", "mongodb://stub", "--format", "xml"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := execCLI(t, tc.args...)
			if err == nil {
				t.Fatal("expected invalid format error")
			}
			if !strings.Contains(err.Error(), "invalid --format") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
