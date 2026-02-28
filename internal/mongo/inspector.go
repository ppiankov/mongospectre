package mongo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
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

const shardingChunkAnalysisLimit int64 = 10_000

// dbClient abstracts the MongoDB client operations for testability.
type dbClient interface {
	Ping(ctx context.Context) error
	Disconnect(ctx context.Context) error
	ListDatabases(ctx context.Context, filter any) (mongo.ListDatabasesResult, error)
	ListCollectionSpecs(ctx context.Context, dbName string) ([]mongo.CollectionSpecification, error)
	RunCommand(ctx context.Context, dbName string, cmd any) *mongo.SingleResult
	ListIndexSpecs(ctx context.Context, dbName, collName string) ([]mongo.IndexSpecification, error)
	Aggregate(ctx context.Context, dbName, collName string, pipeline any) (*mongo.Cursor, error)
}

// mongoDBClient wraps the real mongo.Client to implement dbClient.
type mongoDBClient struct {
	client *mongo.Client
}

func (m *mongoDBClient) Ping(ctx context.Context) error {
	return m.client.Ping(ctx, nil)
}

func (m *mongoDBClient) Disconnect(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}

func (m *mongoDBClient) ListDatabases(ctx context.Context, filter any) (mongo.ListDatabasesResult, error) {
	return m.client.ListDatabases(ctx, filter)
}

func (m *mongoDBClient) ListCollectionSpecs(ctx context.Context, dbName string) ([]mongo.CollectionSpecification, error) {
	return m.client.Database(dbName).ListCollectionSpecifications(ctx, bson.D{})
}

func (m *mongoDBClient) RunCommand(ctx context.Context, dbName string, cmd any) *mongo.SingleResult {
	return m.client.Database(dbName).RunCommand(ctx, cmd)
}

func (m *mongoDBClient) ListIndexSpecs(ctx context.Context, dbName, collName string) ([]mongo.IndexSpecification, error) {
	return m.client.Database(dbName).Collection(collName).Indexes().ListSpecifications(ctx)
}

func (m *mongoDBClient) Aggregate(ctx context.Context, dbName, collName string, pipeline any) (*mongo.Cursor, error) {
	return m.client.Database(dbName).Collection(collName).Aggregate(ctx, pipeline)
}

// Inspector reads MongoDB metadata and statistics.
type Inspector struct {
	db dbClient
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
		return nil, classifyConnectError(fmt.Errorf("connect: %w", err))
	}

	dbc := &mongoDBClient{client: client}
	if err := dbc.Ping(ctx); err != nil {
		_ = dbc.Disconnect(ctx)
		return nil, classifyConnectError(fmt.Errorf("connect: %w", err))
	}

	return &Inspector{db: dbc}, nil
}

// Close disconnects from MongoDB.
func (i *Inspector) Close(ctx context.Context) error {
	return i.db.Disconnect(ctx)
}

// ListDatabases returns non-system databases, or a single database if cfg.Database is set.
func (i *Inspector) ListDatabases(ctx context.Context, database string) ([]DatabaseInfo, error) {
	if database != "" {
		return []DatabaseInfo{{Name: database}}, nil
	}

	result, err := i.db.ListDatabases(ctx, bson.D{})
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
	specs, err := i.db.ListCollectionSpecs(ctx, dbName)
	if err != nil {
		return nil, fmt.Errorf("list collections in %s: %w", dbName, err)
	}

	colls := make([]CollectionInfo, 0, len(specs))
	for idx := range specs {
		colls = append(colls, CollectionInfo{
			Name:     specs[idx].Name,
			Database: dbName,
			Type:     specs[idx].Type,
		})
	}
	return colls, nil
}

// GetValidators returns JSON schema validators configured on collections.
func (i *Inspector) GetValidators(ctx context.Context, database string) ([]ValidatorInfo, error) {
	dbs, err := i.ListDatabases(ctx, database)
	if err != nil {
		return nil, err
	}

	var validators []ValidatorInfo
	for _, db := range dbs {
		specs, err := i.db.ListCollectionSpecs(ctx, db.Name)
		if err != nil {
			return nil, fmt.Errorf("list collections in %s: %w", db.Name, err)
		}
		for i := range specs {
			v, ok := validatorFromSpec(db.Name, &specs[i])
			if ok {
				validators = append(validators, v)
			}
		}
	}
	return validators, nil
}

