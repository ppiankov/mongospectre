package scanner

import "testing"

func writeFieldTypes(matches []writeFieldMatch) map[string]string {
	out := make(map[string]string)
	for _, m := range matches {
		out[m.Field] = m.ValueType
	}
	return out
}

func TestIsWriteOperation(t *testing.T) {
	if !IsWriteOperation(`db.collection("users").insertOne({"email":"a"})`) {
		t.Fatal("expected insertOne to be detected as write operation")
	}
	if IsWriteOperation(`db.collection("users").find({"email":"a"})`) {
		t.Fatal("find should not be detected as write operation")
	}
}

func TestScanLineWriteFields_InsertOneJS(t *testing.T) {
	line := `db.collection("users").insertOne({"email": "a@x.com", "active": true, "age": 42, "prefs": {"theme": "dark"}, "tags": ["a"]})`
	got := writeFieldTypes(ScanLineWriteFields(line))

	if got["email"] != ValueTypeString {
		t.Fatalf("email type = %q, want string", got["email"])
	}
	if got["active"] != ValueTypeBool {
		t.Fatalf("active type = %q, want bool", got["active"])
	}
	if got["age"] != ValueTypeNumber {
		t.Fatalf("age type = %q, want number", got["age"])
	}
	if got["prefs"] != ValueTypeObject {
		t.Fatalf("prefs type = %q, want object", got["prefs"])
	}
	if got["tags"] != ValueTypeArray {
		t.Fatalf("tags type = %q, want array", got["tags"])
	}
}

func TestScanLineWriteFields_UpdateOneUsesSetPayload(t *testing.T) {
	line := `db.collection("users").updateOne({"_id": id}, {"$set": {"email": "x@y.com", "profile": {"verified": true}}})`
	got := writeFieldTypes(ScanLineWriteFields(line))

	if got["email"] != ValueTypeString {
		t.Fatalf("email type = %q, want string", got["email"])
	}
	if got["profile"] != ValueTypeObject {
		t.Fatalf("profile type = %q, want object", got["profile"])
	}
	if _, ok := got["_id"]; ok {
		t.Fatalf("filter field _id should not be treated as write field: %+v", got)
	}
}

func TestScanLineWriteFields_InsertOneGo(t *testing.T) {
	line := `coll.InsertOne(ctx, bson.M{"email": "a@x.com", "profile": bson.M{"verified": true}})`
	got := writeFieldTypes(ScanLineWriteFields(line))

	if got["email"] != ValueTypeString {
		t.Fatalf("email type = %q, want string", got["email"])
	}
	if got["profile"] != ValueTypeObject {
		t.Fatalf("profile type = %q, want object", got["profile"])
	}
}
