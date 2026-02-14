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
	if m.runCmdErr != nil || m.runCmdResult == nil {
		// Return a SingleResult that will error on Decode.
		return mongo.NewSingleResultFromDocument(nil, m.runCmdErr, nil)
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
