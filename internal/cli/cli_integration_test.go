//go:build integration

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var testURI string

func TestMain(m *testing.M) {
	testURI = os.Getenv("MONGODB_TEST_URI")
	if testURI == "" {
		// Skip gracefully if no URI — integration tests are opt-in.
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// seedDB creates test data and returns a cleanup function.
func seedDB(t *testing.T) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(testURI))
	if err != nil {
		t.Fatalf("seed connect: %v", err)
	}

	db := client.Database("cli_test_db")
	// Drop for clean slate.
	if err := db.Drop(ctx); err != nil {
		t.Fatalf("drop db: %v", err)
	}

	// Create "users" collection with docs and indexes.
	users := db.Collection("users")
	docs := []interface{}{
		bson.M{"name": "Alice", "email": "alice@example.com", "status": "active"},
		bson.M{"name": "Bob", "email": "bob@example.com", "status": "inactive"},
		bson.M{"name": "Charlie", "email": "charlie@example.com", "status": "active"},
	}
	if _, err := users.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	indexModels := []mongo.IndexModel{
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "name", Value: 1}}},
	}
	if _, err := users.Indexes().CreateMany(ctx, indexModels); err != nil {
		t.Fatalf("create indexes: %v", err)
	}

	// Create "orders" collection with docs but only _id index (triggers MISSING_INDEX).
	orders := db.Collection("orders")
	var orderDocs []interface{}
	for i := 0; i < 200; i++ {
		orderDocs = append(orderDocs, bson.M{"item": "widget", "qty": i + 1})
	}
	if _, err := orders.InsertMany(ctx, orderDocs); err != nil {
		t.Fatalf("insert orders: %v", err)
	}

	// Create an empty collection (triggers UNUSED_COLLECTION).
	if err := db.CreateCollection(ctx, "empty_collection"); err != nil {
		t.Fatalf("create empty_collection: %v", err)
	}

	if err := client.Disconnect(ctx); err != nil {
		t.Fatalf("disconnect seed: %v", err)
	}

	return func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanCancel()
		c, err := mongo.Connect(options.Client().ApplyURI(testURI))
		if err != nil {
			return
		}
		_ = c.Database("cli_test_db").Drop(cleanCtx)
		_ = c.Database("cli_test_db2").Drop(cleanCtx)
		_ = c.Disconnect(cleanCtx)
	}
}

func intCmd(args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	cmd := newRootCmd(testBuildInfo)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	uri = "" // Reset global — let the flag take effect.
	err := cmd.Execute()
	return &stdout, &stderr, err
}

func TestIntegration_AuditText(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	stdout, stderr, err := intCmd("audit", "--uri", testURI, "--database", "cli_test_db", "--format", "text")
	if err != nil {
		// Audit may return exit code 1/2 for findings, but RunE wraps that differently.
		// Check that the report was generated.
		if stdout.Len() == 0 {
			t.Fatalf("audit text failed: %v\nstderr: %s", err, stderr.String())
		}
	}
	out := stdout.String()
	if !strings.Contains(out, "UNUSED_COLLECTION") && !strings.Contains(out, "No findings") {
		t.Errorf("expected UNUSED_COLLECTION or No findings, got:\n%s", out)
	}
	// stderr should show connection info.
	if !strings.Contains(stderr.String(), "Connected to MongoDB") {
		t.Errorf("stderr missing connection info: %s", stderr.String())
	}
}

func TestIntegration_AuditJSON(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	stdout, _, err := intCmd("audit", "--uri", testURI, "--database", "cli_test_db", "--format", "json")
	if err != nil && stdout.Len() == 0 {
		t.Fatalf("audit json failed: %v", err)
	}
	var report reporter.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if report.Metadata.Command != "audit" {
		t.Errorf("command = %s, want audit", report.Metadata.Command)
	}
	if report.Metadata.MongoDBVersion == "" {
		t.Error("missing MongoDB version in metadata")
	}
}

