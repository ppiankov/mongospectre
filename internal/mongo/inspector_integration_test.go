//go:build integration

package mongo

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func setupMongoDB(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := mongodb.Run(ctx, "mongo:7")
	if err != nil {
		t.Fatalf("start container: %v", err)
	}

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	// Seed test data.
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	db := client.Database("testdb")
	coll := db.Collection("users")

	// Insert documents.
	docs := []interface{}{
		bson.M{"name": "Alice", "email": "alice@example.com", "status": "active"},
		bson.M{"name": "Bob", "email": "bob@example.com", "status": "inactive"},
		bson.M{"name": "Charlie", "email": "charlie@example.com", "status": "active"},
	}
	if _, err := coll.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Create indexes.
	indexModels := []mongo.IndexModel{
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "name", Value: 1}}},
	}
	if _, err := coll.Indexes().CreateMany(ctx, indexModels); err != nil {
		t.Fatalf("create indexes: %v", err)
	}

	// Create an empty collection too.
	if err := db.CreateCollection(ctx, "empty_collection"); err != nil {
		t.Fatalf("create collection: %v", err)
	}

	if err := client.Disconnect(ctx); err != nil {
		t.Fatalf("disconnect seed client: %v", err)
	}

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate container: %v", err)
		}
	}
	return uri, cleanup
}

func TestIntegration_Inspector(t *testing.T) {
	uri, cleanup := setupMongoDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inspector, err := NewInspector(ctx, Config{URI: uri})
	if err != nil {
		t.Fatalf("NewInspector: %v", err)
	}
	defer func() { _ = inspector.Close(ctx) }()

	// Test GetServerVersion.
	info, err := inspector.GetServerVersion(ctx)
	if err != nil {
		t.Fatalf("GetServerVersion: %v", err)
	}
	if info.Version == "" {
		t.Error("server version is empty")
	}
	t.Logf("MongoDB version: %s", info.Version)

	// Test ListDatabases.
	dbs, err := inspector.ListDatabases(ctx, "")
	if err != nil {
		t.Fatalf("ListDatabases: %v", err)
	}
	found := false
	for _, db := range dbs {
		if db.Name == "testdb" {
			found = true
			break
		}
	}
	if !found {
		t.Error("testdb not found in ListDatabases")
	}

	// Test ListCollections.
	colls, err := inspector.ListCollections(ctx, "testdb")
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	collNames := make(map[string]bool)
	for _, c := range colls {
		collNames[c.Name] = true
	}
	if !collNames["users"] {
		t.Error("users collection not found")
	}
	if !collNames["empty_collection"] {
		t.Error("empty_collection not found")
	}

	// Test GetCollectionStats.
	stats, err := inspector.GetCollectionStats(ctx, "testdb", "users")
	if err != nil {
		t.Fatalf("GetCollectionStats: %v", err)
	}
	if stats.DocCount != 3 {
		t.Errorf("doc count = %d, want 3", stats.DocCount)
	}
	if stats.Size <= 0 {
		t.Errorf("size = %d, want > 0", stats.Size)
	}

	// Test GetIndexes.
	indexes, err := inspector.GetIndexes(ctx, "testdb", "users")
	if err != nil {
		t.Fatalf("GetIndexes: %v", err)
	}
	if len(indexes) < 3 { // _id + email + status_name
		t.Errorf("indexes = %d, want >= 3", len(indexes))
	}
	idxNames := make(map[string]bool)
	for _, idx := range indexes {
		idxNames[idx.Name] = true
	}
	if !idxNames["_id_"] {
		t.Error("_id_ index not found")
	}

	// Check email index is unique.
	for _, idx := range indexes {
		if len(idx.Key) > 0 && idx.Key[0].Field == "email" {
			if !idx.Unique {
				t.Error("email index should be unique")
			}
		}
	}

	// Test GetIndexStats.
	idxStats, err := inspector.GetIndexStats(ctx, "testdb", "users")
	if err != nil {
		t.Fatalf("GetIndexStats: %v", err)
	}
	if len(idxStats) == 0 {
		t.Error("no index stats returned")
	}

	// Test Inspect.
	collections, err := inspector.Inspect(ctx, "testdb")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(collections) < 2 {
		t.Errorf("Inspect returned %d collections, want >= 2", len(collections))
	}

	// Verify users collection has full metadata.
	for _, c := range collections {
		if c.Name == "users" {
			if c.DocCount != 3 {
				t.Errorf("users doc count = %d, want 3", c.DocCount)
			}
			if len(c.Indexes) < 3 {
				t.Errorf("users indexes = %d, want >= 3", len(c.Indexes))
			}
			return
		}
	}
	t.Error("users collection not found in Inspect results")
}