// GetCollectionStats populates size/count stats for a collection.
// Returns the collection info and a map of index name → size in bytes.
func (i *Inspector) GetCollectionStats(ctx context.Context, dbName, collName string) (CollectionInfo, map[string]int64, error) {
	result := i.db.RunCommand(ctx, dbName, bson.D{{Key: "collStats", Value: collName}})
	var raw bson.M
	if err := result.Decode(&raw); err != nil {
		return CollectionInfo{Name: collName, Database: dbName}, nil, fmt.Errorf("collStats %s.%s: %w", dbName, collName, err)
	}

	indexSizes := make(map[string]int64)
	switch rawSizes := raw["indexSizes"].(type) {
	case bson.M:
		for name, size := range rawSizes {
			indexSizes[name] = toInt64(size)
		}
	case bson.D:
		for _, e := range rawSizes {
			indexSizes[e.Key] = toInt64(e.Value)
		}
	}

	return CollectionInfo{
		Name:           collName,
		Database:       dbName,
		DocCount:       toInt64(raw["count"]),
		Size:           toInt64(raw["size"]),
		AvgObjSize:     toInt64(raw["avgObjSize"]),
		StorageSize:    toInt64(raw["storageSize"]),
		TotalIndexSize: toInt64(raw["totalIndexSize"]),
	}, indexSizes, nil
}

// GetIndexes returns index definitions for a collection.
func (i *Inspector) GetIndexes(ctx context.Context, dbName, collName string) ([]IndexInfo, error) {
	specs, err := i.db.ListIndexSpecs(ctx, dbName, collName)
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
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$indexStats", Value: bson.D{}}},
	}
	cursor, err := i.db.Aggregate(ctx, dbName, collName, pipeline)
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
		accesses := toBsonM(r["accesses"])
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
	result := i.db.RunCommand(ctx, "admin", bson.D{{Key: "buildInfo", Value: 1}})
	var raw bson.M
	if err := result.Decode(&raw); err != nil {
		return ServerInfo{}, fmt.Errorf("buildInfo: %w", err)
	}
	v, _ := raw["version"].(string)
	return ServerInfo{Version: v}, nil
}

