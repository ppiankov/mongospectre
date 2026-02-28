package analyzer

import (
	"fmt"
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

func kf(fields ...string) []mongoinspect.KeyField {
	out := make([]mongoinspect.KeyField, 0, len(fields))
	for _, f := range fields {
		dir := 1
		if f[0] == '-' {
			dir = -1
			f = f[1:]
		}
		out = append(out, mongoinspect.KeyField{Field: f, Direction: dir})
	}
	return out
}

func idx(name string, key []mongoinspect.KeyField, ops int64) mongoinspect.IndexInfo {
	stats := &mongoinspect.IndexStats{Ops: ops}
	return mongoinspect.IndexInfo{Name: name, Key: key, Stats: stats}
}

func idxTTL(name string, key []mongoinspect.KeyField, ttlSec int32) mongoinspect.IndexInfo {
	return mongoinspect.IndexInfo{Name: name, Key: key, TTL: &ttlSec}
}

func TestDetectUnusedCollection(t *testing.T) {
	tests := []struct {
		name  string
		coll  mongoinspect.CollectionInfo
		count int
	}{
		{
			name:  "empty collection",
			coll:  mongoinspect.CollectionInfo{Name: "empty", Database: "db", DocCount: 0},
			count: 1,
		},
		{
			name:  "non-empty collection",
			coll:  mongoinspect.CollectionInfo{Name: "users", Database: "db", DocCount: 100},
			count: 0,
		},
		{
			name:  "view is skipped",
			coll:  mongoinspect.CollectionInfo{Name: "v", Database: "db", Type: "view", DocCount: 0},
			count: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectUnusedCollection(&tt.coll)
			if len(got) != tt.count {
				t.Errorf("expected %d findings, got %d", tt.count, len(got))
			}
			if tt.count > 0 && got[0].Type != FindingUnusedCollection {
				t.Errorf("expected type %s, got %s", FindingUnusedCollection, got[0].Type)
			}
		})
	}
}

func TestDetectUnusedIndexes(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:     "users",
		Database: "db",
		Indexes: []mongoinspect.IndexInfo{
			idx("_id_", kf("_id"), 100),
			idx("email_1", kf("email"), 0),
			idx("name_1", kf("name"), 50),
		},
	}
	findings := detectUnusedIndexes(&coll)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Index != "email_1" {
		t.Errorf("expected index email_1, got %s", findings[0].Index)
	}
}

func TestDetectUnusedIndexes_SkipsID(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:     "x",
		Database: "db",
		Indexes: []mongoinspect.IndexInfo{
			idx("_id_", kf("_id"), 0),
		},
	}
	if findings := detectUnusedIndexes(&coll); len(findings) != 0 {
		t.Errorf("_id_ should be skipped, got %d findings", len(findings))
	}
}

func TestDetectMissingIndexes(t *testing.T) {
	tests := []struct {
		name  string
		coll  mongoinspect.CollectionInfo
		count int
	}{
		{
			name: "high doc count, only _id",
			coll: mongoinspect.CollectionInfo{
				Name: "big", Database: "db", DocCount: 50_000,
				Indexes: []mongoinspect.IndexInfo{
					{Name: "_id_", Key: kf("_id")},
				},
			},
			count: 1,
		},
		{
			name: "high doc count, has secondary index",
			coll: mongoinspect.CollectionInfo{
				Name: "big", Database: "db", DocCount: 50_000,
				Indexes: []mongoinspect.IndexInfo{
					{Name: "_id_", Key: kf("_id")},
					{Name: "status_1", Key: kf("status")},
				},
			},
			count: 0,
		},
		{
			name: "low doc count, only _id",
			coll: mongoinspect.CollectionInfo{
				Name: "small", Database: "db", DocCount: 100,
				Indexes: []mongoinspect.IndexInfo{
					{Name: "_id_", Key: kf("_id")},
				},
			},
			count: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectMissingIndexes(&tt.coll)
			if len(got) != tt.count {
				t.Errorf("expected %d findings, got %d", tt.count, len(got))
			}
		})
	}
}

