package scanner

import "testing"

func TestScanLine_GoDriver(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{`coll := db.Collection("products")`, "products"},
		{`c := db.Collection( "orders" )`, "orders"},
		{`db.Collection("users").Find(ctx, filter)`, "users"},
	}
	for _, tt := range tests {
		matches := ScanLine(tt.line)
		if len(matches) != 1 {
			t.Errorf("ScanLine(%q) got %d matches, want 1", tt.line, len(matches))
			continue
		}
		if matches[0].Collection != tt.want {
			t.Errorf("ScanLine(%q) = %q, want %q", tt.line, matches[0].Collection, tt.want)
		}
		if matches[0].Pattern != PatternDriverCall {
			t.Errorf("pattern = %s, want %s", matches[0].Pattern, PatternDriverCall)
		}
	}
}

func TestScanLine_JSDriver(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{`db.collection("users")`, "users"},
		{`db.collection('sessions')`, "sessions"},
		{`db.getCollection("logs")`, "logs"},
		{`db.GetCollection("events")`, "events"},
	}
	for _, tt := range tests {
		matches := ScanLine(tt.line)
		if len(matches) != 1 {
			t.Errorf("ScanLine(%q) got %d matches, want 1", tt.line, len(matches))
			continue
		}
		if matches[0].Collection != tt.want {
			t.Errorf("ScanLine(%q) = %q, want %q", tt.line, matches[0].Collection, tt.want)
		}
	}
}

func TestScanLine_Mongoose(t *testing.T) {
	tests := []struct {
		line       string
		wantModel  string
		wantPlural string
	}{
		{`mongoose.model("User", userSchema)`, "User", "users"},
		{`mongoose.model('Order', orderSchema)`, "Order", "orders"},
		{`const Cat = model("Cat", catSchema)`, "Cat", "cats"},
	}
	for _, tt := range tests {
		matches := ScanLine(tt.line)
		if len(matches) != 2 {
			t.Errorf("ScanLine(%q) got %d matches, want 2", tt.line, len(matches))
			continue
		}
		if matches[0].Collection != tt.wantModel {
			t.Errorf("ScanLine(%q) model = %q, want %q", tt.line, matches[0].Collection, tt.wantModel)
		}
		if matches[1].Collection != tt.wantPlural {
			t.Errorf("ScanLine(%q) plural = %q, want %q", tt.line, matches[1].Collection, tt.wantPlural)
		}
		for i, m := range matches {
			if m.Pattern != PatternORM {
				t.Errorf("matches[%d].pattern = %s, want %s", i, m.Pattern, PatternORM)
			}
		}
	}
}

func TestMongoosePluralize(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"User", "users"},
		{"Order", "orders"},
		{"Cat", "cats"},
		{"Address", "addresses"},   // ends with "ss" -> "es"
		{"Bus", "buss"},            // naive; only "ss" triggers "es"
		{"Glass", "glasses"},       // -ss -> -sses
		{"Dish", "dishes"},         // -sh -> -shes
		{"Match", "matches"},       // -ch -> -ches
		{"Box", "boxes"},           // -x -> -xes
		{"Category", "categories"}, // consonant+y -> -ies
		{"Monkey", "monkeys"},      // vowel+y -> -ys
	}
	for _, tt := range tests {
		got := mongoosePluralize(tt.model)
		if got != tt.want {
			t.Errorf("mongoosePluralize(%q) = %q, want %q", tt.model, got, tt.want)
		}
	}
}

func TestScanLine_MongoEngine(t *testing.T) {
	line := `meta = {'collection': 'audit_logs'}`
	matches := ScanLine(line)
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Collection != "audit_logs" {
		t.Errorf("got %q, want audit_logs", matches[0].Collection)
	}
}

func TestScanLine_BracketAccess(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{`db["users"].find({})`, "users"},
		{`db['orders'].insert_one(doc)`, "orders"},
	}
	for _, tt := range tests {
		matches := ScanLine(tt.line)
		if len(matches) != 1 {
			t.Errorf("ScanLine(%q) got %d matches, want 1", tt.line, len(matches))
			continue
		}
		if matches[0].Collection != tt.want {
			t.Errorf("got %q, want %q", matches[0].Collection, tt.want)
		}
		if matches[0].Pattern != PatternBracket {
			t.Errorf("pattern = %s, want %s", matches[0].Pattern, PatternBracket)
		}
	}
}

func TestScanLine_PyMongoDotAccess(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{`db.users.find({"status": "active"})`, "users"},
		{`db.orders.insert_one(doc)`, "orders"},
		{`db.events.aggregate(pipeline)`, "events"},
		{`result = db.products.count_documents({})`, "products"},
	}
	for _, tt := range tests {
		matches := ScanLine(tt.line)
		if len(matches) != 1 {
			t.Errorf("ScanLine(%q) got %d matches, want 1", tt.line, len(matches))
			continue
		}
		if matches[0].Collection != tt.want {
			t.Errorf("got %q, want %q", matches[0].Collection, tt.want)
		}
		if matches[0].Pattern != PatternDotAccess {
			t.Errorf("pattern = %s, want %s", matches[0].Pattern, PatternDotAccess)
		}
	}
}

func TestScanLine_NoMatch(t *testing.T) {
	lines := []string{
		`fmt.Println("hello")`,
		`// just a comment`,
		`x := 42`,
		`db.users.status`, // no mongo operation following
		``,
	}
	for _, line := range lines {
		if matches := ScanLine(line); len(matches) != 0 {
			t.Errorf("ScanLine(%q) = %v, want no matches", line, matches)
		}
	}
}

func TestScanLine_SkipsInvalid(t *testing.T) {
	lines := []string{
		`db.collection("${varName}")`,         // template variable
		`db.collection("/path/to/something")`, // path
	}
	for _, line := range lines {
		if matches := ScanLine(line); len(matches) != 0 {
			t.Errorf("ScanLine(%q) should skip invalid, got %v", line, matches)
		}
	}
}

func TestIsValidCollectionName(t *testing.T) {
	if isValidCollectionName("") {
		t.Error("empty should be invalid")
	}
	if isValidCollectionName("${var}") {
		t.Error("template should be invalid")
	}
	if !isValidCollectionName("users") {
		t.Error("users should be valid")
	}
	if !isValidCollectionName("audit_logs") {
		t.Error("audit_logs should be valid")
	}
}
