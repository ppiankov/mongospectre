package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// mockClient implements dbClient for unit tests.
type mockClient struct {
	pingErr       error
	disconnectErr error
	listDBsResult mongo.ListDatabasesResult
	listDBsErr    error
	collSpecs     []mongo.CollectionSpecification
	collSpecsErr  error
	runCmdResult  bson.Raw
	runCmdErr     error
	runCmdHook    func(dbName string, cmd any) (bson.Raw, error)
	indexSpecs    []mongo.IndexSpecification
	indexSpecsErr error
	aggregateErr  error
	aggregateData []bson.M
}

func (m *mockClient) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockClient) Disconnect(ctx context.Context) error {
	return m.disconnectErr
}

func (m *mockClient) ListDatabases(ctx context.Context, filter any) (mongo.ListDatabasesResult, error) {
	return m.listDBsResult, m.listDBsErr
}

func (m *mockClient) ListCollectionSpecs(ctx context.Context, dbName string) ([]mongo.CollectionSpecification, error) {
	return m.collSpecs, m.collSpecsErr
}

func (m *mockClient) RunCommand(ctx context.Context, dbName string, cmd any) *mongo.SingleResult {
	if m.runCmdHook != nil {
		raw, err := m.runCmdHook(dbName, cmd)
		if err != nil {
			return mongo.NewSingleResultFromDocument(bson.D{}, err, nil)
		}
		return mongo.NewSingleResultFromDocument(raw, nil, nil)
	}

	if m.runCmdErr != nil || m.runCmdResult == nil {
		// Return a SingleResult that will error on Decode.
		return mongo.NewSingleResultFromDocument(bson.D{}, m.runCmdErr, nil)
	}
	return mongo.NewSingleResultFromDocument(m.runCmdResult, nil, nil)
}

func (m *mockClient) ListIndexSpecs(ctx context.Context, dbName, collName string) ([]mongo.IndexSpecification, error) {
	return m.indexSpecs, m.indexSpecsErr
}

func (m *mockClient) Aggregate(ctx context.Context, dbName, collName string, pipeline any) (*mongo.Cursor, error) {
	if m.aggregateErr != nil {
		return nil, m.aggregateErr
	}
	docs := make([]any, len(m.aggregateData))
	for i, d := range m.aggregateData {
		docs[i] = d
	}
	cursor, err := mongo.NewCursorFromDocuments(docs, nil, nil)
	if err != nil {
		return nil, err
	}
	return cursor, nil
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64
	}{
		{"int32", int32(42), 42},
		{"int64", int64(100), 100},
		{"float64", float64(3.14), 3},
		{"nil", nil, 0},
		{"string", "nope", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toInt64(tt.in)
			if got != tt.want {
				t.Errorf("toInt64(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestToTime(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	got := toTime(now)
	if !got.Equal(now) {
		t.Errorf("toTime(%v) = %v, want %v", now, got, now)
	}

	zero := toTime("not a time")
	if !zero.IsZero() {
		t.Errorf("toTime(string) should be zero, got %v", zero)
	}
}

func TestBsonRawToKeyFields(t *testing.T) {
	raw, err := bson.Marshal(bson.D{{Key: "name", Value: 1}, {Key: "age", Value: -1}})
	if err != nil {
		t.Fatal(err)
	}
	fields := bsonRawToKeyFields(raw)

	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Field != "name" || fields[0].Direction != 1 {
		t.Errorf("fields[0] = %+v, want {name, 1}", fields[0])
	}
	if fields[1].Field != "age" || fields[1].Direction != -1 {
		t.Errorf("fields[1] = %+v, want {age, -1}", fields[1])
	}
}

func TestBsonRawToKeyFields_Empty(t *testing.T) {
	raw, err := bson.Marshal(bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	fields := bsonRawToKeyFields(raw)
	if len(fields) != 0 {
		t.Errorf("expected empty slice, got %v", fields)
	}
}

func TestBsonRawToKeyFields_Invalid(t *testing.T) {
	fields := bsonRawToKeyFields(bson.Raw{0xFF})
	if fields != nil {
		t.Errorf("expected nil for invalid raw, got %v", fields)
	}
}

func TestSystemDBs(t *testing.T) {
	for _, name := range []string{"admin", "local", "config"} {
		if !systemDBs[name] {
			t.Errorf("%s should be a system db", name)
		}
	}
	if systemDBs["myapp"] {
		t.Error("myapp should not be a system db")
	}
}

func TestToTime_BsonDateTime(t *testing.T) {
	dt := bson.DateTime(1735689600000)
	got := toTime(dt)
	want := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("toTime(bson.DateTime) = %v, want %v", got, want)
	}
}

func TestNewInspector_InvalidURI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err := NewInspector(ctx, Config{URI: "mongodb://localhost:1/"})
	if err == nil {
		t.Fatal("expected connection error for unreachable host")
	}
}

func TestToBsonM(t *testing.T) {
	// bson.M passthrough
	m := bson.M{"key": "val"}
	got := toBsonM(m)
	if got["key"] != "val" {
		t.Errorf("toBsonM(bson.M) = %v", got)
	}

	// bson.D conversion
	d := bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 2}}
	got = toBsonM(d)
	if got["a"] != 1 || got["b"] != 2 {
		t.Errorf("toBsonM(bson.D) = %v", got)
	}

	// nil/other type
	if toBsonM("string") != nil {
		t.Error("toBsonM(string) should be nil")
	}
	if toBsonM(nil) != nil {
		t.Error("toBsonM(nil) should be nil")
	}
}

