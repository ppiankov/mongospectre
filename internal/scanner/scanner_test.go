package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan_MultiLanguage(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "main.go", `package main

import "go.mongodb.org/mongo-driver/v2/mongo"

func run(db *mongo.Database) {
	coll := db.Collection("products")
	_ = coll
	orders := db.Collection("orders")
	_ = orders
}
`)

	writeFile(t, dir, "app.py", `from pymongo import MongoClient

client = MongoClient("mongodb://localhost")
db = client.mydb
db.users.find({"active": True})
db["sessions"].insert_one({"token": "abc"})
`)

	writeFile(t, dir, "models.js", `const mongoose = require('mongoose');

const User = mongoose.model("User", userSchema);
const order = db.collection("orders");
`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.FilesScanned != 3 {
		t.Errorf("files scanned = %d, want 3", result.FilesScanned)
	}

	// Expected collections: products, orders, users, sessions, User
	if len(result.Collections) < 4 {
		t.Errorf("expected at least 4 unique collections, got %d: %v", len(result.Collections), result.Collections)
	}

	collSet := make(map[string]bool)
	for _, c := range result.Collections {
		collSet[c] = true
	}

	for _, want := range []string{"products", "orders", "users", "sessions"} {
		if !collSet[want] {
			t.Errorf("missing expected collection %q in %v", want, result.Collections)
		}
	}
}

func TestScan_SkipsDirs(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "src/app.go", `package main
func f(db interface{}) { db.Collection("real") }
`)
	writeFile(t, dir, "node_modules/lib.js", `db.collection("should_skip")`)
	writeFile(t, dir, "vendor/dep.go", `package dep
func f(db interface{}) { db.Collection("also_skip") }
`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.FilesScanned != 1 {
		t.Errorf("files scanned = %d, want 1 (should skip node_modules and vendor)", result.FilesScanned)
	}

	if len(result.Collections) != 1 || result.Collections[0] != "real" {
		t.Errorf("collections = %v, want [real]", result.Collections)
	}
}

func TestScan_SkipsNonCode(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "readme.md", `db.collection("ignored")`)
	writeFile(t, dir, "data.json", `{"collection": "also_ignored"}`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 0 {
		t.Errorf("files scanned = %d, want 0", result.FilesScanned)
	}
}

func TestScan_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 0 || len(result.Refs) != 0 {
		t.Errorf("expected empty result, got %+v", result)
	}
}

func TestScan_MultiLine(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "multiline.go", `package main

import "go.mongodb.org/mongo-driver/v2/mongo"

func run(db *mongo.Database) {
	coll := db.Collection(
		"users",
	)
	_ = coll
}
`)

	writeFile(t, dir, "multiline.py", `from pymongo import MongoClient

db = client["mydb"]
col = db.collection(
    "orders"
)
`)

	writeFile(t, dir, "multiline.js", `const db = client.db("mydb");
const coll = db.collection(
    "products"
);
`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	collSet := make(map[string]bool)
	for _, c := range result.Collections {
		collSet[c] = true
	}

	for _, want := range []string{"users", "orders", "products"} {
		if !collSet[want] {
			t.Errorf("missing multi-line collection %q in %v", want, result.Collections)
		}
	}
}

func TestJoinContinuationLines_SingleLine(t *testing.T) {
	lines := []string{
		`db.Collection("users")`,
		`db.Collection("orders")`,
	}
	joined := joinContinuationLines(lines)
	if len(joined) != 2 {
		t.Fatalf("expected 2 joined lines, got %d", len(joined))
	}
	if joined[0].lineNum != 1 || joined[1].lineNum != 2 {
		t.Errorf("line numbers: %d, %d", joined[0].lineNum, joined[1].lineNum)
	}
}

func TestJoinContinuationLines_MultiLine(t *testing.T) {
	lines := []string{
		`db.Collection(`,
		`    "users",`,
		`)`,
	}
	joined := joinContinuationLines(lines)
	if len(joined) != 1 {
		t.Fatalf("expected 1 joined line, got %d", len(joined))
	}
	if joined[0].lineNum != 1 {
		t.Errorf("lineNum = %d, want 1", joined[0].lineNum)
	}
	// The joined text should contain "users"
	if !contains(joined[0].text, `"users"`) {
		t.Errorf("joined text should contain \"users\": %s", joined[0].text)
	}
}

func TestJoinContinuationLines_MaxJoin(t *testing.T) {
	// More than maxJoinLines should be capped.
	lines := make([]string, 10)
	lines[0] = "func("
	for i := 1; i < 9; i++ {
		lines[i] = "  arg,"
	}
	lines[9] = ")"

	joined := joinContinuationLines(lines)
	// Should produce at least 2 joined entries due to maxJoinLines cap.
	if len(joined) < 2 {
		t.Errorf("expected at least 2 entries due to maxJoin, got %d", len(joined))
	}
}

func TestJoinContinuationLines_Empty(t *testing.T) {
	joined := joinContinuationLines(nil)
	if len(joined) != 0 {
		t.Errorf("expected 0, got %d", len(joined))
	}
}

func TestParenBalance(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{"balanced", `db.Collection("users")`, 0},
		{"open", `db.Collection(`, 1},
		{"close", `)`, -1},
		{"nested", `func(a, func(b, c))`, 0},
		{"string_with_paren", `"hello(world)"`, 0},
		{"comment_with_paren", `foo( // bar)`, 1},
		{"backtick_string", "`hello(`", 0},
		{"escaped_quote", `"hello\"world"`, 0},
		{"empty", "", 0},
		{"multiple_open", `func(a, func(b,`, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parenBalance(tt.line)
			if got != tt.want {
				t.Errorf("parenBalance(%q) = %d, want %d", tt.line, got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestUniqueCollections(t *testing.T) {
	refs := []CollectionRef{
		{Collection: "users"},
		{Collection: "orders"},
		{Collection: "users"},
		{Collection: "products"},
	}
	unique := uniqueCollections(refs)
	if len(unique) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(unique), unique)
	}
	// Should be sorted
	if unique[0] != "orders" || unique[1] != "products" || unique[2] != "users" {
		t.Errorf("expected sorted [orders products users], got %v", unique)
	}
}
