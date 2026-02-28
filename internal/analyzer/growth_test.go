package analyzer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

func TestDetectRapidGrowth(t *testing.T) {
	current := &mongoinspect.CollectionInfo{
		Database: "app", Name: "orders", Size: 1_500_000_000,
	}
	baseline := &mongoinspect.CollectionInfo{
		Database: "app", Name: "orders", Size: 900_000_000,
	}
	findings := detectRapidGrowth(current, baseline, "7 days")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingRapidGrowth {
		t.Fatalf("expected RAPID_GROWTH, got %s", findings[0].Type)
	}
}

func TestDetectRapidGrowth_AbsoluteThreshold(t *testing.T) {
	// 10% growth but >1GB absolute.
	current := &mongoinspect.CollectionInfo{
		Database: "app", Name: "logs", Size: 12_000_000_000,
	}
	baseline := &mongoinspect.CollectionInfo{
		Database: "app", Name: "logs", Size: 10_800_000_000,
	}
	findings := detectRapidGrowth(current, baseline, "7 days")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for >1GB absolute growth, got %d", len(findings))
	}
}

func TestDetectRapidGrowth_NoGrowth(t *testing.T) {
	current := &mongoinspect.CollectionInfo{
		Database: "app", Name: "users", Size: 500_000,
	}
	baseline := &mongoinspect.CollectionInfo{
		Database: "app", Name: "users", Size: 500_000,
	}
	findings := detectRapidGrowth(current, baseline, "7 days")
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestDetectRapidGrowth_NewCollection(t *testing.T) {
	current := &mongoinspect.CollectionInfo{
		Database: "app", Name: "new_coll", Size: 1_000_000,
	}
	findings := detectRapidGrowth(current, nil, "7 days")
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for nil baseline, got %d", len(findings))
	}
}

func TestDetectIndexGrowthOutpacingData(t *testing.T) {
	current := &mongoinspect.CollectionInfo{
		Database: "app", Name: "events",
		Size: 2_000_000_000, TotalIndexSize: 1_500_000_000,
	}
	baseline := &mongoinspect.CollectionInfo{
		Database: "app", Name: "events",
		Size: 1_500_000_000, TotalIndexSize: 800_000_000,
	}
	// Data grew 33%, index grew 87%.
	findings := detectIndexGrowthOutpacingData(current, baseline, "7 days")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingIndexGrowthOutpacing {
		t.Fatalf("expected INDEX_GROWTH_OUTPACING_DATA, got %s", findings[0].Type)
	}
}

func TestDetectIndexGrowthOutpacingData_Normal(t *testing.T) {
	current := &mongoinspect.CollectionInfo{
		Database: "app", Name: "events",
		Size: 2_000_000_000, TotalIndexSize: 900_000_000,
	}
	baseline := &mongoinspect.CollectionInfo{
		Database: "app", Name: "events",
		Size: 1_000_000_000, TotalIndexSize: 800_000_000,
	}
	// Data grew 100%, index grew 12.5%.
	findings := detectIndexGrowthOutpacingData(current, baseline, "7 days")
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestDetectApproachingLimit(t *testing.T) {
	current := &mongoinspect.CollectionInfo{
		Database: "app", Name: "logs", Size: 13 * (1 << 30), // 13 GB
	}
	findings := detectApproachingLimit(current)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingApproachingLimit {
		t.Fatalf("expected APPROACHING_LIMIT, got %s", findings[0].Type)
	}
}

func TestDetectApproachingLimit_Small(t *testing.T) {
	current := &mongoinspect.CollectionInfo{
		Database: "app", Name: "users", Size: 5 * (1 << 30), // 5 GB
	}
	findings := detectApproachingLimit(current)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestDetectStorageReclaim(t *testing.T) {
	current := &mongoinspect.CollectionInfo{
		Database: "app", Name: "events",
		Size: 1_000_000_000, StorageSize: 3_000_000_000,
	}
	findings := detectStorageReclaim(current)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingStorageReclaim {
		t.Fatalf("expected STORAGE_RECLAIM, got %s", findings[0].Type)
	}
}

func TestDetectStorageReclaim_Normal(t *testing.T) {
	current := &mongoinspect.CollectionInfo{
		Database: "app", Name: "events",
		Size: 1_000_000_000, StorageSize: 1_500_000_000,
	}
	findings := detectStorageReclaim(current)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestDetectGrowth_EmptyBaseline(t *testing.T) {
	current := []mongoinspect.CollectionInfo{
		{Database: "app", Name: "users", Size: 1_000_000},
	}
	findings := DetectGrowth(current, nil, 24*time.Hour)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for nil baseline, got %d", len(findings))
	}
}

func TestLoadBaselineWithCollections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	content := `{
		"metadata": {"timestamp": "2026-02-27T10:00:00Z"},
		"findings": [{"type":"UNUSED_INDEX","severity":"medium","database":"app","collection":"users","index":"idx_old","message":"test"}],
		"collections": [{"name":"users","database":"app","docCount":1000,"size":500000,"storageSize":600000,"totalIndexSize":100000}]
	}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	findings, collections, ts, err := LoadBaselineWithCollections(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if len(collections) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(collections))
	}
	if collections[0].Name != "users" {
		t.Errorf("collection name = %s, want users", collections[0].Name)
	}
	if collections[0].Size != 500000 {
		t.Errorf("collection size = %d, want 500000", collections[0].Size)
	}
	if ts.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if ts.Year() != 2026 || ts.Month() != 2 || ts.Day() != 27 {
		t.Errorf("timestamp = %v, want 2026-02-27", ts)
	}
}

func TestLoadBaselineWithCollections_NoCollections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old_baseline.json")
	content := `{"findings":[{"type":"UNUSED_INDEX","severity":"medium","database":"app","collection":"users","index":"idx_old","message":"test"}]}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	findings, collections, ts, err := LoadBaselineWithCollections(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if len(collections) != 0 {
		t.Fatalf("expected 0 collections, got %d", len(collections))
	}
	if !ts.IsZero() {
		t.Errorf("expected zero timestamp, got %v", ts)
	}
}

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{7 * 24 * time.Hour, "7 days"},
		{36 * time.Hour, "1 days"},
		{12 * time.Hour, "12 hours"},
		{30 * time.Minute, "30 minutes"},
	}
	for _, tt := range tests {
		got := formatElapsed(tt.d)
		if got != tt.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
