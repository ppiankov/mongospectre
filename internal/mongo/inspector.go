package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var systemDBs = map[string]bool{
	"admin":  true,
	"local":  true,
	"config": true,
}

// Inspector reads MongoDB metadata and statistics.
type Inspector struct {
	client *mongo.Client
}

// NewInspector connects to MongoDB and verifies the connection.
// The context deadline is used to bound connection and server selection time.
func NewInspector(ctx context.Context, cfg Config) (*Inspector, error) {
	opts := options.Client().ApplyURI(cfg.URI)

	// Derive connection timeouts from context deadline so unreachable hosts
	// don't hang for the OS-level TCP timeout (~2 min).
	if deadline, ok := ctx.Deadline(); ok {
		d := time.Until(deadline)
		opts.SetConnectTimeout(d)
		opts.SetServerSelectionTimeout(d)
	}

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &Inspector{client: client}, nil
}

// Close disconnects from MongoDB.
func (i *Inspector) Close(ctx context.Context) error {
	return i.client.Disconnect(ctx)
}

// ListDatabases returns non-system databases, or a single database if cfg.Database is set.
func (i *Inspector) ListDatabases(ctx context.Context, database string) ([]DatabaseInfo, error) {
	if database != "" {
		return []DatabaseInfo{{Name: database}}, nil
	}

	result, err := i.client.ListDatabases(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}

	var dbs []DatabaseInfo
	for _, db := range result.Databases {
		if systemDBs[db.Name] {
			continue
		}
		dbs = append(dbs, DatabaseInfo{
			Name:       db.Name,
			SizeOnDisk: db.SizeOnDisk,
			Empty:      db.SizeOnDisk == 0,
		})
	}
	return dbs, nil
}

// ListCollections returns metadata for all collections in a database.
func (i *Inspector) ListCollections(ctx context.Context, dbName string) ([]CollectionInfo, error) {
	db := i.client.Database(dbName)
	specs, err := db.ListCollectionSpecifications(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("list collections in %s: %w", dbName, err)
	}

	colls := make([]CollectionInfo, 0, len(specs))
	for i := range specs {
		colls = append(colls, CollectionInfo{
			Name:     specs[i].Name,
			Database: dbName,
			Type:     specs[i].Type,
		})
	}
	return colls, nil
}

// GetCollectionStats populates size/count stats for a collection.
func (i *Inspector) GetCollectionStats(ctx context.Context, dbName, collName string) (CollectionInfo, error) {
	db := i.client.Database(dbName)

	result := db.RunCommand(ctx, bson.D{{Key: "collStats", Value: collName}})
	var raw bson.M
	if err := result.Decode(&raw); err != nil {
		return CollectionInfo{Name: collName, Database: dbName}, fmt.Errorf("collStats %s.%s: %w", dbName, collName, err)
	}

	return CollectionInfo{
		Name:        collName,
		Database:    dbName,
		DocCount:    toInt64(raw["count"]),
		Size:        toInt64(raw["size"]),
		AvgObjSize:  toInt64(raw["avgObjSize"]),
		StorageSize: toInt64(raw["storageSize"]),
	}, nil
}

// GetIndexes returns index definitions for a collection.
func (i *Inspector) GetIndexes(ctx context.Context, dbName, collName string) ([]IndexInfo, error) {
	coll := i.client.Database(dbName).Collection(collName)
	specs, err := coll.Indexes().ListSpecifications(ctx)
	if err != nil {
		return nil, fmt.Errorf("list indexes %s.%s: %w", dbName, collName, err)
	}

	indexes := make([]IndexInfo, 0, len(specs))
	for _, spec := range specs {
		idx := IndexInfo{
			Name: spec.Name,
			Key:  bsonRawToKeyFields(spec.KeysDocument),
		}
		if spec.Unique != nil {
			idx.Unique = *spec.Unique
		}
		if spec.Sparse != nil {
			idx.Sparse = *spec.Sparse
		}
		if spec.ExpireAfterSeconds != nil {
			ttl := *spec.ExpireAfterSeconds
			idx.TTL = &ttl
		}
		indexes = append(indexes, idx)
	}
	return indexes, nil
}

// GetIndexStats returns usage statistics for all indexes on a collection.
func (i *Inspector) GetIndexStats(ctx context.Context, dbName, collName string) (map[string]IndexStats, error) {
	coll := i.client.Database(dbName).Collection(collName)

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$indexStats", Value: bson.D{}}},
	}
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("$indexStats %s.%s: %w", dbName, collName, err)
	}

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("read $indexStats %s.%s: %w", dbName, collName, err)
	}

	stats := make(map[string]IndexStats, len(results))
	for _, r := range results {
		name, _ := r["name"].(string)
		if name == "" {
			continue
		}
		accesses, _ := r["accesses"].(bson.M)
		if accesses == nil {
			continue
		}
		stats[name] = IndexStats{
			Ops:   toInt64(accesses["ops"]),
			Since: toTime(accesses["since"]),
		}
	}
	return stats, nil
}

// GetServerVersion returns the MongoDB server version string.
func (i *Inspector) GetServerVersion(ctx context.Context) (ServerInfo, error) {
	result := i.client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}})
	var raw bson.M
	if err := result.Decode(&raw); err != nil {
		return ServerInfo{}, fmt.Errorf("buildInfo: %w", err)
	}
	v, _ := raw["version"].(string)
	return ServerInfo{Version: v}, nil
}

// Inspect gathers full metadata for all collections in the given databases.
func (i *Inspector) Inspect(ctx context.Context, database string) ([]CollectionInfo, error) {
	dbs, err := i.ListDatabases(ctx, database)
	if err != nil {
		return nil, err
	}

	var all []CollectionInfo
	for _, db := range dbs {
		colls, err := i.ListCollections(ctx, db.Name)
		if err != nil {
			return nil, err
		}
		for _, coll := range colls {
			if coll.Type == "view" {
				all = append(all, coll)
				continue
			}

			stats, statsErr := i.GetCollectionStats(ctx, db.Name, coll.Name)
			if statsErr == nil {
				coll.DocCount = stats.DocCount
				coll.Size = stats.Size
				coll.AvgObjSize = stats.AvgObjSize
				coll.StorageSize = stats.StorageSize
			}

			indexes, idxErr := i.GetIndexes(ctx, db.Name, coll.Name)
			if idxErr == nil {
				idxStats, _ := i.GetIndexStats(ctx, db.Name, coll.Name)
				for j := range indexes {
					if s, ok := idxStats[indexes[j].Name]; ok {
						indexes[j].Stats = &s
					}
				}
				coll.Indexes = indexes
			}

			all = append(all, coll)
		}
	}
	return all, nil
}

// toInt64 converts a BSON numeric value to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

// toTime converts a BSON value to time.Time.
func toTime(v any) time.Time {
	if t, ok := v.(time.Time); ok {
		return t
	}
	return time.Time{}
}

// bsonRawToKeyFields converts a bson.Raw key document to ordered []KeyField.
func bsonRawToKeyFields(raw bson.Raw) []KeyField {
	elems, err := raw.Elements()
	if err != nil {
		return nil
	}
	fields := make([]KeyField, 0, len(elems))
	for _, elem := range elems {
		fields = append(fields, KeyField{
			Field:     elem.Key(),
			Direction: int(elem.Value().AsInt64()),
		})
	}
	return fields
}
