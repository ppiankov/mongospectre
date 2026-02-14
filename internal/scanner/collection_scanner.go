package scanner

import (
	"regexp"
	"strings"
)

// pattern pairs a compiled regex with the capture group index and pattern type.
type pattern struct {
	re      *regexp.Regexp
	group   int // capture group index for the collection name
	patType PatternType
}

// collectionPatterns are the regexes used to find collection references.
// Order matters: more specific patterns should come first.
var collectionPatterns = []pattern{
	// Go: db.Collection("products")
	{re: regexp.MustCompile(`\.Collection\(\s*"([^"]+)"\s*\)`), group: 1, patType: PatternDriverCall},

	// JS/TS/Java: db.collection("users"), db.getCollection("users")
	{re: regexp.MustCompile(`\.(?:collection|getCollection|GetCollection)\(\s*["']([^"']+)["']\s*\)`), group: 1, patType: PatternDriverCall},

	// Mongoose: mongoose.model("User", ...) or model("User", ...)
	{re: regexp.MustCompile(`(?:mongoose\.)?model\(\s*["']([^"']+)["']`), group: 1, patType: PatternORM},

	// Python MongoEngine: class User(Document): meta = {'collection': 'users'}
	{re: regexp.MustCompile(`['"]collection['"]\s*:\s*["']([^"']+)["']`), group: 1, patType: PatternORM},

	// Bracket access: db["users"], db['users']
	{re: regexp.MustCompile(`db\[["']([^"']+)["']\]`), group: 1, patType: PatternBracket},

	// PyMongo dot access: db.users.find, db.users.insert, db.users.aggregate, etc.
	// Must be followed by a MongoDB operation to avoid false positives.
	{re: regexp.MustCompile(`db\.([a-z][a-z0-9_]+)\.(find|insert|update|delete|aggregate|count|distinct|drop|create_index|remove|replace|bulk_write|watch|rename|map_reduce)`), group: 1, patType: PatternDotAccess},
}

// ScanLine checks a single line of source code for collection references.
func ScanLine(line string) []match {
	var matches []match
	for _, p := range collectionPatterns {
		for _, m := range p.re.FindAllStringSubmatch(line, -1) {
			name := m[p.group]
			if isValidCollectionName(name) {
				matches = append(matches, match{
					Collection: name,
					Pattern:    p.patType,
				})
			}
		}
	}
	return dedupMatches(matches)
}

type match struct {
	Collection string
	Pattern    PatternType
}

// isValidCollectionName filters out obvious non-collection strings.
func isValidCollectionName(name string) bool {
	if len(name) == 0 || len(name) > 120 {
		return false
	}
	// Skip variables/templates
	if strings.ContainsAny(name, "${}") {
		return false
	}
	// Skip paths
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	return true
}

// dedupMatches removes duplicate collection names from a single line.
func dedupMatches(matches []match) []match {
	seen := make(map[string]bool)
	out := make([]match, 0, len(matches))
	for _, m := range matches {
		if !seen[m.Collection] {
			seen[m.Collection] = true
			out = append(out, m)
		}
	}
	return out
}