// ReadProfiler reads recent profiler entries from system.profile and normalizes query shapes.
// It never modifies profiler settings and returns an empty slice when profiler data is unavailable.
func (i *Inspector) ReadProfiler(ctx context.Context, database string, limit int64) ([]ProfileEntry, error) {
	if limit <= 0 {
		limit = 1000
	}

	dbs, err := i.ListDatabases(ctx, database)
	if err != nil {
		return nil, err
	}
	if len(dbs) == 0 {
		return nil, nil
	}

	sortByNewest := bson.D{{Key: "ts", Value: int32(-1)}}
	entries := make([]ProfileEntry, 0)
	missingProfileNamespaces := 0

	for _, db := range dbs {
		docs, err := i.findDocumentsWithSort(ctx, db.Name, "system.profile", nil, sortByNewest, limit)
		if err != nil {
			if isNamespaceNotFoundErr(err) {
				missingProfileNamespaces++
				continue
			}
			return nil, fmt.Errorf("read %s.system.profile: %w", db.Name, err)
		}

		for _, doc := range docs {
			entry, ok := profileEntryFromDoc(db.Name, doc)
			if ok {
				entries = append(entries, entry)
			}
		}
	}

	if len(entries) == 0 {
		// All profile namespaces missing (profiler disabled) or simply empty.
		if missingProfileNamespaces == len(dbs) {
			return nil, nil
		}
		return nil, nil
	}

	sort.Slice(entries, func(i, j int) bool {
		ti := entries[i].Timestamp
		tj := entries[j].Timestamp
		if ti.Equal(tj) {
			return entries[i].DurationMillis > entries[j].DurationMillis
		}
		return ti.After(tj)
	})

	if int64(len(entries)) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// SampleDocuments samples documents from each collection and builds field frequency maps.
// Uses $sample aggregation to randomly select documents, then flattens them into dot-notation
// field paths with BSON type counts. Skips views and system collections.
func (i *Inspector) SampleDocuments(ctx context.Context, database string, sampleSize int64) ([]FieldSampleResult, error) {
	if sampleSize <= 0 {
		sampleSize = 100
	}

	dbs, err := i.ListDatabases(ctx, database)
	if err != nil {
		return nil, err
	}
	if len(dbs) == 0 {
		return nil, nil
	}

	var results []FieldSampleResult
	for _, db := range dbs {
		specs, err := i.db.ListCollectionSpecs(ctx, db.Name)
		if err != nil {
			return nil, fmt.Errorf("list collections in %s: %w", db.Name, err)
		}

		for idx := range specs {
			if specs[idx].Type == "view" || strings.HasPrefix(specs[idx].Name, "system.") {
				continue
			}

			pipeline := mongo.Pipeline{
				bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: sampleSize}}}},
			}
			cursor, err := i.db.Aggregate(ctx, db.Name, specs[idx].Name, pipeline)
			if err != nil {
				if isNamespaceNotFoundErr(err) {
					continue
				}
				return nil, fmt.Errorf("$sample %s.%s: %w", db.Name, specs[idx].Name, err)
			}

			var docs []bson.M
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, fmt.Errorf("read $sample %s.%s: %w", db.Name, specs[idx].Name, err)
			}
			if len(docs) == 0 {
				continue
			}

			// Build field frequency map: path -> type -> count.
			fieldTypes := make(map[string]map[string]int64)
			for _, doc := range docs {
				flattenDocument(doc, "", fieldTypes)
			}

			fields := make([]FieldFrequency, 0, len(fieldTypes))
			for path, types := range fieldTypes {
				var total int64
				for _, c := range types {
					total += c
				}
				fields = append(fields, FieldFrequency{
					Path:  path,
					Count: total,
					Types: types,
				})
			}
			sort.Slice(fields, func(a, b int) bool { return fields[a].Path < fields[b].Path })

			results = append(results, FieldSampleResult{
				Database:   db.Name,
				Collection: specs[idx].Name,
				SampleSize: int64(len(docs)),
				Fields:     fields,
			})
		}
	}

	return results, nil
}

// flattenDocument recursively walks a BSON document and records each field path
// with its BSON type into out[path][typeName]++.
func flattenDocument(doc bson.M, prefix string, out map[string]map[string]int64) {
	for key, val := range doc {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		typeName := bsonTypeName(val)

		if types, ok := out[path]; ok {
			types[typeName]++
		} else {
			out[path] = map[string]int64{typeName: 1}
		}

		switch v := val.(type) {
		case bson.M:
			flattenDocument(v, path, out)
		case bson.D:
			m := make(bson.M, len(v))
			for _, e := range v {
				m[e.Key] = e.Value
			}
			flattenDocument(m, path, out)
		case bson.A:
			flattenArray(v, path, out)
		case []any:
			flattenArray(bson.A(v), path, out)
		}
	}
}

// flattenArray walks array elements and records nested fields under path[].
func flattenArray(arr bson.A, path string, out map[string]map[string]int64) {
	arrayPath := path + "[]"
	for _, elem := range arr {
		switch v := elem.(type) {
		case bson.M:
			flattenDocument(v, arrayPath, out)
		case bson.D:
			m := make(bson.M, len(v))
			for _, e := range v {
				m[e.Key] = e.Value
			}
			flattenDocument(m, arrayPath, out)
		}
	}
}

