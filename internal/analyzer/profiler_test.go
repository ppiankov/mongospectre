package analyzer

import (
	"strings"
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

func TestCorrelateProfiler_SlowAndCOLLSCANLinkedToSource(t *testing.T) {
	scan := &scanner.ScanResult{
		Refs: []scanner.CollectionRef{
			{Collection: "users", File: "app/models/user.go", Line: 15},
		},
		FieldRefs: []scanner.FieldRef{
			{Collection: "users", Field: "status", File: "app/models/user.go", Line: 15},
		},
	}
	entries := []mongoinspect.ProfileEntry{
		{
			Database:       "app",
			Collection:     "users",
			FilterFields:   []string{"status"},
			DurationMillis: 850,
			PlanSummary:    "COLLSCAN",
		},
	}

	findings := CorrelateProfiler(scan, entries)

	var slow, collscan *Finding
	for i := range findings {
		switch findings[i].Type {
		case FindingSlowQuerySource:
			slow = &findings[i]
		case FindingCollectionScanSource:
			collscan = &findings[i]
		}
	}

	if slow == nil {
		t.Fatal("expected SLOW_QUERY_SOURCE finding")
	}
	if slow.Severity != SeverityMedium {
		t.Fatalf("slow severity = %s, want medium", slow.Severity)
	}
	if !strings.Contains(slow.Message, "app/models/user.go:15") {
		t.Fatalf("slow message missing source location: %q", slow.Message)
	}

	if collscan == nil {
		t.Fatal("expected COLLECTION_SCAN_SOURCE finding")
	}
	if collscan.Severity != SeverityHigh {
		t.Fatalf("collscan severity = %s, want high", collscan.Severity)
	}
	if !strings.Contains(collscan.Message, "app/models/user.go:15") {
		t.Fatalf("collscan message missing source location: %q", collscan.Message)
	}
}

func TestCorrelateProfiler_FrequentSlowQuery(t *testing.T) {
	scan := &scanner.ScanResult{
		Refs: []scanner.CollectionRef{
			{Collection: "orders", File: "app/handlers/orders.go", Line: 42},
		},
		FieldRefs: []scanner.FieldRef{
			{Collection: "orders", Field: "status", File: "app/handlers/orders.go", Line: 42},
		},
	}

	entries := make([]mongoinspect.ProfileEntry, 0, frequentSlowQueryThreshold)
	for i := 0; i < frequentSlowQueryThreshold; i++ {
		entries = append(entries, mongoinspect.ProfileEntry{
			Database:       "app",
			Collection:     "orders",
			FilterFields:   []string{"status"},
			DurationMillis: 300,
			PlanSummary:    "IXSCAN {status: 1}",
		})
	}

	findings := CorrelateProfiler(scan, entries)

	found := false
	for _, finding := range findings {
		if finding.Type != FindingFrequentSlowQuery {
			continue
		}
		found = true
		if finding.Severity != SeverityMedium {
			t.Fatalf("frequent severity = %s, want medium", finding.Severity)
		}
		if !strings.Contains(finding.Message, "appears 50 times") {
			t.Fatalf("frequent message missing count: %q", finding.Message)
		}
		if !strings.Contains(finding.Message, "app/handlers/orders.go:42") {
			t.Fatalf("frequent message missing source: %q", finding.Message)
		}
	}
	if !found {
		t.Fatal("expected FREQUENT_SLOW_QUERY finding")
	}
}

func TestCorrelateProfiler_NoMatchingCollection(t *testing.T) {
	scan := &scanner.ScanResult{
		Refs: []scanner.CollectionRef{
			{Collection: "users", File: "app/main.go", Line: 10},
		},
	}
	entries := []mongoinspect.ProfileEntry{
		{Database: "app", Collection: "orders", DurationMillis: 120},
	}

	findings := CorrelateProfiler(scan, entries)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %d", len(findings))
	}
}

func TestCorrelateProfiler_FallbackToCollectionReference(t *testing.T) {
	scan := &scanner.ScanResult{
		Refs: []scanner.CollectionRef{
			{Collection: "users", File: "app/main.go", Line: 10},
		},
	}
	entries := []mongoinspect.ProfileEntry{
		{
			Database:       "app",
			Collection:     "users",
			FilterFields:   []string{"email"},
			DurationMillis: 200,
		},
	}

	findings := CorrelateProfiler(scan, entries)

	var slow *Finding
	for i := range findings {
		if findings[i].Type == FindingSlowQuerySource {
			slow = &findings[i]
			break
		}
	}
	if slow == nil {
		t.Fatal("expected SLOW_QUERY_SOURCE finding")
	}
	if !strings.Contains(slow.Message, "app/main.go:10") {
		t.Fatalf("slow message missing fallback collection location: %q", slow.Message)
	}
}
