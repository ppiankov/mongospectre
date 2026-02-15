package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(origDir) }()

	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"init"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := out.String()
	if !strings.Contains(output, ".mongospectre.yml") {
		t.Error("output should mention .mongospectre.yml")
	}
	if !strings.Contains(output, ".mongospectreignore") {
		t.Error("output should mention .mongospectreignore")
	}

	// Verify files exist and have content.
	for _, name := range []string{".mongospectre.yml", ".mongospectreignore"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestInitSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(origDir) }()

	// Pre-create .mongospectre.yml with custom content.
	existing := "custom: true\n"
	_ = os.WriteFile(filepath.Join(dir, ".mongospectre.yml"), []byte(existing), 0o644)

	cmd := newRootCmd(testBuildInfo)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"init"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Should skip the existing file.
	if !strings.Contains(errBuf.String(), "skip") {
		t.Error("should report skipping existing file")
	}

	// Existing file content should be preserved.
	data, _ := os.ReadFile(filepath.Join(dir, ".mongospectre.yml"))
	if string(data) != existing {
		t.Errorf("existing file was overwritten: %q", string(data))
	}

	// .mongospectreignore should still be created.
	if !strings.Contains(out.String(), ".mongospectreignore") {
		t.Error("should create .mongospectreignore")
	}
}

func TestInitAllExist(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(origDir) }()

	// Pre-create both files with valid content.
	_ = os.WriteFile(filepath.Join(dir, ".mongospectre.yml"), []byte("uri: mongodb://localhost\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".mongospectreignore"), []byte("# empty\n"), 0o644)

	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"init"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "Nothing to do") {
		t.Error("should report nothing to do when all files exist")
	}
}

func TestInitHelpFlags(t *testing.T) {
	cmd := newRootCmd(testBuildInfo)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"init", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "starter") {
		t.Error("init help should mention starter configs")
	}
}
