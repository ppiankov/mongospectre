package analyzer

import (
	"fmt"
	"testing"
	"time"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

func makeCollections(n int, indexesPerColl int) []mongoinspect.CollectionInfo {
	collections := make([]mongoinspect.CollectionInfo, n)
	since := time.Now().Add(-30 * 24 * time.Hour)
	for i := range collections {
		indexes := make([]mongoinspect.IndexInfo, indexesPerColl)
		for j := range indexes {
			ops := int64(100)
			if j%5 == 0 {
				ops = 0 // Some unused
			}
			indexes[j] = mongoinspect.IndexInfo{
				Name: fmt.Sprintf("idx_%d_%d", i, j),
				Key:  []mongoinspect.KeyField{{Field: fmt.Sprintf("field_%d", j), Direction: 1}},
				Stats: &mongoinspect.IndexStats{
					Ops:   ops,
					Since: since,
				},
			}
		}
		collections[i] = mongoinspect.CollectionInfo{
			Name:     fmt.Sprintf("coll_%d", i),
			Database: "testdb",
			Type:     "collection",
			DocCount: int64(i * 1000),
			Indexes:  indexes,
		}
	}
	return collections
}

func BenchmarkAudit_100Collections(b *testing.B) {
	collections := makeCollections(100, 5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Audit(collections)
	}
}

func BenchmarkAudit_1000Collections(b *testing.B) {
	collections := makeCollections(1000, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Audit(collections)
	}
}

func BenchmarkDiff_100Collections(b *testing.B) {
	collections := makeCollections(100, 5)
	scan := &scanner.ScanResult{
		RepoPath: "/test",
		Collections: func() []string {
			names := make([]string, 50)
			for i := range names {
				names[i] = fmt.Sprintf("coll_%d", i*2)
			}
			return names
		}(),
		Refs: func() []scanner.CollectionRef {
			refs := make([]scanner.CollectionRef, 50)
			for i := range refs {
				refs[i] = scanner.CollectionRef{
					Collection: fmt.Sprintf("coll_%d", i*2),
					File:       "app.go",
					Line:       i + 1,
				}
			}
			return refs
		}(),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Diff(scan, collections)
	}
}

func BenchmarkDiffBaseline(b *testing.B) {
	current := make([]Finding, 500)
	baseline := make([]Finding, 500)
	for i := range current {
		current[i] = Finding{
			Type:       FindingUnusedIndex,
			Severity:   SeverityMedium,
			Database:   "db",
			Collection: fmt.Sprintf("coll_%d", i),
			Index:      fmt.Sprintf("idx_%d", i),
			Message:    fmt.Sprintf("index %d unused", i),
		}
		// Offset baseline so ~half are new, ~half are unchanged.
		baseline[i] = Finding{
			Type:       FindingUnusedIndex,
			Severity:   SeverityMedium,
			Database:   "db",
			Collection: fmt.Sprintf("coll_%d", i+250),
			Index:      fmt.Sprintf("idx_%d", i+250),
			Message:    fmt.Sprintf("index %d unused", i+250),
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DiffBaseline(current, baseline)
	}
}
