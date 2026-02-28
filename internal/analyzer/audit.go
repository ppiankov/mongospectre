package analyzer

import (
	"fmt"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

const (
	// Collections with more docs than this but only _id index get flagged.
	missingIndexThreshold int64 = 10_000

	// Collections larger than this (bytes) get flagged as oversized.
	oversizedThreshold int64 = 10 * 1024 * 1024 * 1024 // 10 GB

	// Common timestamp field names that suggest a TTL index might be needed.
	timestampFieldHint = "created,updated,timestamp,expires,expiry,ttl,lastModified,createdAt,updatedAt,expiresAt"

	// Collections with more indexes than this get flagged for write amplification.
	maxIndexesPerCollection = 10

	// Individual indexes larger than this (bytes) get flagged.
	largeIndexThreshold int64 = 1 << 30 // 1 GB
)

// Audit runs all cluster-only detections against the given collections.
func Audit(collections []mongoinspect.CollectionInfo) []Finding {
	var findings []Finding
	for _, c := range collections {
		findings = append(findings, detectUnusedCollection(&c)...)
		findings = append(findings, detectUnusedIndexes(&c)...)
		findings = append(findings, detectMissingIndexes(&c)...)
		findings = append(findings, detectDuplicateIndexes(&c)...)
		findings = append(findings, detectOversizedCollection(&c)...)
		findings = append(findings, detectMissingTTL(&c)...)
		findings = append(findings, detectIndexBloat(&c)...)
		findings = append(findings, detectWriteHeavyOverIndexed(&c)...)
		findings = append(findings, detectSingleFieldRedundant(&c)...)
		findings = append(findings, detectLargeIndex(&c)...)
	}
	return findings
}

// detectUnusedCollection flags collections with zero documents.
func detectUnusedCollection(c *mongoinspect.CollectionInfo) []Finding {
	if c.Type == "view" || c.DocCount > 0 {
		return nil
	}
	return []Finding{{
		Type:       FindingUnusedCollection,
		Severity:   SeverityMedium,
		Database:   c.Database,
		Collection: c.Name,
		Message:    "collection has 0 documents",
	}}
}

// detectUnusedIndexes flags indexes with zero operations (excluding _id).
func detectUnusedIndexes(c *mongoinspect.CollectionInfo) []Finding {
	var findings []Finding
	for _, idx := range c.Indexes {
		if idx.Name == "_id_" {
			continue
		}
		if idx.Stats != nil && idx.Stats.Ops == 0 {
			findings = append(findings, Finding{
				Type:       FindingUnusedIndex,
				Severity:   SeverityMedium,
				Database:   c.Database,
				Collection: c.Name,
				Index:      idx.Name,
				Message:    fmt.Sprintf("index %q has never been used", idx.Name),
			})
		}
	}
	return findings
}

// detectMissingIndexes flags collections with high doc count but only the _id index.
func detectMissingIndexes(c *mongoinspect.CollectionInfo) []Finding {
	if c.DocCount < missingIndexThreshold {
		return nil
	}
	nonIDCount := 0
	for _, idx := range c.Indexes {
		if idx.Name != "_id_" {
			nonIDCount++
		}
	}
	if nonIDCount > 0 {
		return nil
	}
	return []Finding{{
		Type:       FindingMissingIndex,
		Severity:   SeverityHigh,
		Database:   c.Database,
		Collection: c.Name,
		Message:    fmt.Sprintf("collection has %d documents but only the _id index", c.DocCount),
	}}
}

// detectDuplicateIndexes flags indexes whose key pattern is a prefix of another.
func detectDuplicateIndexes(c *mongoinspect.CollectionInfo) []Finding {
	var findings []Finding
	for i, a := range c.Indexes {
		for j, b := range c.Indexes {
			if i >= j || a.Name == "_id_" || b.Name == "_id_" {
				continue
			}
			if isKeyPrefix(a.Key, b.Key) {
				findings = append(findings, Finding{
					Type:       FindingDuplicateIndex,
					Severity:   SeverityLow,
					Database:   c.Database,
					Collection: c.Name,
					Index:      a.Name,
					Message:    fmt.Sprintf("index %q is a prefix of %q", a.Name, b.Name),
				})
			}
		}
	}
	return findings
}

// detectOversizedCollection flags collections exceeding the size threshold.
func detectOversizedCollection(c *mongoinspect.CollectionInfo) []Finding {
	if c.StorageSize < oversizedThreshold {
		return nil
	}
	gb := float64(c.StorageSize) / (1024 * 1024 * 1024)
	return []Finding{{
		Type:       FindingOversizedCollection,
		Severity:   SeverityLow,
		Database:   c.Database,
		Collection: c.Name,
		Message:    fmt.Sprintf("collection storage is %.1f GB", gb),
	}}
}

// detectMissingTTL flags indexes on common timestamp fields that lack a TTL.
func detectMissingTTL(c *mongoinspect.CollectionInfo) []Finding {
	hints := strings.Split(timestampFieldHint, ",")
	hintSet := make(map[string]bool, len(hints))
	for _, h := range hints {
		hintSet[strings.ToLower(h)] = true
	}

	// Collect fields that already have a TTL index.
	ttlFields := make(map[string]bool)
	for _, idx := range c.Indexes {
		if idx.TTL != nil {
			for _, kf := range idx.Key {
				ttlFields[strings.ToLower(kf.Field)] = true
			}
		}
	}

	// Check indexed timestamp-like fields missing TTL.
	var findings []Finding
	seen := make(map[string]bool)
	for _, idx := range c.Indexes {
		for _, kf := range idx.Key {
			lower := strings.ToLower(kf.Field)
			if hintSet[lower] && !ttlFields[lower] && !seen[lower] {
				seen[lower] = true
				findings = append(findings, Finding{
					Type:       FindingMissingTTL,
					Severity:   SeverityLow,
					Database:   c.Database,
					Collection: c.Name,
					Index:      idx.Name,
					Message:    fmt.Sprintf("field %q looks like a timestamp but has no TTL index", kf.Field),
				})
			}
		}
	}
	return findings
}

// detectIndexBloat flags collections where total index size exceeds data size.
func detectIndexBloat(c *mongoinspect.CollectionInfo) []Finding {
	if c.Size == 0 || c.TotalIndexSize <= c.Size {
		return nil
	}
	dataMB := float64(c.Size) / (1024 * 1024)
	idxMB := float64(c.TotalIndexSize) / (1024 * 1024)
	ratio := float64(c.TotalIndexSize) / float64(c.Size)
	return []Finding{{
		Type:       FindingIndexBloat,
		Severity:   SeverityMedium,
		Database:   c.Database,
		Collection: c.Name,
		Message:    fmt.Sprintf("total index size (%.1f MB) exceeds data size (%.1f MB) — ratio %.1f:1", idxMB, dataMB, ratio),
	}}
}

// detectWriteHeavyOverIndexed flags collections with too many indexes.
func detectWriteHeavyOverIndexed(c *mongoinspect.CollectionInfo) []Finding {
	if len(c.Indexes) <= maxIndexesPerCollection {
		return nil
	}
	return []Finding{{
		Type:       FindingWriteHeavyOverIndexed,
		Severity:   SeverityMedium,
		Database:   c.Database,
		Collection: c.Name,
		Message:    fmt.Sprintf("collection has %d indexes — every write updates all of them", len(c.Indexes)),
	}}
}

// detectSingleFieldRedundant flags single-field indexes covered by a compound index.
func detectSingleFieldRedundant(c *mongoinspect.CollectionInfo) []Finding {
	var findings []Finding
	for _, single := range c.Indexes {
		if single.Name == "_id_" || len(single.Key) != 1 {
			continue
		}
		for _, compound := range c.Indexes {
			if len(compound.Key) <= 1 {
				continue
			}
			if isKeyPrefix(single.Key, compound.Key) {
				findings = append(findings, Finding{
					Type:       FindingSingleFieldRedundant,
					Severity:   SeverityLow,
					Database:   c.Database,
					Collection: c.Name,
					Index:      single.Name,
					Message:    fmt.Sprintf("single-field index %q is covered by compound index %q", single.Name, compound.Name),
				})
				break
			}
		}
	}
	return findings
}

// detectLargeIndex flags individual indexes exceeding the size threshold.
func detectLargeIndex(c *mongoinspect.CollectionInfo) []Finding {
	var findings []Finding
	for _, idx := range c.Indexes {
		if idx.Size < largeIndexThreshold {
			continue
		}
		gb := float64(idx.Size) / (1024 * 1024 * 1024)
		findings = append(findings, Finding{
			Type:       FindingLargeIndex,
			Severity:   SeverityLow,
			Database:   c.Database,
			Collection: c.Name,
			Index:      idx.Name,
			Message:    fmt.Sprintf("index %q is %.1f GB — review whether this index is necessary", idx.Name, gb),
		})
	}
	return findings
}

// isKeyPrefix returns true if a's key fields are an ordered prefix of b's.
func isKeyPrefix(a, b []mongoinspect.KeyField) bool {
	if len(a) == 0 || len(a) > len(b) {
		return false
	}
	for i, kf := range a {
		if kf.Field != b[i].Field || kf.Direction != b[i].Direction {
			return false
		}
	}
	return true
}