// bsonTypeName returns a human-readable BSON type name for a Go value.
func bsonTypeName(v any) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case int32:
		return "int32"
	case int64:
		return "int64"
	case float64:
		return "double"
	case bool:
		return "bool"
	case bson.ObjectID:
		return "objectId"
	case time.Time:
		return "date"
	case bson.DateTime:
		return "date"
	case bson.M, bson.D:
		return "object"
	case bson.A, []any:
		return "array"
	case bson.Binary:
		return "binData"
	case bson.Regex:
		return "regex"
	default:
		return "unknown"
	}
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

			stats, indexSizes, statsErr := i.GetCollectionStats(ctx, db.Name, coll.Name)
			if statsErr == nil {
				coll.DocCount = stats.DocCount
				coll.Size = stats.Size
				coll.AvgObjSize = stats.AvgObjSize
				coll.StorageSize = stats.StorageSize
				coll.TotalIndexSize = stats.TotalIndexSize
			}

			indexes, idxErr := i.GetIndexes(ctx, db.Name, coll.Name)
			if idxErr == nil {
				idxStats, _ := i.GetIndexStats(ctx, db.Name, coll.Name)
				for j := range indexes {
					if s, ok := idxStats[indexes[j].Name]; ok {
						indexes[j].Stats = &s
					}
					if size, ok := indexSizes[indexes[j].Name]; ok {
						indexes[j].Size = size
					}
				}
				coll.Indexes = indexes
			}

			all = append(all, coll)
		}
	}
	return all, nil
}

// InspectUsers queries the usersInfo command on a database and returns user metadata.
func (i *Inspector) InspectUsers(ctx context.Context, dbName string) ([]UserInfo, error) {
	result := i.db.RunCommand(ctx, dbName, bson.D{{Key: "usersInfo", Value: 1}})
	var resp struct {
		Users []UserInfo `bson:"users"`
	}
	if err := result.Decode(&resp); err != nil {
		return nil, fmt.Errorf("usersInfo on %s: %w", dbName, err)
	}
	return resp.Users, nil
}

// InspectSharding gathers sharding metadata from config collections.
// Returns Enabled=false with no error for non-sharded deployments.
func (i *Inspector) InspectSharding(ctx context.Context) (ShardingInfo, error) {
	shardDocs, err := i.findDocuments(ctx, "config", "shards", bson.M{}, 0)
	if err != nil {
		if isNonShardedDeploymentErr(err) {
			return ShardingInfo{}, nil
		}
		return ShardingInfo{}, fmt.Errorf("read config.shards: %w", err)
	}
	if len(shardDocs) == 0 {
		return ShardingInfo{}, nil
	}

	info := ShardingInfo{
		Enabled:         true,
		BalancerEnabled: true,
		Shards:          make([]string, 0, len(shardDocs)),
	}
	for _, doc := range shardDocs {
		shardName := toString(doc["_id"])
		if shardName == "" {
			continue
		}
		info.Shards = append(info.Shards, shardName)
	}
	sort.Strings(info.Shards)

	collectionDocs, err := i.findDocuments(ctx, "config", "collections", bson.M{
		"dropped": bson.M{"$ne": true},
		"key":     bson.M{"$exists": true},
	}, 0)
	if err != nil {
		return ShardingInfo{}, fmt.Errorf("read config.collections: %w", err)
	}

	collections := make([]ShardedCollectionInfo, 0, len(collectionDocs))
	collectionUUIDs := make(map[string]any, len(collectionDocs))
	for _, doc := range collectionDocs {
		ns := toString(doc["_id"])
		dbName, collName := splitNamespace(ns)
		if ns == "" || dbName == "" || collName == "" {
			continue
		}

		collections = append(collections, ShardedCollectionInfo{
			Namespace:         ns,
			Database:          dbName,
			Collection:        collName,
			Key:               bsonAnyToKeyFields(doc["key"]),
			ChunkDistribution: make(map[string]int64),
		})
		if uuid, ok := doc["uuid"]; ok {
			collectionUUIDs[ns] = uuid
		}
	}

	sort.Slice(collections, func(a, b int) bool {
		return collections[a].Namespace < collections[b].Namespace
	})

	for idx := range collections {
		filter := bson.M{"ns": collections[idx].Namespace}
		if uuid, ok := collectionUUIDs[collections[idx].Namespace]; ok && uuid != nil {
			filter = bson.M{"uuid": uuid}
		}

		chunkDocs, err := i.findDocuments(ctx, "config", "chunks", filter, shardingChunkAnalysisLimit+1)
		if err != nil {
			return ShardingInfo{}, fmt.Errorf("read config.chunks for %s: %w", collections[idx].Namespace, err)
		}
		if int64(len(chunkDocs)) > shardingChunkAnalysisLimit {
			chunkDocs = chunkDocs[:shardingChunkAnalysisLimit]
			collections[idx].ChunkLimitHit = true
		}

		var jumboCount int64
		for _, chunk := range chunkDocs {
			shardName := toString(chunk["shard"])
			if shardName != "" {
				collections[idx].ChunkDistribution[shardName]++
				collections[idx].ChunkCount++
			}
			if toBool(chunk["jumbo"]) {
				jumboCount++
			}
		}
		collections[idx].JumboChunks = jumboCount
	}

	balancerDocs, err := i.findDocuments(ctx, "config", "settings", bson.M{"_id": "balancer"}, 1)
	if err != nil {
		if !isNamespaceNotFoundErr(err) {
			return ShardingInfo{}, fmt.Errorf("read config.settings: %w", err)
		}
	} else if len(balancerDocs) > 0 {
		info.BalancerEnabled = !toBool(balancerDocs[0]["stopped"])
	}

	info.Collections = collections
	return info, nil
}