func TestDetectDuplicateIndexes(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:     "orders",
		Database: "db",
		Indexes: []mongoinspect.IndexInfo{
			{Name: "_id_", Key: kf("_id")},
			{Name: "status_1", Key: kf("status")},
			{Name: "status_1_date_1", Key: kf("status", "date")},
		},
	}
	findings := detectDuplicateIndexes(&coll)
	if len(findings) != 1 {
		t.Fatalf("expected 1 duplicate finding, got %d", len(findings))
	}
	if findings[0].Type != FindingDuplicateIndex {
		t.Errorf("expected type %s, got %s", FindingDuplicateIndex, findings[0].Type)
	}
}

func TestDetectDuplicateIndexes_NoDuplicates(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:     "products",
		Database: "db",
		Indexes: []mongoinspect.IndexInfo{
			{Name: "_id_", Key: kf("_id")},
			{Name: "sku_1", Key: kf("sku")},
			{Name: "category_1", Key: kf("category")},
		},
	}
	if findings := detectDuplicateIndexes(&coll); len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestDetectOversizedCollection(t *testing.T) {
	small := mongoinspect.CollectionInfo{Name: "small", Database: "db", StorageSize: 1024}
	if findings := detectOversizedCollection(&small); len(findings) != 0 {
		t.Errorf("expected 0, got %d", len(findings))
	}

	big := mongoinspect.CollectionInfo{Name: "big", Database: "db", StorageSize: 15 * 1024 * 1024 * 1024}
	findings := detectOversizedCollection(&big)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingOversizedCollection {
		t.Errorf("expected type %s, got %s", FindingOversizedCollection, findings[0].Type)
	}
}

func TestDetectMissingTTL(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:     "sessions",
		Database: "db",
		Indexes: []mongoinspect.IndexInfo{
			{Name: "createdAt_1", Key: kf("createdAt")},
		},
	}
	findings := detectMissingTTL(&coll)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingMissingTTL {
		t.Errorf("expected type %s, got %s", FindingMissingTTL, findings[0].Type)
	}
}

func TestDetectMissingTTL_HasTTL(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:     "sessions",
		Database: "db",
		Indexes: []mongoinspect.IndexInfo{
			idxTTL("createdAt_1", kf("createdAt"), 3600),
		},
	}
	if findings := detectMissingTTL(&coll); len(findings) != 0 {
		t.Errorf("expected 0 (TTL exists), got %d", len(findings))
	}
}

func TestAudit_Integration(t *testing.T) {
	collections := []mongoinspect.CollectionInfo{
		{
			Name: "users", Database: "app", DocCount: 50_000,
			Indexes: []mongoinspect.IndexInfo{
				idx("_id_", kf("_id"), 1000),
				idx("email_1", kf("email"), 500),
			},
		},
		{
			Name: "empty_coll", Database: "app", DocCount: 0,
			Indexes: []mongoinspect.IndexInfo{
				idx("_id_", kf("_id"), 0),
			},
		},
	}
	findings := Audit(collections)
	if len(findings) == 0 {
		t.Fatal("expected at least 1 finding")
	}

	hasUnused := false
	for _, f := range findings {
		if f.Type == FindingUnusedCollection && f.Collection == "empty_coll" {
			hasUnused = true
		}
	}
	if !hasUnused {
		t.Error("expected UNUSED_COLLECTION for empty_coll")
	}
}

func TestDetectIndexBloat(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:           "bloated",
		Database:       "db",
		Size:           100 * 1024 * 1024, // 100 MB data
		TotalIndexSize: 500 * 1024 * 1024, // 500 MB indexes
	}
	findings := detectIndexBloat(&coll)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingIndexBloat {
		t.Errorf("expected type %s, got %s", FindingIndexBloat, findings[0].Type)
	}
	if findings[0].Severity != SeverityMedium {
		t.Errorf("severity = %s, want medium", findings[0].Severity)
	}
}

func TestDetectIndexBloat_NoData(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:           "empty",
		Database:       "db",
		Size:           0,
		TotalIndexSize: 4096,
	}
	if findings := detectIndexBloat(&coll); len(findings) != 0 {
		t.Errorf("expected 0 findings for empty collection, got %d", len(findings))
	}
}

