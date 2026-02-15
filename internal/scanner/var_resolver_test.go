package scanner

import "testing"

func TestCollectStringVars_Go(t *testing.T) {
	lines := []joinedLine{
		{text: `const usersCollection = "users"`, lineNum: 1},
		{text: `var ordersTable = "orders"`, lineNum: 2},
		{text: `const x int = 42`, lineNum: 3},
	}
	vars := collectStringVars(lines)
	if vars["usersCollection"] != "users" {
		t.Errorf("usersCollection = %q, want %q", vars["usersCollection"], "users")
	}
	if vars["ordersTable"] != "orders" {
		t.Errorf("ordersTable = %q, want %q", vars["ordersTable"], "orders")
	}
	if _, ok := vars["x"]; ok {
		t.Error("int constant should not be captured")
	}
}

func TestCollectStringVars_JS(t *testing.T) {
	lines := []joinedLine{
		{text: `const USERS = 'users';`, lineNum: 1},
		{text: `let orders = "orders";`, lineNum: 2},
		{text: `var products = "products";`, lineNum: 3},
	}
	vars := collectStringVars(lines)
	if vars["USERS"] != "users" {
		t.Errorf("USERS = %q", vars["USERS"])
	}
	if vars["orders"] != "orders" {
		t.Errorf("orders = %q", vars["orders"])
	}
	if vars["products"] != "products" {
		t.Errorf("products = %q", vars["products"])
	}
}

func TestCollectStringVars_Python(t *testing.T) {
	lines := []joinedLine{
		{text: `COLLECTION_NAME = "users"`, lineNum: 1},
		{text: `other = 'orders'`, lineNum: 2},
	}
	vars := collectStringVars(lines)
	if vars["COLLECTION_NAME"] != "users" {
		t.Errorf("COLLECTION_NAME = %q", vars["COLLECTION_NAME"])
	}
	if vars["other"] != "orders" {
		t.Errorf("other = %q", vars["other"])
	}
}

func TestResolveVarCollections_Resolved(t *testing.T) {
	vars := map[string]string{
		"usersCollection": "users",
		"ordersColl":      "orders",
	}

	resolved, dynamic := resolveVarCollections(`coll := db.Collection(usersCollection)`, vars)
	if len(resolved) != 1 || resolved[0].Collection != "users" {
		t.Errorf("resolved = %+v, want [{Collection:users}]", resolved)
	}
	if len(dynamic) != 0 {
		t.Errorf("dynamic = %v, want empty", dynamic)
	}
}

func TestResolveVarCollections_Dynamic(t *testing.T) {
	vars := map[string]string{}

	resolved, dynamic := resolveVarCollections(`coll := db.Collection(unknownVar)`, vars)
	if len(resolved) != 0 {
		t.Errorf("resolved = %+v, want empty", resolved)
	}
	if len(dynamic) != 1 || dynamic[0] != "unknownVar" {
		t.Errorf("dynamic = %v, want [unknownVar]", dynamic)
	}
}

func TestResolveVarCollections_JS(t *testing.T) {
	vars := map[string]string{"COLL": "products"}

	resolved, dynamic := resolveVarCollections(`const c = db.collection(COLL);`, vars)
	if len(resolved) != 1 || resolved[0].Collection != "products" {
		t.Errorf("resolved = %+v", resolved)
	}
	if len(dynamic) != 0 {
		t.Errorf("dynamic = %v", dynamic)
	}
}

func TestResolveVarCollections_NoMatch(t *testing.T) {
	vars := map[string]string{"x": "y"}

	// String literal — should not match var patterns.
	resolved, dynamic := resolveVarCollections(`db.Collection("users")`, vars)
	if len(resolved) != 0 {
		t.Errorf("resolved = %+v, want empty for string literal", resolved)
	}
	if len(dynamic) != 0 {
		t.Errorf("dynamic = %v, want empty for string literal", dynamic)
	}
}

func TestScan_VarResolution(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "varref.go", `package main

import "go.mongodb.org/mongo-driver/v2/mongo"

const usersCollection = "users"

func run(db *mongo.Database) {
	coll := db.Collection(usersCollection)
	_ = coll
}
`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	collSet := make(map[string]bool)
	for _, c := range result.Collections {
		collSet[c] = true
	}

	if !collSet["users"] {
		t.Errorf("expected 'users' via variable resolution, got collections: %v", result.Collections)
	}
}

func TestScan_DynamicRef(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "dynamic.go", `package main

import "go.mongodb.org/mongo-driver/v2/mongo"

func run(db *mongo.Database, name string) {
	coll := db.Collection(name)
	_ = coll
}
`)

	result, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.DynamicRefs) != 1 {
		t.Fatalf("expected 1 dynamic ref, got %d", len(result.DynamicRefs))
	}
	if result.DynamicRefs[0].Variable != "name" {
		t.Errorf("variable = %q, want %q", result.DynamicRefs[0].Variable, "name")
	}
}