func (i *Inspector) findDocuments(ctx context.Context, dbName, collName string, filter bson.M, limit int64) ([]bson.M, error) {
	return i.findDocumentsWithSort(ctx, dbName, collName, filter, nil, limit)
}

func (i *Inspector) findDocumentsWithSort(
	ctx context.Context,
	dbName, collName string,
	filter bson.M,
	sortDoc bson.D,
	limit int64,
) ([]bson.M, error) {
	cmd := bson.D{{Key: "find", Value: collName}}
	if filter != nil {
		cmd = append(cmd, bson.E{Key: "filter", Value: filter})
	}
	if len(sortDoc) > 0 {
		cmd = append(cmd, bson.E{Key: "sort", Value: sortDoc})
	}
	if limit > 0 {
		cmd = append(cmd, bson.E{Key: "limit", Value: limit})
	}

	batchSize := int32(500)
	if limit > 0 && limit < int64(batchSize) {
		batchSize = int32(limit)
	}
	cmd = append(cmd, bson.E{Key: "batchSize", Value: batchSize})

	var findResp struct {
		Cursor struct {
			ID         int64    `bson:"id"`
			FirstBatch []bson.M `bson:"firstBatch"`
		} `bson:"cursor"`
	}
	if err := i.db.RunCommand(ctx, dbName, cmd).Decode(&findResp); err != nil {
		return nil, err
	}

	docs := append([]bson.M(nil), findResp.Cursor.FirstBatch...)
	cursorID := findResp.Cursor.ID

	if limit > 0 && int64(len(docs)) >= limit {
		return docs[:limit], nil
	}

	for cursorID != 0 {
		getMore := bson.D{
			{Key: "getMore", Value: cursorID},
			{Key: "collection", Value: collName},
			{Key: "batchSize", Value: batchSize},
		}
		if limit > 0 {
			remaining := limit - int64(len(docs))
			if remaining <= 0 {
				break
			}
			if remaining < int64(batchSize) {
				getMore[2].Value = int32(remaining)
			}
		}

		var getMoreResp struct {
			Cursor struct {
				ID        int64    `bson:"id"`
				NextBatch []bson.M `bson:"nextBatch"`
			} `bson:"cursor"`
		}
		if err := i.db.RunCommand(ctx, dbName, getMore).Decode(&getMoreResp); err != nil {
			return nil, err
		}

		docs = append(docs, getMoreResp.Cursor.NextBatch...)
		cursorID = getMoreResp.Cursor.ID
		if limit > 0 && int64(len(docs)) >= limit {
			return docs[:limit], nil
		}
	}

	return docs, nil
}

