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

func TestScanLineFields_SortStage(t *testing.T) {
	line := `{"$sort": {"created_at": -1, "name": 1}}`
	got := fieldNames(ScanLineFields(line))
	if len(got) < 1 {
		t.Fatalf("expected at least 1 field, got %v", got)
	}
	found := make(map[string]bool)
	for _, f := range got {
		found[f] = true
	}
	if !found["created_at"] {
		t.Error("missing created_at")
	}
}

func TestScanLineFields_ProjectStage(t *testing.T) {
	line := `{"$project": {"name": 1, "email": 1, "_id": 0}}`
	got := fieldNames(ScanLineFields(line))
	found := make(map[string]bool)
	for _, f := range got {
		found[f] = true
	}
	if !found["name"] {
		t.Error("missing name from $project")
	}
	if !found["email"] {
		t.Error("missing email from $project")
	}
}

func TestScanLineFields_GroupStage(t *testing.T) {
	line := `{"$group": {"_id": "$category", "total": {"$sum": "$amount"}}}`
	got := fieldNames(ScanLineFields(line))
	found := make(map[string]bool)
	for _, f := range got {
		found[f] = true
	}
	if !found["category"] {
		t.Errorf("missing category from $group _id, got %v", got)
	}
	if !found["amount"] {
		t.Errorf("missing amount from $sum, got %v", got)
	}
}

func TestScanLineFields_Unwind(t *testing.T) {
	line := `{"$unwind": "$items"}`
	got := fieldNames(ScanLineFields(line))
	if len(got) != 1 || got[0] != "items" {
		t.Errorf("ScanLineFields($unwind) got %v, want [items]", got)
	}
}

func TestScanLineFields_LookupFields(t *testing.T) {
	line := `{"$lookup": {"from": "users", "localField": "userId", "foreignField": "_id", "as": "user"}}`
	got := fieldNames(ScanLineFields(line))
	found := make(map[string]bool)
	for _, f := range got {
		found[f] = true
	}
	if !found["userId"] {
		t.Errorf("missing userId from $lookup localField, got %v", got)
	}
	// _id is valid to extract (filtered by analyzer, not scanner)
	// "from", "as", "localField", "foreignField" should NOT appear as field names
	for _, bad := range []string{"from", "as", "localField", "foreignField"} {
		if found[bad] {
			t.Errorf("%q should not appear as a field name", bad)
		}
	}
}

func TestScanLine_LookupFrom(t *testing.T) {
	line := `{"$lookup": {"from": "users", "localField": "userId", "foreignField": "_id"}}`
	matches := ScanLine(line)
	found := false
	for _, m := range matches {
		if m.Collection == "users" {
			found = true
		}
	}
	if !found {
		t.Errorf("$lookup from should produce collection ref 'users', got %v", matches)
	}
}

func TestScanLineFields_AddFieldsStage(t *testing.T) {
	line := `{"$addFields": {"fullName": {"$concat": ["$firstName", " ", "$lastName"]}}}`
	got := fieldNames(ScanLineFields(line))
	found := make(map[string]bool)
	for _, f := range got {
		found[f] = true
	}
	if !found["firstName"] {
		t.Errorf("missing firstName, got %v", got)
	}
	if !found["lastName"] {
		t.Errorf("missing lastName, got %v", got)
	}
}

func TestScanLineFields_GoPipeline(t *testing.T) {
	line := `bson.D{{Key: "$match", Value: bson.M{"status": "active"}}}`
	got := fieldNames(ScanLineFields(line))
	found := make(map[string]bool)
	for _, f := range got {
		found[f] = true
	}
	if !found["status"] {
		t.Errorf("missing status from Go pipeline $match, got %v", got)
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
