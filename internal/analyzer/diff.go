package analyzer

import (
	"fmt"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

// Diff compares code repo references against live MongoDB collections.
func Diff(scan *scanner.ScanResult, collections []mongoinspect.CollectionInfo) []Finding {
	// Build lookup: db.collection -> CollectionInfo
	dbColls := make(map[string]mongoinspect.CollectionInfo)
	for _, c := range collections {
		key := c.Database + "." + c.Name
		dbColls[key] = c
		// Also index by collection name only for cross-db matching.
		dbColls[c.Name] = c
	}

	// Build set of collection names referenced in code (lowercased for comparison).
	codeRefs := make(map[string]bool)
	for _, name := range scan.Collections {
		codeRefs[strings.ToLower(name)] = true
	}

	var findings []Finding

	// 1. MISSING_COLLECTION: in code, not in DB
	for _, name := range scan.Collections {
		if _, found := findCollection(name, collections); !found {
			findings = append(findings, Finding{
				Type:       FindingMissingCollection,
				Severity:   SeverityHigh,
				Collection: name,
				Message:    fmt.Sprintf("collection %q referenced in code but does not exist in database", name),
			})
		}
	}

	// 2. UNUSED_COLLECTION: in DB, not in code, zero docs
	for _, c := range collections {
		if c.Type == "view" {
			continue
		}
		if !codeRefs[strings.ToLower(c.Name)] && c.DocCount == 0 {
			findings = append(findings, Finding{
				Type:       FindingUnusedCollection,
				Severity:   SeverityMedium,
				Database:   c.Database,
				Collection: c.Name,
				Message:    fmt.Sprintf("collection %q exists in database with 0 documents and is not referenced in code", c.Name),
			})
		}
	}

	// 3. ORPHANED_INDEX: index exists on a collection not referenced in code
	for _, c := range collections {
		if c.Type == "view" {
			continue
		}
		if codeRefs[strings.ToLower(c.Name)] {
			continue
		}
		for _, idx := range c.Indexes {
			if idx.Name == "_id_" {
				continue
			}
			if idx.Stats != nil && idx.Stats.Ops == 0 {
				findings = append(findings, Finding{
					Type:       FindingOrphanedIndex,
					Severity:   SeverityLow,
					Database:   c.Database,
					Collection: c.Name,
					Index:      idx.Name,
					Message:    fmt.Sprintf("index %q on unreferenced collection %q has 0 operations", idx.Name, c.Name),
				})
			}
		}
	}

	// 4. UNINDEXED_QUERY: code queries a field that has no covering index
	findings = append(findings, detectUnindexedQueries(scan, collections)...)

	// 5. SUGGEST_INDEX: recommend indexes based on queried fields
	findings = append(findings, suggestIndexes(scan, collections)...)

	// 6. OK: collection referenced in code and exists in DB
	for _, name := range scan.Collections {
		if _, found := findCollection(name, collections); found {
			findings = append(findings, Finding{
				Type:       FindingOK,
				Severity:   SeverityInfo,
				Collection: name,
				Message:    fmt.Sprintf("collection %q exists in database and is referenced in code", name),
			})
		}
	}

	return findings
}

// detectUnindexedQueries finds fields queried in code that have no covering index.
func detectUnindexedQueries(scan *scanner.ScanResult, collections []mongoinspect.CollectionInfo) []Finding {
	if len(scan.FieldRefs) == 0 {
		return nil
	}

	// Group queried fields by collection.
	fieldsByCollection := make(map[string]map[string]bool)
	for _, fr := range scan.FieldRefs {
		lower := strings.ToLower(fr.Collection)
		if fieldsByCollection[lower] == nil {
			fieldsByCollection[lower] = make(map[string]bool)
		}
		fieldsByCollection[lower][fr.Field] = true
	}

	var findings []Finding
	for collName, fields := range fieldsByCollection {
		coll, found := findCollection(collName, collections)
		if !found {
			continue // already reported as MISSING_COLLECTION
		}

		for field := range fields {
			if field == "_id" {
				continue // always indexed
			}
			if isFieldIndexed(field, coll.Indexes) {
				continue
			}
			findings = append(findings, Finding{
				Type:       FindingUnindexedQuery,
				Severity:   SeverityMedium,
				Database:   coll.Database,
				Collection: coll.Name,
				Message:    fmt.Sprintf("field %q is queried in code but has no covering index", field),
			})
		}
	}
	return findings
}

// isFieldIndexed checks if a field is the first key (prefix) of any index.
func isFieldIndexed(field string, indexes []mongoinspect.IndexInfo) bool {
	for _, idx := range indexes {
		if len(idx.Key) > 0 && idx.Key[0].Field == field {
			return true
		}
	}
	return false
}

const (
	suggestMinDocs    int64 = 1000 // skip suggestions for small collections
	suggestMaxPerColl int   = 5    // limit suggestions per collection
)

// suggestIndexes recommends indexes for fields queried in code that have no index.
func suggestIndexes(scan *scanner.ScanResult, collections []mongoinspect.CollectionInfo) []Finding {
	if len(scan.FieldRefs) == 0 {
		return nil
	}

	// Group unindexed fields by collection.
	unindexedByCollection := make(map[string][]string)
	for _, fr := range scan.FieldRefs {
		lower := strings.ToLower(fr.Collection)
		coll, found := findCollection(lower, collections)
		if !found || coll.DocCount < suggestMinDocs {
			continue
		}
		field := fr.Field
		if field == "_id" || isFieldIndexed(field, coll.Indexes) {
			continue
		}
		unindexedByCollection[lower] = append(unindexedByCollection[lower], field)
	}

	var findings []Finding
	for collName, fields := range unindexedByCollection {
		coll, _ := findCollection(collName, collections)

		// Deduplicate fields.
		unique := make(map[string]bool)
		var dedupFields []string
		for _, f := range fields {
			if !unique[f] {
				unique[f] = true
				dedupFields = append(dedupFields, f)
			}
		}

		// Limit suggestions.
		if len(dedupFields) > suggestMaxPerColl {
			dedupFields = dedupFields[:suggestMaxPerColl]
		}

		for _, field := range dedupFields {
			findings = append(findings, Finding{
				Type:       FindingSuggestIndex,
				Severity:   SeverityInfo,
				Database:   coll.Database,
				Collection: coll.Name,
				Message:    fmt.Sprintf("consider index {%s: 1} to cover queries on field %q", field, field),
			})
		}
	}
	return findings
}

// findCollection checks if a collection name exists in the live metadata.
// Comparison is case-insensitive on collection name.
func findCollection(name string, collections []mongoinspect.CollectionInfo) (mongoinspect.CollectionInfo, bool) {
	lower := strings.ToLower(name)
	for _, c := range collections {
		if strings.EqualFold(c.Name, lower) {
			return c, true
		}
	}
	return mongoinspect.CollectionInfo{}, false
}