func profileEntryFromDoc(defaultDB string, doc bson.M) (ProfileEntry, bool) {
	command := toBsonM(doc["command"])
	collName := profileCollectionFromCommand(command)
	nsDB, nsColl := splitNamespace(toString(doc["ns"]))
	if collName == "" {
		collName = nsColl
	}
	if collName == "" {
		return ProfileEntry{}, false
	}

	dbName := defaultDB
	if nsDB != "" {
		dbName = nsDB
	}

	filterFields := extractProfileFields(nil)
	sortFields := extractProfileFields(nil)
	projectionFields := extractProfileFields(nil)
	if command != nil {
		filterFields = extractProfileFields(command["filter"])
		sortFields = extractProfileFields(command["sort"])
		projectionFields = extractProfileFields(command["projection"])
	}
	if len(filterFields) == 0 {
		// Older profiler payloads may store query predicates under "query".
		filterFields = extractProfileFields(doc["query"])
	}

	durationMillis := toInt64(doc["millis"])
	if durationMillis == 0 {
		durationMillis = toInt64(doc["durationMillis"])
	}

	return ProfileEntry{
		Database:         dbName,
		Collection:       collName,
		FilterFields:     filterFields,
		SortFields:       sortFields,
		ProjectionFields: projectionFields,
		DurationMillis:   durationMillis,
		Timestamp:        toTime(doc["ts"]),
		PlanSummary:      toString(doc["planSummary"]),
	}, true
}

func profileCollectionFromCommand(command bson.M) string {
	if command == nil {
		return ""
	}

	for _, key := range []string{"find", "aggregate", "count", "distinct", "delete", "update", "findAndModify", "findandmodify"} {
		if collName := toString(command[key]); collName != "" {
			return collName
		}
	}
	return ""
}

func extractProfileFields(v any) []string {
	seen := make(map[string]bool)
	var fields []string
	walkProfileFields(v, "", seen, &fields)
	sort.Strings(fields)
	return fields
}

func walkProfileFields(v any, prefix string, seen map[string]bool, fields *[]string) {
	switch value := v.(type) {
	case bson.M:
		for key, nested := range value {
			walkProfileFieldEntry(key, nested, prefix, seen, fields)
		}
	case map[string]any:
		for key, nested := range value {
			walkProfileFieldEntry(key, nested, prefix, seen, fields)
		}
	case bson.D:
		for _, entry := range value {
			walkProfileFieldEntry(entry.Key, entry.Value, prefix, seen, fields)
		}
	case bson.A:
		for _, nested := range value {
			walkProfileFields(nested, prefix, seen, fields)
		}
	case []any:
		for _, nested := range value {
			walkProfileFields(nested, prefix, seen, fields)
		}
	}
}

func walkProfileFieldEntry(key string, value any, prefix string, seen map[string]bool, fields *[]string) {
	if key == "" {
		return
	}
	if strings.HasPrefix(key, "$") {
		walkProfileFields(value, prefix, seen, fields)
		return
	}

	name := key
	if prefix != "" {
		name = prefix + "." + key
	}
	if !seen[name] {
		seen[name] = true
		*fields = append(*fields, name)
	}
	walkProfileFields(value, name, seen, fields)
}

func bsonAnyToKeyFields(v any) []KeyField {
	switch keyDoc := v.(type) {
	case bson.D:
		out := make([]KeyField, 0, len(keyDoc))
		for _, e := range keyDoc {
			out = append(out, KeyField{Field: e.Key, Direction: int(toInt64(e.Value))})
		}
		return out
	case bson.M:
		out := make([]KeyField, 0, len(keyDoc))
		for field, dir := range keyDoc {
			out = append(out, KeyField{Field: field, Direction: int(toInt64(dir))})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
		return out
	default:
		return nil
	}
}

func splitNamespace(ns string) (string, string) {
	parts := strings.SplitN(ns, ".", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// classifyConnectError wraps connection errors with actionable troubleshooting hints.
func classifyConnectError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "context deadline exceeded") &&
		(strings.Contains(msg, "ReplicaSetNoPrimary") || strings.Contains(msg, "server selection")):
		return fmt.Errorf("%w\n\nhint: could not reach any replica set member within the timeout. Common causes:\n"+
			"  - IP address not in Atlas Network Access list\n"+
			"  - firewall or VPN blocking port 27017\n"+
			"  - DNS cannot resolve SRV record (try: nslookup _mongodb._tcp.<host>)\n"+
			"  - increase timeout with --timeout 60s\n"+
			"  see: docs/troubleshooting.md", err)
	case strings.Contains(msg, "authentication failed") || strings.Contains(msg, "auth error"):
		return fmt.Errorf("%w\n\nhint: authentication failed. Check username, password, and authSource in your URI\n"+
			"  see: docs/troubleshooting.md", err)
	case strings.Contains(msg, "connection refused"):
		return fmt.Errorf("%w\n\nhint: connection refused. Is MongoDB running at this address?\n"+
			"  see: docs/troubleshooting.md", err)
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "server misbehaving"):
		return fmt.Errorf("%w\n\nhint: DNS resolution failed. Check the hostname in your URI\n"+
			"  see: docs/troubleshooting.md", err)
	default:
		return err
	}
}

