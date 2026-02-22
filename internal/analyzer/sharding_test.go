package analyzer

import (
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

func TestAuditSharding_Detections(t *testing.T) {
	collections := []mongoinspect.CollectionInfo{
		{
			Database:    "app",
			Name:        "events",
			Type:        "collection",
			StorageSize: 12 * 1024 * 1024 * 1024,
		},
		{
			Database:    "app",
			Name:        "logs",
			Type:        "collection",
			StorageSize: 15 * 1024 * 1024 * 1024,
		},
	}
	sharding := mongoinspect.ShardingInfo{
		Enabled:         true,
		BalancerEnabled: false,
		Shards:          []string{"shardA", "shardB"},
		Collections: []mongoinspect.ShardedCollectionInfo{
			{
				Namespace:  "app.events",
				Database:   "app",
				Collection: "events",
				Key:        []mongoinspect.KeyField{{Field: "_id", Direction: 1}},
				ChunkDistribution: map[string]int64{
					"shardA": 9,
					"shardB": 1,
				},
				ChunkCount:  10,
				JumboChunks: 2,
			},
		},
	}

	findings := AuditSharding(collections, sharding)
	assertHasFindingType(t, findings, FindingMonotonicShardKey)
	assertHasFindingType(t, findings, FindingUnbalancedChunks)
	assertHasFindingType(t, findings, FindingJumboChunks)
	assertHasFindingType(t, findings, FindingUnshardedLarge)
	assertHasFindingType(t, findings, FindingBalancerDisabled)
}

func TestAuditSharding_ExactTwoXNotUnbalanced(t *testing.T) {
	sharding := mongoinspect.ShardingInfo{
		Enabled: true,
		Shards:  []string{"shardA", "shardB"},
		Collections: []mongoinspect.ShardedCollectionInfo{
			{
				Namespace:  "app.orders",
				Database:   "app",
				Collection: "orders",
				Key:        []mongoinspect.KeyField{{Field: "customer_id", Direction: 1}},
				ChunkDistribution: map[string]int64{
					"shardA": 4,
					"shardB": 2,
				},
				ChunkCount: 6,
			},
		},
	}

	findings := AuditSharding(nil, sharding)
	if hasFindingType(findings, FindingUnbalancedChunks) {
		t.Fatalf("did not expect %s when ratio is exactly 2x", FindingUnbalancedChunks)
	}
}

func TestAuditSharding_Disabled(t *testing.T) {
	findings := AuditSharding(nil, mongoinspect.ShardingInfo{})
	if len(findings) != 0 {
		t.Fatalf("expected no findings for non-sharded deployment, got %d", len(findings))
	}
}

func assertHasFindingType(t *testing.T, findings []Finding, want FindingType) {
	t.Helper()
	if !hasFindingType(findings, want) {
		t.Fatalf("expected finding type %s, got %+v", want, findings)
	}
}

func hasFindingType(findings []Finding, want FindingType) bool {
	for _, f := range findings {
		if f.Type == want {
			return true
		}
	}
	return false
}