func TestListDatabases_SpecificDB(t *testing.T) {
	insp := &Inspector{db: &mockClient{}}
	dbs, err := insp.ListDatabases(context.TODO(), "mydb")
	if err != nil {
		t.Fatal(err)
	}
	if len(dbs) != 1 || dbs[0].Name != "mydb" {
		t.Errorf("expected [{Name:mydb}], got %+v", dbs)
	}
}

func TestListDatabases_All(t *testing.T) {
	mc := &mockClient{
		listDBsResult: mongo.ListDatabasesResult{
			Databases: []mongo.DatabaseSpecification{
				{Name: "myapp", SizeOnDisk: 1024},
				{Name: "admin", SizeOnDisk: 512},
				{Name: "local", SizeOnDisk: 256},
				{Name: "staging", SizeOnDisk: 0},
			},
		},
	}
	insp := &Inspector{db: mc}
	dbs, err := insp.ListDatabases(context.TODO(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(dbs) != 2 {
		t.Fatalf("expected 2 non-system dbs, got %d: %+v", len(dbs), dbs)
	}
	if dbs[0].Name != "myapp" || dbs[0].SizeOnDisk != 1024 || dbs[0].Empty {
		t.Errorf("dbs[0] = %+v", dbs[0])
	}
	if dbs[1].Name != "staging" || !dbs[1].Empty {
		t.Errorf("dbs[1] = %+v", dbs[1])
	}
}

func TestListDatabases_Error(t *testing.T) {
	mc := &mockClient{listDBsErr: errors.New("auth failed")}
	insp := &Inspector{db: mc}
	_, err := insp.ListDatabases(context.TODO(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, mc.listDBsErr) {
		// Wrapped error
		if err.Error() != "list databases: auth failed" {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestListCollections(t *testing.T) {
	mc := &mockClient{
		collSpecs: []mongo.CollectionSpecification{
			{Name: "users", Type: "collection"},
			{Name: "user_view", Type: "view"},
		},
	}
	insp := &Inspector{db: mc}
	colls, err := insp.ListCollections(context.TODO(), "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(colls) != 2 {
		t.Fatalf("expected 2, got %d", len(colls))
	}
	if colls[0].Name != "users" || colls[0].Database != "app" || colls[0].Type != "collection" {
		t.Errorf("colls[0] = %+v", colls[0])
	}
	if colls[1].Name != "user_view" || colls[1].Type != "view" {
		t.Errorf("colls[1] = %+v", colls[1])
	}
}

func TestListCollections_Error(t *testing.T) {
	mc := &mockClient{collSpecsErr: errors.New("permission denied")}
	insp := &Inspector{db: mc}
	_, err := insp.ListCollections(context.TODO(), "app")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetValidators(t *testing.T) {
	options, err := bson.Marshal(bson.M{
		"validator": bson.M{
			"$jsonSchema": bson.M{
				"required":             bson.A{"email"},
				"additionalProperties": false,
				"properties": bson.M{
					"email": bson.M{"bsonType": "string"},
					"age":   bson.M{"bsonType": bson.A{"int", "null"}},
				},
			},
		},
		"validationLevel":  "strict",
		"validationAction": "error",
	})
	if err != nil {
		t.Fatal(err)
	}

	mc := &mockClient{
		collSpecs: []mongo.CollectionSpecification{
			{Name: "users", Type: "collection", Options: options},
			{Name: "events", Type: "collection"},
		},
	}
	insp := &Inspector{db: mc}

	validators, err := insp.GetValidators(context.TODO(), "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(validators) != 1 {
		t.Fatalf("expected 1 validator, got %d", len(validators))
	}
	v := validators[0]
	if v.Database != "app" || v.Collection != "users" {
		t.Fatalf("validator target = %s.%s, want app.users", v.Database, v.Collection)
	}
	if v.ValidationLevel != "strict" || v.ValidationAction != "error" {
		t.Fatalf("mode = %s/%s, want strict/error", v.ValidationLevel, v.ValidationAction)
	}
	if len(v.Schema.Required) != 1 || v.Schema.Required[0] != "email" {
		t.Fatalf("required = %v, want [email]", v.Schema.Required)
	}
	if v.Schema.AdditionalProperties == nil || *v.Schema.AdditionalProperties {
		t.Fatalf("additionalProperties = %v, want false", v.Schema.AdditionalProperties)
	}
	if got := v.Schema.Properties["email"].BSONTypes; len(got) != 1 || got[0] != "string" {
		t.Fatalf("email bson types = %v, want [string]", got)
	}
	if got := v.Schema.Properties["age"].BSONTypes; len(got) != 2 || got[0] != "number" || got[1] != "null" {
		t.Fatalf("age bson types = %v, want [number null]", got)
	}
}

func TestGetValidators_InvalidOptionsIgnored(t *testing.T) {
	mc := &mockClient{
		collSpecs: []mongo.CollectionSpecification{
			{Name: "users", Type: "collection", Options: bson.Raw{0xFF}},
		},
	}
	insp := &Inspector{db: mc}

	validators, err := insp.GetValidators(context.TODO(), "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(validators) != 0 {
		t.Fatalf("expected 0 validators, got %d", len(validators))
	}
}

func TestGetCollectionStats(t *testing.T) {
	raw, _ := bson.Marshal(bson.M{
		"count":       int64(1000),
		"size":        int64(50000),
		"avgObjSize":  int64(50),
		"storageSize": int64(60000),
	})
	mc := &mockClient{runCmdResult: raw}
	insp := &Inspector{db: mc}
	info, err := insp.GetCollectionStats(context.TODO(), "app", "users")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "users" || info.Database != "app" {
		t.Errorf("name/db = %s/%s", info.Name, info.Database)
	}
	if info.DocCount != 1000 {
		t.Errorf("docCount = %d, want 1000", info.DocCount)
	}
	if info.Size != 50000 {
		t.Errorf("size = %d, want 50000", info.Size)
	}
	if info.AvgObjSize != 50 {
		t.Errorf("avgObjSize = %d, want 50", info.AvgObjSize)
	}
	if info.StorageSize != 60000 {
		t.Errorf("storageSize = %d, want 60000", info.StorageSize)
	}
}

func TestGetCollectionStats_Error(t *testing.T) {
	mc := &mockClient{runCmdErr: errors.New("not found")}
	insp := &Inspector{db: mc}
	info, err := insp.GetCollectionStats(context.TODO(), "app", "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if info.Name != "missing" || info.Database != "app" {
		t.Errorf("should return partial info on error: %+v", info)
	}
}

func TestGetIndexes(t *testing.T) {
	keyDoc, _ := bson.Marshal(bson.D{{Key: "email", Value: 1}})
	unique := true
	sparse := true
	ttlSec := int32(3600)
	mc := &mockClient{
		indexSpecs: []mongo.IndexSpecification{
			{
				Name:               "email_1",
				KeysDocument:       keyDoc,
				Unique:             &unique,
				Sparse:             &sparse,
				ExpireAfterSeconds: &ttlSec,
			},
		},
	}
	insp := &Inspector{db: mc}
	indexes, err := insp.GetIndexes(context.TODO(), "app", "users")
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 1 {
		t.Fatalf("expected 1, got %d", len(indexes))
	}
	idx := indexes[0]
	if idx.Name != "email_1" {
		t.Errorf("name = %s", idx.Name)
	}
	if !idx.Unique || !idx.Sparse {
		t.Errorf("unique=%v sparse=%v", idx.Unique, idx.Sparse)
	}
	if idx.TTL == nil || *idx.TTL != 3600 {
		t.Errorf("ttl = %v", idx.TTL)
	}
	if len(idx.Key) != 1 || idx.Key[0].Field != "email" {
		t.Errorf("key = %+v", idx.Key)
	}
}

func TestGetIndexes_NoOptionalFields(t *testing.T) {
	keyDoc, _ := bson.Marshal(bson.D{{Key: "name", Value: 1}})
	mc := &mockClient{
		indexSpecs: []mongo.IndexSpecification{
			{Name: "name_1", KeysDocument: keyDoc},
		},
	}
	insp := &Inspector{db: mc}
	indexes, err := insp.GetIndexes(context.TODO(), "app", "users")
	if err != nil {
		t.Fatal(err)
	}
	idx := indexes[0]
	if idx.Unique || idx.Sparse || idx.TTL != nil {
		t.Errorf("expected no optional fields: unique=%v sparse=%v ttl=%v", idx.Unique, idx.Sparse, idx.TTL)
	}
}

func TestGetIndexes_Error(t *testing.T) {
	mc := &mockClient{indexSpecsErr: errors.New("fail")}
	insp := &Inspector{db: mc}
	_, err := insp.GetIndexes(context.TODO(), "app", "users")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetIndexStats(t *testing.T) {
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mc := &mockClient{
		aggregateData: []bson.M{
			{
				"name":     "email_1",
				"accesses": bson.M{"ops": int64(500), "since": since},
			},
			{
				"name":     "status_1",
				"accesses": bson.M{"ops": int64(0), "since": since},
			},
			// Edge: missing name
			{"accesses": bson.M{"ops": int64(10)}},
			// Edge: empty name
			{"name": "", "accesses": bson.M{"ops": int64(10)}},
			// Edge: nil accesses
			{"name": "idx_no_accesses"},
		},
	}
	insp := &Inspector{db: mc}
	stats, err := insp.GetIndexStats(context.TODO(), "app", "users")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 stats entries, got %d", len(stats))
	}
	if stats["email_1"].Ops != 500 {
		t.Errorf("email_1 ops = %d", stats["email_1"].Ops)
	}
	if !stats["email_1"].Since.Equal(since) {
		t.Errorf("email_1 since = %v", stats["email_1"].Since)
	}
	if stats["status_1"].Ops != 0 {
		t.Errorf("status_1 ops = %d", stats["status_1"].Ops)
	}
}

func TestGetIndexStats_Error(t *testing.T) {
	mc := &mockClient{aggregateErr: errors.New("aggregate fail")}
	insp := &Inspector{db: mc}
	_, err := insp.GetIndexStats(context.TODO(), "app", "users")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetServerVersion(t *testing.T) {
	raw, _ := bson.Marshal(bson.M{"version": "7.0.5"})
	mc := &mockClient{runCmdResult: raw}
	insp := &Inspector{db: mc}
	info, err := insp.GetServerVersion(context.TODO())
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "7.0.5" {
		t.Errorf("version = %s, want 7.0.5", info.Version)
	}
}

func TestGetServerVersion_Error(t *testing.T) {
	mc := &mockClient{runCmdErr: errors.New("unauthorized")}
	insp := &Inspector{db: mc}
	_, err := insp.GetServerVersion(context.TODO())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClose(t *testing.T) {
	mc := &mockClient{}
	insp := &Inspector{db: mc}
	if err := insp.Close(context.TODO()); err != nil {
		t.Fatal(err)
	}
}

func TestClose_Error(t *testing.T) {
	mc := &mockClient{disconnectErr: errors.New("disconnect fail")}
	insp := &Inspector{db: mc}
	if err := insp.Close(context.TODO()); err == nil {
		t.Fatal("expected error")
	}
}

func TestInspect_SpecificDB(t *testing.T) {
	keyDoc, _ := bson.Marshal(bson.D{{Key: "_id", Value: 1}})
	statsRaw, _ := bson.Marshal(bson.M{
		"count": int64(500), "size": int64(10000), "avgObjSize": int64(20), "storageSize": int64(15000),
	})
	since := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	mc := &mockClient{
		collSpecs: []mongo.CollectionSpecification{
			{Name: "users", Type: "collection"},
			{Name: "user_view", Type: "view"},
		},
		runCmdResult: statsRaw,
		indexSpecs: []mongo.IndexSpecification{
			{Name: "_id_", KeysDocument: keyDoc},
		},
		aggregateData: []bson.M{
			{"name": "_id_", "accesses": bson.M{"ops": int64(100), "since": since}},
		},
	}
	insp := &Inspector{db: mc}
	colls, err := insp.Inspect(context.TODO(), "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(colls) != 2 {
		t.Fatalf("expected 2 collections, got %d", len(colls))
	}

	// View should be passed through without stats.
	view := colls[1]
	if view.Name != "user_view" || view.Type != "view" {
		t.Errorf("expected view, got %+v", view)
	}
	if view.DocCount != 0 {
		t.Errorf("view should have no stats, docCount = %d", view.DocCount)
	}

	// Regular collection should have stats and indexes.
	users := colls[0]
	if users.DocCount != 500 {
		t.Errorf("docCount = %d, want 500", users.DocCount)
	}
	if len(users.Indexes) != 1 {
		t.Fatalf("expected 1 index, got %d", len(users.Indexes))
	}
	if users.Indexes[0].Stats == nil || users.Indexes[0].Stats.Ops != 100 {
		t.Errorf("index stats = %+v", users.Indexes[0].Stats)
	}
}

func TestInspect_ListCollectionsError(t *testing.T) {
	mc := &mockClient{collSpecsErr: errors.New("fail")}
	insp := &Inspector{db: mc}
	_, err := insp.Inspect(context.TODO(), "app")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInspect_StatsErrorGraceful(t *testing.T) {
	mc := &mockClient{
		collSpecs: []mongo.CollectionSpecification{
			{Name: "users", Type: "collection"},
		},
		runCmdErr:     errors.New("stats fail"),
		indexSpecsErr: errors.New("index fail"),
	}
	insp := &Inspector{db: mc}
	colls, err := insp.Inspect(context.TODO(), "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(colls) != 1 {
		t.Fatalf("expected 1, got %d", len(colls))
	}
	// Stats errors are gracefully ignored.
	if colls[0].DocCount != 0 {
		t.Errorf("docCount should be 0 on stats error, got %d", colls[0].DocCount)
	}
}

func TestInspect_ListDBsError(t *testing.T) {
	mc := &mockClient{listDBsErr: errors.New("auth fail")}
	insp := &Inspector{db: mc}
	_, err := insp.Inspect(context.TODO(), "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInspectUsers(t *testing.T) {
	raw, err := bson.Marshal(bson.M{
		"users": bson.A{
			bson.M{
				"user": "admin",
				"db":   "admin",
				"roles": bson.A{
					bson.M{"role": "root", "db": "admin"},
				},
			},
			bson.M{
				"user": "appUser",
				"db":   "myapp",
				"roles": bson.A{
					bson.M{"role": "readWrite", "db": "myapp"},
					bson.M{"role": "read", "db": "reporting"},
				},
			},
		},
		"ok": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	mc := &mockClient{runCmdResult: raw}
	insp := &Inspector{db: mc}
	users, err := insp.InspectUsers(context.TODO(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}

	if users[0].Username != "admin" || users[0].Database != "admin" {
		t.Errorf("users[0] = %+v", users[0])
	}
	if len(users[0].Roles) != 1 || users[0].Roles[0].Role != "root" {
		t.Errorf("users[0].Roles = %+v", users[0].Roles)
	}

	if users[1].Username != "appUser" || users[1].Database != "myapp" {
		t.Errorf("users[1] = %+v", users[1])
	}
	if len(users[1].Roles) != 2 {
		t.Errorf("expected 2 roles for appUser, got %d", len(users[1].Roles))
	}
}

func TestInspectUsers_Empty(t *testing.T) {
	raw, err := bson.Marshal(bson.M{
		"users": bson.A{},
		"ok":    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	mc := &mockClient{runCmdResult: raw}
	insp := &Inspector{db: mc}
	users, err := insp.InspectUsers(context.TODO(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestInspectUsers_Error(t *testing.T) {
	mc := &mockClient{runCmdErr: errors.New("unauthorized")}
	insp := &Inspector{db: mc}
	_, err := insp.InspectUsers(context.TODO(), "admin")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInspectSharding_NonShardedDeployment(t *testing.T) {
	mc := &mockClient{
		runCmdHook: func(dbName string, cmd any) (bson.Raw, error) {
			return nil, mongo.CommandError{Code: 26, Message: "NamespaceNotFound: config.shards"}
		},
	}
	insp := &Inspector{db: mc}

	info, err := insp.InspectSharding(context.TODO())
	if err != nil {
		t.Fatalf("InspectSharding returned error for non-sharded deployment: %v", err)
	}
	if info.Enabled {
		t.Fatalf("expected sharding disabled, got %+v", info)
	}
	if info.BalancerEnabled {
		t.Fatalf("balancer should be false when sharding is disabled")
	}
}

func TestInspectSharding(t *testing.T) {
	eventsChunks := make([]bson.M, 0, 10)
	for i := 0; i < 9; i++ {
		eventsChunks = append(eventsChunks, bson.M{"shard": "shardA"})
	}
	eventsChunks = append(eventsChunks, bson.M{"shard": "shardB", "jumbo": true})

	mc := &mockClient{
		runCmdHook: func(dbName string, cmd any) (bson.Raw, error) {
			command, ok := cmd.(bson.D)
			if !ok {
				return nil, errors.New("expected bson.D command")
			}

			commandName := command[0].Key
			if commandName != "find" {
				return nil, errors.New("unexpected command")
			}

			collName := toString(lookupBSONValue(command, "find"))
			filter := toBsonM(lookupBSONValue(command, "filter"))

			switch collName {
			case "shards":
				return mustMarshalRaw(t, bson.M{
					"cursor": bson.M{
						"id":         int64(0),
						"firstBatch": []bson.M{{"_id": "shardA"}, {"_id": "shardB"}},
					},
				}), nil
			case "collections":
				return mustMarshalRaw(t, bson.M{
					"cursor": bson.M{
						"id": int64(0),
						"firstBatch": []bson.M{
							{"_id": "app.events", "key": bson.D{{Key: "_id", Value: 1}}},
							{"_id": "app.orders", "key": bson.D{{Key: "customer_id", Value: 1}}},
						},
					},
				}), nil
			case "chunks":
				ns := toString(filter["ns"])
				switch ns {
				case "app.events":
					return mustMarshalRaw(t, bson.M{
						"cursor": bson.M{
							"id":         int64(0),
							"firstBatch": eventsChunks,
						},
					}), nil
				case "app.orders":
					return mustMarshalRaw(t, bson.M{
						"cursor": bson.M{
							"id": int64(0),
							"firstBatch": []bson.M{
								{"shard": "shardA"},
								{"shard": "shardA"},
								{"shard": "shardB"},
								{"shard": "shardB"},
							},
						},
					}), nil
				default:
					return nil, errors.New("unexpected chunks filter")
				}
			case "settings":
				return mustMarshalRaw(t, bson.M{
					"cursor": bson.M{
						"id":         int64(0),
						"firstBatch": []bson.M{{"_id": "balancer", "stopped": true}},
					},
				}), nil
			default:
				return nil, errors.New("unexpected collection command")
			}
		},
	}
	insp := &Inspector{db: mc}

	info, err := insp.InspectSharding(context.TODO())
	if err != nil {
		t.Fatalf("InspectSharding returned error: %v", err)
	}
	if !info.Enabled {
		t.Fatal("expected sharding to be enabled")
	}
	if info.BalancerEnabled {
		t.Fatal("expected balancer to be disabled")
	}
	if len(info.Shards) != 2 {
		t.Fatalf("expected 2 shards, got %d", len(info.Shards))
	}
	if len(info.Collections) != 2 {
		t.Fatalf("expected 2 sharded collections, got %d", len(info.Collections))
	}

	events, ok := findShardedCollection(info.Collections, "app.events")
	if !ok {
		t.Fatalf("missing app.events sharding metadata")
	}
	if events.ChunkCount != 10 {
		t.Fatalf("events chunk count = %d, want 10", events.ChunkCount)
	}
	if events.JumboChunks != 1 {
		t.Fatalf("events jumbo chunks = %d, want 1", events.JumboChunks)
	}
	if events.ChunkDistribution["shardA"] != 9 || events.ChunkDistribution["shardB"] != 1 {
		t.Fatalf("events distribution = %+v", events.ChunkDistribution)
	}
}

func TestReadProfiler(t *testing.T) {
	newest := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	older := newest.Add(-2 * time.Minute)

	mc := &mockClient{
		runCmdHook: func(dbName string, cmd any) (bson.Raw, error) {
			command, ok := cmd.(bson.D)
			if !ok {
				return nil, errors.New("expected bson.D command")
			}
			if toString(lookupBSONValue(command, "find")) != "system.profile" {
				return nil, errors.New("unexpected collection read")
			}
			if toInt64(lookupBSONValue(command, "limit")) != 10 {
				return nil, errors.New("missing limit in profile query")
			}

			sortDoc, ok := lookupBSONValue(command, "sort").(bson.D)
			if !ok || len(sortDoc) != 1 || sortDoc[0].Key != "ts" || toInt64(sortDoc[0].Value) != -1 {
				return nil, errors.New("missing ts sort in profile query")
			}

			return mustMarshalRaw(t, bson.M{
				"cursor": bson.M{
					"id": int64(0),
					"firstBatch": []bson.M{
						{
							"ns": "app.users",
							"command": bson.M{
								"find":       "users",
								"filter":     bson.M{"status": "active", "profile": bson.M{"verified": true}},
								"sort":       bson.M{"created_at": -1},
								"projection": bson.M{"email": 1},
							},
							"millis":      int64(850),
							"ts":          bson.DateTime(older.UnixMilli()),
							"planSummary": "COLLSCAN",
						},
						{
							"ns": "app.users",
							"command": bson.M{
								"find":   "users",
								"filter": bson.M{"email": bson.M{"$eq": "alice@example.com"}},
							},
							"durationMillis": int64(120),
							"ts":             bson.DateTime(newest.UnixMilli()),
							"planSummary":    "IXSCAN { email: 1 }",
						},
					},
				},
			}), nil
		},
	}
	insp := &Inspector{db: mc}

	entries, err := insp.ReadProfiler(context.TODO(), "app", 10)
	if err != nil {
		t.Fatalf("ReadProfiler: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}

	if !entries[0].Timestamp.Equal(newest) {
		t.Fatalf("entries[0] timestamp = %v, want %v", entries[0].Timestamp, newest)
	}
	if entries[0].DurationMillis != 120 {
		t.Fatalf("entries[0] duration = %d, want 120", entries[0].DurationMillis)
	}
	if !containsString(entries[0].FilterFields, "email") {
		t.Fatalf("entries[0] filter fields = %v, want email", entries[0].FilterFields)
	}

	if entries[1].DurationMillis != 850 {
		t.Fatalf("entries[1] duration = %d, want 850", entries[1].DurationMillis)
	}
	if !containsString(entries[1].FilterFields, "status") || !containsString(entries[1].FilterFields, "profile.verified") {
		t.Fatalf("entries[1] filter fields = %v, want status and profile.verified", entries[1].FilterFields)
	}
	if !containsString(entries[1].SortFields, "created_at") {
		t.Fatalf("entries[1] sort fields = %v, want created_at", entries[1].SortFields)
	}
	if !containsString(entries[1].ProjectionFields, "email") {
		t.Fatalf("entries[1] projection fields = %v, want email", entries[1].ProjectionFields)
	}
}

func TestReadProfiler_ProfilerDisabled(t *testing.T) {
	mc := &mockClient{
		runCmdHook: func(string, any) (bson.Raw, error) {
			return nil, mongo.CommandError{Code: 26, Message: "NamespaceNotFound: app.system.profile"}
		},
	}
	insp := &Inspector{db: mc}

	entries, err := insp.ReadProfiler(context.TODO(), "app", 100)
	if err != nil {
		t.Fatalf("ReadProfiler returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty profiler entries when disabled, got %d", len(entries))
	}
}

func lookupBSONValue(doc bson.D, key string) any {
	for _, elem := range doc {
		if elem.Key == key {
			return elem.Value
		}
	}
	return nil
}

func mustMarshalRaw(t *testing.T, v any) bson.Raw {
	t.Helper()
	raw, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("marshal bson: %v", err)
	}
	return raw
}

func findShardedCollection(collections []ShardedCollectionInfo, namespace string) (ShardedCollectionInfo, bool) {
	for _, coll := range collections {
		if coll.Namespace == namespace {
			return coll, true
		}
	}
	return ShardedCollectionInfo{}, false
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
