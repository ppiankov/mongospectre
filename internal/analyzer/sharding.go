package analyzer

import (
	"fmt"
	"sort"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

// AuditSharding runs sharding-specific detections against inspected metadata.
func AuditSharding(collections []mongoinspect.CollectionInfo, sharding mongoinspect.ShardingInfo) []Finding {
	if !sharding.Enabled {
		return nil
	}

	var findings []Finding
	shardedCollections := make(map[string]mongoinspect.ShardedCollectionInfo, len(sharding.Collections))
	for _, coll := range sharding.Collections {
		key := coll.Database + "." + coll.Collection
		shardedCollections[key] = coll

		findings = append(findings, detectMonotonicShardKey(&coll)...)
		findings = append(findings, detectUnbalancedChunks(&coll, sharding.Shards)...)
		findings = append(findings, detectJumboChunks(&coll)...)
	}

	for _, coll := range collections {
		if coll.Type == "view" {
			continue
		}
		if coll.StorageSize < oversizedThreshold {
			continue
		}
		key := coll.Database + "." + coll.Name
		if _, ok := shardedCollections[key]; ok {
			continue
		}

		gb := float64(coll.StorageSize) / (1024 * 1024 * 1024)
		findings = append(findings, Finding{
			Type:       FindingUnshardedLarge,
			Severity:   SeverityMedium,
			Database:   coll.Database,
			Collection: coll.Name,
			Message:    fmt.Sprintf("collection storage is %.1f GB and collection is not sharded", gb),
		})
	}

	if !sharding.BalancerEnabled {
		findings = append(findings, Finding{
			Type:       FindingBalancerDisabled,
			Severity:   SeverityMedium,
			Database:   "config",
			Collection: "settings",
			Message:    "chunk balancer is disabled",
		})
	}

	return findings
}

func detectMonotonicShardKey(coll *mongoinspect.ShardedCollectionInfo) []Finding {
	if len(coll.Key) != 1 {
		return nil
	}

	field := strings.ToLower(coll.Key[0].Field)
	if field != "_id" && field != "created_at" && field != "createdat" {
		return nil
	}

	return []Finding{{
		Type:       FindingMonotonicShardKey,
		Severity:   SeverityMedium,
		Database:   coll.Database,
		Collection: coll.Collection,
		Message:    fmt.Sprintf("shard key %q can be monotonic and create a hot shard", formatShardKey(coll.Key)),
	}}
}

func detectUnbalancedChunks(coll *mongoinspect.ShardedCollectionInfo, shardNames []string) []Finding {
	loads := shardLoads(coll, shardNames)
	if len(loads) < 2 {
		return nil
	}

	minLoad := loads[0]
	maxLoad := loads[0]
	for _, load := range loads[1:] {
		if load.chunks < minLoad.chunks {
			minLoad = load
		}
		if load.chunks > maxLoad.chunks {
			maxLoad = load
		}
	}

	if maxLoad.chunks == 0 {
		return nil
	}
	if minLoad.chunks > 0 && maxLoad.chunks <= 2*minLoad.chunks {
		return nil
	}

	return []Finding{{
		Type:       FindingUnbalancedChunks,
		Severity:   SeverityHigh,
		Database:   coll.Database,
		Collection: coll.Collection,
		Message: fmt.Sprintf("chunk distribution is unbalanced: %s has %d chunks, %s has %d%s",
			maxLoad.name, maxLoad.chunks, minLoad.name, minLoad.chunks, sampledSuffix(coll.ChunkLimitHit)),
	}}
}

func detectJumboChunks(coll *mongoinspect.ShardedCollectionInfo) []Finding {
	if coll.JumboChunks == 0 {
		return nil
	}
	return []Finding{{
		Type:       FindingJumboChunks,
		Severity:   SeverityHigh,
		Database:   coll.Database,
		Collection: coll.Collection,
		Message:    fmt.Sprintf("%d jumbo chunk(s) detected%s", coll.JumboChunks, sampledSuffix(coll.ChunkLimitHit)),
	}}
}

type shardLoad struct {
	name   string
	chunks int64
}

func shardLoads(coll *mongoinspect.ShardedCollectionInfo, shardNames []string) []shardLoad {
	seen := make(map[string]bool)
	loads := make([]shardLoad, 0, len(coll.ChunkDistribution))

	for _, shard := range shardNames {
		loads = append(loads, shardLoad{name: shard, chunks: coll.ChunkDistribution[shard]})
		seen[shard] = true
	}
	for shard, chunks := range coll.ChunkDistribution {
		if seen[shard] {
			continue
		}
		loads = append(loads, shardLoad{name: shard, chunks: chunks})
	}

	sort.Slice(loads, func(i, j int) bool { return loads[i].name < loads[j].name })
	return loads
}

func formatShardKey(key []mongoinspect.KeyField) string {
	parts := make([]string, 0, len(key))
	for _, kf := range key {
		parts = append(parts, fmt.Sprintf("%s:%d", kf.Field, kf.Direction))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func sampledSuffix(chunkLimitHit bool) string {
	if !chunkLimitHit {
		return ""
	}
	return " (first 10000 chunks sampled)"
}
