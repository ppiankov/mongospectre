package analyzer

import (
	"strings"
	"testing"

	"github.com/ppiankov/mongospectre/internal/atlas"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

func TestAuditAtlas_IndexSuggestionsCorrelatedWithCode(t *testing.T) {
	scan := &scanner.ScanResult{
		Collections: []string{"orders"},
		FieldRefs: []scanner.FieldRef{
			{Collection: "orders", Field: "status", File: "repo/orders.go", Line: 12},
		},
	}

	findings := AuditAtlas(&AtlasAuditInput{
		SuggestedIndexes: []atlas.SuggestedIndex{{
			Namespace:   "app.orders",
			IndexFields: []string{"status", "createdAt"},
		}},
		Scan: scan,
	})

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Type != FindingAtlasIndexSuggestion {
		t.Fatalf("type = %s, want %s", f.Type, FindingAtlasIndexSuggestion)
	}
	if f.Severity != SeverityLow {
		t.Fatalf("severity = %s, want %s", f.Severity, SeverityLow)
	}
	if !strings.Contains(f.Message, "matching queried code fields: status") {
		t.Fatalf("message missing code correlation: %q", f.Message)
	}
}

func TestAuditAtlas_AlertsOnlyActive(t *testing.T) {
	findings := AuditAtlas(&AtlasAuditInput{
		ProjectID: "proj-1",
		Cluster:   atlas.Cluster{Name: "Cluster0"},
		Alerts: []atlas.Alert{
			{EventTypeName: "OUTSIDE_METRIC_THRESHOLD", Status: "OPEN"},
			{EventTypeName: "CLUSTER_DELETE", Status: "CLOSED"},
		},
	})

	if len(findings) != 1 {
		t.Fatalf("expected 1 active alert finding, got %d", len(findings))
	}
	if findings[0].Type != FindingAtlasAlertActive {
		t.Fatalf("type = %s, want %s", findings[0].Type, FindingAtlasAlertActive)
	}
}

func TestAuditAtlas_TierMismatch(t *testing.T) {
	const gb = int64(1024 * 1024 * 1024)

	findings := AuditAtlas(&AtlasAuditInput{
		Cluster: atlas.Cluster{
			Name:             "Cluster0",
			InstanceSizeName: "M10",
		},
		Collections: []mongoinspect.CollectionInfo{
			{Database: "app", Name: "events", StorageSize: 300 * gb},
			{Database: "app", Name: "logs", StorageSize: 250 * gb},
		},
	})

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingAtlasTierMismatch {
		t.Fatalf("type = %s, want %s", findings[0].Type, FindingAtlasTierMismatch)
	}
}

func TestAuditAtlas_VersionBehind(t *testing.T) {
	findings := AuditAtlas(&AtlasAuditInput{
		Cluster: atlas.Cluster{
			Name:           "Cluster0",
			MongoDBVersion: "7.0.5",
		},
		AvailableVersions: []string{"6.0", "7.0", "8.0"},
	})

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingAtlasVersionBehind {
		t.Fatalf("type = %s, want %s", findings[0].Type, FindingAtlasVersionBehind)
	}
}
