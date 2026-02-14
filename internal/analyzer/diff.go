package analyzer

import (
	"fmt"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

// Diff compares code repo references against live MongoDB collections.
func Diff(scan scanner.ScanResult, collections []mongoinspect.CollectionInfo) []Finding {
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

	// 4. OK: collection referenced in code and exists in DB
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

// findCollection checks if a collection name exists in the live metadata.
// Comparison is case-insensitive on collection name.
func findCollection(name string, collections []mongoinspect.CollectionInfo) (mongoinspect.CollectionInfo, bool) {
	lower := strings.ToLower(name)
	for _, c := range collections {
		if strings.ToLower(c.Name) == lower {
			return c, true
		}
	}
	return mongoinspect.CollectionInfo{}, false
}
