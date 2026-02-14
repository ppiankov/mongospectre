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
