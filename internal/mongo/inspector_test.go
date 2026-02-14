package mongo

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

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

func TestBsonDToKeyMap(t *testing.T) {
	raw, err := bson.Marshal(bson.D{{Key: "name", Value: 1}, {Key: "age", Value: -1}})
	if err != nil {
		t.Fatal(err)
	}
	m := bsonDToKeyMap(raw)

	if len(m) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(m))
	}
	if m["name"] != 1 {
		t.Errorf("name = %d, want 1", m["name"])
	}
	if m["age"] != -1 {
		t.Errorf("age = %d, want -1", m["age"])
	}
}

func TestBsonDToKeyMap_Empty(t *testing.T) {
	raw, err := bson.Marshal(bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	m := bsonDToKeyMap(raw)
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestBsonDToKeyMap_Invalid(t *testing.T) {
	m := bsonDToKeyMap(bson.Raw{0xFF})
	if m != nil {
		t.Errorf("expected nil for invalid raw, got %v", m)
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

func TestListDatabases_SpecificDB(t *testing.T) {
	// When a specific database is provided, ListDatabases should return it directly
	// without needing a real connection. We test the logic, not the MongoDB call.
	insp := &Inspector{} // nil client is fine — we won't hit MongoDB
	dbs, err := insp.ListDatabases(nil, "mydb")
	if err != nil {
		t.Fatal(err)
	}
	if len(dbs) != 1 || dbs[0].Name != "mydb" {
		t.Errorf("expected [{Name:mydb}], got %+v", dbs)
	}
}