func isNamespaceNotFoundErr(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) && cmdErr.Code == 26 {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "namespace") && strings.Contains(msg, "not found")
}

func isNonShardedDeploymentErr(err error) bool {
	if isNamespaceNotFoundErr(err) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not running with --configsvr") ||
		strings.Contains(msg, "not supported on mongod") ||
		strings.Contains(msg, "does not support sharding")
}

func toBool(v any) bool {
	b, _ := v.(bool)
	return b
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
// Handles both time.Time (from real cursor) and bson.DateTime (from round-tripped BSON).
func toTime(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case bson.DateTime:
		return time.UnixMilli(int64(t)).UTC()
	default:
		return time.Time{}
	}
}

// toBsonM converts a value to bson.M, handling both bson.M and bson.D inputs.
func toBsonM(v any) bson.M {
	switch m := v.(type) {
	case bson.M:
		return m
	case bson.D:
		result := make(bson.M, len(m))
		for _, e := range m {
			result[e.Key] = e.Value
		}
		return result
	default:
		return nil
	}
}

func validatorFromSpec(dbName string, spec *mongo.CollectionSpecification) (ValidatorInfo, bool) {
	if len(spec.Options) == 0 {
		return ValidatorInfo{}, false
	}

	var opts bson.M
	if err := bson.Unmarshal(spec.Options, &opts); err != nil {
		return ValidatorInfo{}, false
	}

	validator := toBsonM(opts["validator"])
	if validator == nil {
		return ValidatorInfo{}, false
	}

	jsonSchema := toBsonM(validator["$jsonSchema"])
	if jsonSchema == nil {
		return ValidatorInfo{}, false
	}

	return ValidatorInfo{
		Collection:       spec.Name,
		Database:         dbName,
		Schema:           parseValidatorSchema(jsonSchema),
		ValidationLevel:  toString(opts["validationLevel"]),
		ValidationAction: toString(opts["validationAction"]),
	}, true
}

func parseValidatorSchema(schema bson.M) ValidatorSchema {
	out := ValidatorSchema{
		Required:             parseStringArray(schema["required"]),
		AdditionalProperties: parseBoolPointer(schema["additionalProperties"]),
	}

	props := toBsonM(schema["properties"])
	if len(props) == 0 {
		return out
	}

	out.Properties = make(map[string]ValidatorField, len(props))
	for field, raw := range props {
		fieldSchema := toBsonM(raw)
		if fieldSchema == nil {
			continue
		}
		types := parseBSONTypes(fieldSchema["bsonType"])
		if len(types) == 0 {
			types = parseBSONTypes(fieldSchema["type"])
		}
		out.Properties[field] = ValidatorField{BSONTypes: types}
	}
	return out
}