func TestDetectWriteHeavyOverIndexed(t *testing.T) {
	indexes := make([]mongoinspect.IndexInfo, 11)
	for i := range indexes {
		indexes[i] = mongoinspect.IndexInfo{
			Name: fmt.Sprintf("idx_%d", i),
			Key:  kf(fmt.Sprintf("field%d", i)),
		}
	}
	coll := mongoinspect.CollectionInfo{
		Name:     "over_indexed",
		Database: "db",
		Indexes:  indexes,
	}
	findings := detectWriteHeavyOverIndexed(&coll)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingWriteHeavyOverIndexed {
		t.Errorf("expected type %s, got %s", FindingWriteHeavyOverIndexed, findings[0].Type)
	}
}

func TestDetectWriteHeavyOverIndexed_Under(t *testing.T) {
	indexes := make([]mongoinspect.IndexInfo, 10)
	for i := range indexes {
		indexes[i] = mongoinspect.IndexInfo{
			Name: fmt.Sprintf("idx_%d", i),
			Key:  kf(fmt.Sprintf("field%d", i)),
		}
	}
	coll := mongoinspect.CollectionInfo{
		Name:     "normal",
		Database: "db",
		Indexes:  indexes,
	}
	if findings := detectWriteHeavyOverIndexed(&coll); len(findings) != 0 {
		t.Errorf("expected 0 findings for 10 indexes, got %d", len(findings))
	}
}

func TestDetectSingleFieldRedundant(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:     "orders",
		Database: "db",
		Indexes: []mongoinspect.IndexInfo{
			{Name: "_id_", Key: kf("_id")},
			{Name: "status_1", Key: kf("status")},
			{Name: "status_1_date_1", Key: kf("status", "date")},
		},
	}
	findings := detectSingleFieldRedundant(&coll)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingSingleFieldRedundant {
		t.Errorf("expected type %s, got %s", FindingSingleFieldRedundant, findings[0].Type)
	}
	if findings[0].Index != "status_1" {
		t.Errorf("expected index status_1, got %s", findings[0].Index)
	}
}

func TestDetectSingleFieldRedundant_NotCovered(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:     "products",
		Database: "db",
		Indexes: []mongoinspect.IndexInfo{
			{Name: "_id_", Key: kf("_id")},
			{Name: "sku_1", Key: kf("sku")},
			{Name: "category_1_name_1", Key: kf("category", "name")},
		},
	}
	if findings := detectSingleFieldRedundant(&coll); len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestDetectLargeIndex(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:     "logs",
		Database: "db",
		Indexes: []mongoinspect.IndexInfo{
			{Name: "ts_1", Key: kf("ts"), Size: 2 * 1024 * 1024 * 1024}, // 2 GB
		},
	}
	findings := detectLargeIndex(&coll)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingLargeIndex {
		t.Errorf("expected type %s, got %s", FindingLargeIndex, findings[0].Type)
	}
	if findings[0].Index != "ts_1" {
		t.Errorf("expected index ts_1, got %s", findings[0].Index)
	}
}

func TestDetectLargeIndex_Under(t *testing.T) {
	coll := mongoinspect.CollectionInfo{
		Name:     "small",
		Database: "db",
		Indexes: []mongoinspect.IndexInfo{
			{Name: "idx_1", Key: kf("field"), Size: 500 * 1024 * 1024}, // 500 MB
		},
	}
	if findings := detectLargeIndex(&coll); len(findings) != 0 {
		t.Errorf("expected 0 findings for 500MB index, got %d", len(findings))
	}
}

func TestIsKeyPrefix(t *testing.T) {
	tests := []struct {
		name string
		a, b []mongoinspect.KeyField
		want bool
	}{
		{"exact match", kf("a"), kf("a"), true},
		{"prefix", kf("a"), kf("a", "b"), true},
		{"not prefix - different fields", kf("a", "c"), kf("a", "b"), false},
		{"longer a", kf("a", "b"), kf("a"), false},
		{"empty a", nil, kf("a"), false},
		{"different direction", kf("-a"), kf("a", "b"), false},
		{"order matters", kf("b"), kf("a", "b"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isKeyPrefix(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("isKeyPrefix(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
