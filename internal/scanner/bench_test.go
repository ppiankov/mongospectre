package scanner

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkScanLine(b *testing.B) {
	lines := []string{
		`db.collection("users").find({"email": "test@example.com"})`,
		`coll.Find(ctx, bson.M{"status": "active", "created_at": bson.M{"$gt": t}})`,
		`db.orders.find({"status": "pending", "amount": {"$gte": 100}})`,
		`collection.updateOne({"_id": id, "status": "pending"})`,
		`fmt.Println("hello world")`,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			ScanLine(line)
		}
	}
}

func BenchmarkScanLineFields(b *testing.B) {
	lines := []string{
		`coll.Find(ctx, bson.M{"status": "active", "created_at": bson.M{"$gt": t}})`,
		`{"$group": {"_id": "$category", "total": {"$sum": "$amount"}}}`,
		`{"$lookup": {"from": "users", "localField": "userId", "foreignField": "_id", "as": "user"}}`,
		`{"$sort": {"created_at": -1, "name": 1}}`,
		`x := 42`,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			ScanLineFields(line)
		}
	}
}

func BenchmarkScanLine_LargeFile(b *testing.B) {
	// Simulate scanning a 10k-line file with ~5% MongoDB lines.
	var lines []string
	for i := 0; i < 9500; i++ {
		lines = append(lines, fmt.Sprintf(`    result[%d] = process(data[%d])`, i, i))
	}
	for i := 0; i < 500; i++ {
		lines = append(lines, fmt.Sprintf(`    db.collection("coll_%d").find({"field_%d": 1})`, i, i))
	}
	file := strings.Join(lines, "\n")
	allLines := strings.Split(file, "\n")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range allLines {
			ScanLine(line)
			ScanLineFields(line)
		}
	}
}