func parseBSONTypes(v any) []string {
	switch t := v.(type) {
	case string:
		normalized := normalizeBSONType(t)
		if normalized == "" {
			return nil
		}
		return []string{normalized}
	case []string:
		out := make([]string, 0, len(t))
		for _, it := range t {
			if normalized := normalizeBSONType(it); normalized != "" {
				out = append(out, normalized)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, it := range t {
			s, ok := it.(string)
			if !ok {
				continue
			}
			if normalized := normalizeBSONType(s); normalized != "" {
				out = append(out, normalized)
			}
		}
		return out
	case bson.A:
		out := make([]string, 0, len(t))
		for _, it := range t {
			s, ok := it.(string)
			if !ok {
				continue
			}
			if normalized := normalizeBSONType(s); normalized != "" {
				out = append(out, normalized)
			}
		}
		return out
	default:
		return nil
	}
}

func normalizeBSONType(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	switch s {
	case "int", "long", "double", "decimal":
		return "number"
	case "boolean":
		return "bool"
	case "objectid":
		return "objectId"
	default:
		return s
	}
}

func parseStringArray(v any) []string {
	switch arr := v.(type) {
	case []string:
		return append([]string(nil), arr...)
	case []any:
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			s, ok := item.(string)
			if ok {
				out = append(out, s)
			}
		}
		return out
	case bson.A:
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			s, ok := item.(string)
			if ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func parseBoolPointer(v any) *bool {
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

// bsonRawToKeyFields converts a bson.Raw key document to ordered []KeyField.
// Handles numeric directions (1, -1) and string index types ("text", "2dsphere",
// InspectSecurity queries server parameters and command-line options to assess
// security configuration. Requires admin access. Returns partial results on
// permission errors rather than failing completely.
func (i *Inspector) InspectSecurity(ctx context.Context) (SecurityInfo, error) {
	var info SecurityInfo

	// getParameter: auth, TLS, localhost bypass.
	paramResult := i.db.RunCommand(ctx, "admin", bson.D{{Key: "getParameter", Value: "*"}})
	var params bson.M
	if err := paramResult.Decode(&params); err != nil {
		return info, fmt.Errorf("getParameter: %w", err)
	}

	// authenticationMechanisms: non-empty array means auth is active.
	if mechs, ok := params["authenticationMechanisms"]; ok {
		if arr, isArr := mechs.(bson.A); isArr && len(arr) > 0 {
			info.AuthEnabled = true
		}
	}

	if mode, ok := params["tlsMode"].(string); ok {
		info.TLSMode = mode
	}
	if bypass, ok := params["enableLocalhostAuthBypass"].(bool); ok {
		info.LocalhostAuthBypass = bypass
	}
	if allow, ok := params["tlsAllowInvalidCertificates"].(bool); ok {
		info.TLSAllowInvalidCerts = allow
	}

	// getCmdLineOpts: bind IP, authorization, audit log.
	cmdResult := i.db.RunCommand(ctx, "admin", bson.D{{Key: "getCmdLineOpts", Value: 1}})
	var cmdOpts bson.M
	if err := cmdResult.Decode(&cmdOpts); err != nil {
		// Permission denied is common — return what we have from getParameter.
		return info, nil //nolint:nilerr // partial results are acceptable
	}

	parsed := toBsonM(cmdOpts["parsed"])

	// net.bindIp / net.bindIpAll
	if netSection := toBsonM(parsed["net"]); netSection != nil {
		if bindIP, ok := netSection["bindIp"].(string); ok {
			info.BindIP = bindIP
		}
		if bindAll, ok := netSection["bindIpAll"].(bool); ok && bindAll {
			info.BindIP = "0.0.0.0"
		}
	}

	// security.authorization
	if secSection := toBsonM(parsed["security"]); secSection != nil {
		if auth, ok := secSection["authorization"].(string); ok && auth == "enabled" {
			info.AuthEnabled = true
		}
	}

	// auditLog.destination
	if auditSection := toBsonM(parsed["auditLog"]); auditSection != nil {
		if dest, ok := auditSection["destination"].(string); ok && dest != "" {
			info.AuditLogEnabled = true
		}
	}

	return info, nil
}

// "2d", "hashed") which are stored as Direction=0 (non-directional).
func bsonRawToKeyFields(raw bson.Raw) []KeyField {
	elems, err := raw.Elements()
	if err != nil {
		return nil
	}
	fields := make([]KeyField, 0, len(elems))
	for _, elem := range elems {
		kf := KeyField{Field: elem.Key()}
		v := elem.Value()
		switch v.Type {
		case bson.TypeInt32, bson.TypeInt64, bson.TypeDouble:
			kf.Direction = int(v.AsInt64())
		default:
			// text, 2dsphere, 2d, hashed — non-directional
			kf.Direction = 0
		}
		fields = append(fields, kf)
	}
	return fields
}