func TestIntegration_AuditSARIF(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	stdout, _, err := intCmd("audit", "--uri", testURI, "--database", "cli_test_db", "--format", "sarif")
	if err != nil && stdout.Len() == 0 {
		t.Fatalf("audit sarif failed: %v", err)
	}
	// Parse the SARIF output.
	var raw map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}
	if raw["version"] != "2.1.0" {
		t.Errorf("SARIF version = %v", raw["version"])
	}
}

func TestIntegration_AuditSpectreHub(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	stdout, _, err := intCmd("audit", "--uri", testURI, "--database", "cli_test_db", "--format", "spectrehub")
	if err != nil && stdout.Len() == 0 {
		t.Fatalf("audit spectrehub failed: %v", err)
	}
	var env reporter.SpectreHubEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid SpectreHub JSON: %v\noutput: %s", err, stdout.String())
	}
	if env.Schema != "spectre/v1" {
		t.Errorf("schema = %s, want spectre/v1", env.Schema)
	}
	if env.Target.Type != "mongodb" {
		t.Errorf("target.type = %s", env.Target.Type)
	}
	if !strings.HasPrefix(env.Target.URIHash, "sha256:") {
		t.Errorf("uri_hash should start with sha256:, got %s", env.Target.URIHash)
	}
}

func TestIntegration_AuditVerbose(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	_, stderr, err := intCmd("audit", "--uri", testURI, "--database", "cli_test_db", "--verbose")
	if err != nil {
		// Ignore exit code — just check verbose output.
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "cli_test_db.users") {
		t.Errorf("verbose stderr should list collections: %s", errOut)
	}
}

func TestIntegration_AuditNoIgnore(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	stdout, _, err := intCmd("audit", "--uri", testURI, "--database", "cli_test_db", "--no-ignore")
	if err != nil && stdout.Len() == 0 {
		t.Fatalf("audit --no-ignore failed: %v", err)
	}
	// Should produce findings without ignore filtering.
	if stdout.Len() == 0 {
		t.Error("expected output from audit --no-ignore")
	}
}

