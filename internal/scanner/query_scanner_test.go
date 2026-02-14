package scanner

import (
	"sort"
	"testing"
)

func fieldNames(matches []fieldMatch) []string {
	var names []string
	for _, m := range matches {
		names = append(names, m.Field)
	}
	sort.Strings(names)
	return names
}

func TestScanLineFields_GoBsonM(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{`coll.Find(ctx, bson.M{"status": "active"})`, []string{"status"}},
		{`coll.Find(ctx, bson.M{"status": "active", "created_at": bson.M{"$gt": t}})`, []string{"created_at", "status"}},
		{`coll.Find(ctx, bson.D{{Key: "email", Value: "test@example.com"}})`, []string{"email"}},
	}
	for _, tt := range tests {
		got := fieldNames(ScanLineFields(tt.line))
		if len(got) != len(tt.want) {
			t.Errorf("ScanLineFields(%q) got %v, want %v", tt.line, got, tt.want)
			continue
		}
		for i, f := range got {
			if f != tt.want[i] {
				t.Errorf("ScanLineFields(%q)[%d] = %q, want %q", tt.line, i, f, tt.want[i])
			}
		}
	}
}

func TestScanLineFields_JSFind(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{`db.collection("users").find({"email": 1})`, []string{"email"}},
		{`db.collection("users").findOne({"status": "active"})`, []string{"status"}},
		{`collection.updateOne({"_id": id, "status": "pending"})`, []string{"_id", "status"}},
		{`coll.countDocuments({"type": "admin"})`, []string{"type"}},
	}
	for _, tt := range tests {
		got := fieldNames(ScanLineFields(tt.line))
		if len(got) != len(tt.want) {
			t.Errorf("ScanLineFields(%q) got %v, want %v", tt.line, got, tt.want)
			continue
		}
		for i, f := range got {
			if f != tt.want[i] {
				t.Errorf("ScanLineFields(%q)[%d] = %q, want %q", tt.line, i, f, tt.want[i])
			}
		}
	}
}

func TestScanLineFields_PythonFind(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{`db.users.find({"status": "active"})`, []string{"status"}},
		{`collection.find_one({"email": "test@example.com"})`, []string{"email"}},
		{`db.orders.count_documents({"status": "pending"})`, []string{"status"}},
	}
	for _, tt := range tests {
		got := fieldNames(ScanLineFields(tt.line))
		if len(got) != len(tt.want) {
			t.Errorf("ScanLineFields(%q) got %v, want %v", tt.line, got, tt.want)
			continue
		}
		for i, f := range got {
			if f != tt.want[i] {
				t.Errorf("ScanLineFields(%q)[%d] = %q, want %q", tt.line, i, f, tt.want[i])
			}
		}
	}
}

func TestScanLineFields_MatchStage(t *testing.T) {
	line := `pipeline := []bson.M{{"$match": {"status": "active"}}}`
	got := fieldNames(ScanLineFields(line))
	if len(got) != 1 || got[0] != "status" {
		t.Errorf("ScanLineFields($match) got %v, want [status]", got)
	}
}

func TestScanLineFields_Sort(t *testing.T) {
	line := `cursor.sort({"created_at": -1})`
	got := fieldNames(ScanLineFields(line))
	if len(got) != 1 || got[0] != "created_at" {
		t.Errorf("ScanLineFields(sort) got %v, want [created_at]", got)
	}
}

func TestScanLineFields_NoMatch(t *testing.T) {
	lines := []string{
		`fmt.Println("hello")`,
		`x := 42`,
		`// just a comment`,
		``,
	}
	for _, line := range lines {
		if matches := ScanLineFields(line); len(matches) != 0 {
			t.Errorf("ScanLineFields(%q) = %v, want no matches", line, matches)
		}
	}
}

func TestScanLineFields_SkipsOperators(t *testing.T) {
	// $gt, $in etc. should not appear as field names
	line := `coll.Find(ctx, bson.M{"age": bson.M{"$gt": 18}})`
	got := fieldNames(ScanLineFields(line))
	for _, f := range got {
		if f[0] == '$' {
			t.Errorf("operator %q should be filtered", f)
		}
	}
}

func TestScanLineFields_DottedField(t *testing.T) {
	line := `coll.Find(ctx, bson.M{"address.city": "NYC"})`
	got := fieldNames(ScanLineFields(line))
	if len(got) != 1 || got[0] != "address.city" {
		t.Errorf("ScanLineFields(dotted) got %v, want [address.city]", got)
	}
}

func TestIsValidFieldName(t *testing.T) {
	if isValidFieldName("") {
		t.Error("empty should be invalid")
	}
	if isValidFieldName("$gt") {
		t.Error("operator should be invalid")
	}
	if isValidFieldName("find") {
		t.Error("method name should be invalid")
	}
	if !isValidFieldName("status") {
		t.Error("status should be valid")
	}
	if !isValidFieldName("created_at") {
		t.Error("created_at should be valid")
	}
	if !isValidFieldName("address.city") {
		t.Error("dotted field should be valid")
	}
}