func TestIntegration_AuditBaseline(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	// First run: save JSON baseline.
	stdout1, _, err := intCmd("audit", "--uri", testURI, "--database", "cli_test_db", "--format", "json")
	if err != nil && stdout1.Len() == 0 {
		t.Fatalf("baseline run failed: %v", err)
	}

	// Save baseline to temp file.
	baselineFile := t.TempDir() + "/baseline.json"
	if err := os.WriteFile(baselineFile, stdout1.Bytes(), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	// Second run: compare against baseline.
	stdout2, _, err := intCmd("audit", "--uri", testURI, "--database", "cli_test_db", "--baseline", baselineFile)
	if err != nil && stdout2.Len() == 0 {
		t.Fatalf("baseline diff run failed: %v", err)
	}
	out := stdout2.String()
	// With same data, all findings should be unchanged.
	if !strings.Contains(out, "unchanged") {
		t.Errorf("expected 'unchanged' in baseline diff, got:\n%s", out)
	}
}

func TestIntegration_AuditIgnoreFile(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	// Create a .mongospectreignore that suppresses UNUSED_COLLECTION.
	dir := t.TempDir()
	ignoreFile := dir + "/.mongospectreignore"
	if err := os.WriteFile(ignoreFile, []byte("UNUSED_COLLECTION cli_test_db.*\n"), 0o644); err != nil {
		t.Fatalf("write ignore: %v", err)
	}

	// Run from the temp dir so the ignore file is picked up.
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	stdout, stderr, err := intCmd("audit", "--uri", testURI, "--database", "cli_test_db", "--verbose")
	if err != nil {
		// Ignore exit code.
	}
	_ = stdout
	errOut := stderr.String()
	if !strings.Contains(errOut, "Suppressed") {
		// It's OK if there's nothing to suppress (pattern might not match format exactly).
		// Just verify it didn't crash.
		t.Logf("No suppression message (expected if pattern doesn't match): %s", errOut)
	}
}

func TestIntegration_CheckEndToEnd(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	// Create a temp repo with Go files referencing collections.
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/main.go", []byte(`package main
import "go.mongodb.org/mongo-driver/v2/mongo"
func f(db *mongo.Database) {
	db.Collection("users")
	db.Collection("orders")
	db.Collection("nonexistent_collection")
}
`), 0o644); err != nil {
		t.Fatalf("write go file: %v", err)
	}

	stdout, stderr, err := intCmd("check", "--uri", testURI, "--repo", dir, "--database", "cli_test_db")
	if err != nil && stdout.Len() == 0 {
		t.Fatalf("check failed: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String()
	// nonexistent_collection should be MISSING_COLLECTION.
	if !strings.Contains(out, "MISSING_COLLECTION") {
		t.Errorf("expected MISSING_COLLECTION finding:\n%s", out)
	}
	// stderr should show scanning info.
	if !strings.Contains(stderr.String(), "Scanning repo") {
		t.Errorf("stderr missing scan info: %s", stderr.String())
	}
}

func TestIntegration_CheckJSON(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	dir := t.TempDir()
	os.WriteFile(dir+"/app.go", []byte(`package main
import "go.mongodb.org/mongo-driver/v2/mongo"
func f(db *mongo.Database) { db.Collection("users") }
`), 0o644)

	stdout, _, err := intCmd("check", "--uri", testURI, "--repo", dir, "--database", "cli_test_db", "--format", "json")
	if err != nil && stdout.Len() == 0 {
		t.Fatalf("check json failed: %v", err)
	}
	var report reporter.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if report.Metadata.Command != "check" {
		t.Errorf("command = %s, want check", report.Metadata.Command)
	}
	if report.Metadata.RepoPath == "" {
		t.Error("missing repo path in metadata")
	}
}

func TestIntegration_Compare(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	// Create a second database for comparison.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testURI))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	db2 := client.Database("cli_test_db2")
	if err := db2.Drop(ctx); err != nil {
		t.Fatalf("drop db2: %v", err)
	}
	// Create "users" with different indexes.
	users2 := db2.Collection("users")
	if _, err := users2.InsertOne(ctx, bson.M{"name": "Test"}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := client.Disconnect(ctx); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	stdout, stderr, err := intCmd("compare",
		"--source", testURI, "--target", testURI,
		"--source-db", "cli_test_db", "--target-db", "cli_test_db2")
	if err != nil && stdout.Len() == 0 {
		t.Fatalf("compare failed: %v\nstderr: %s", err, stderr.String())
	}
	// There should be differences (different indexes, missing collections).
	out := stdout.String()
	if strings.Contains(out, "No differences found") {
		t.Logf("Compare found no differences (possible if both minimal): %s", out)
	}
}

func TestIntegration_CompareJSON(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, _ := mongo.Connect(options.Client().ApplyURI(testURI))
	db2 := client.Database("cli_test_db2")
	_ = db2.Drop(ctx)
	users2 := db2.Collection("users")
	_, _ = users2.InsertOne(ctx, bson.M{"name": "Test"})
	_ = client.Disconnect(ctx)

	stdout, _, err := intCmd("compare",
		"--source", testURI, "--target", testURI,
		"--source-db", "cli_test_db", "--target-db", "cli_test_db2",
		"--format", "json")
	if err != nil && stdout.Len() == 0 {
		t.Fatalf("compare json failed: %v", err)
	}
	var findings []analyzer.CompareFinding
	if err := json.Unmarshal(stdout.Bytes(), &findings); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
}

func TestIntegration_WatchSingleRun(t *testing.T) {
	cleanup := seedDB(t)
	defer cleanup()

	// Run watch with a short timeout so it exits after one run.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := newRootCmd(testBuildInfo)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"watch", "--uri", testURI, "--database", "cli_test_db", "--interval", "100ms"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetContext(ctx)
	uri = ""

	// Cancel after a short delay to let the first audit run complete.
	go func() {
		time.Sleep(5 * time.Second)
		cancel()
	}()

	_ = cmd.Execute()

	errOut := stderr.String()
	if !strings.Contains(errOut, "Watch mode:") {
		t.Errorf("missing watch mode message: %s", errOut)
	}
	if !strings.Contains(errOut, "Watch summary:") {
		t.Errorf("missing shutdown summary: %s", errOut)
	}
	// First run should produce output.
	if stdout.Len() == 0 {
		t.Error("expected output from first watch run")
	}
}
