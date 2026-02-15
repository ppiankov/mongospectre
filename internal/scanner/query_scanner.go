package scanner

import (
	"regexp"
	"strings"
)

// fieldPattern pairs a regex with the capture group index for the field name.
type fieldPattern struct {
	re         *regexp.Regexp
	fieldGroup int // capture group for field name
}

// queryFieldPatterns extract queried field names from MongoDB operations.
var queryFieldPatterns = []fieldPattern{
	// Go bson.M: bson.M{"status": ..., "created_at": ...}
	// Captures individual keys from bson.M/bson.D literals.
	{re: regexp.MustCompile(`bson\.[MD]\{.*?"([a-zA-Z_][a-zA-Z0-9_.]*)":`), fieldGroup: 1},

	// Go bson.D with Key: bson.D{{Key: "status", ...}}
	{re: regexp.MustCompile(`Key:\s*"([a-zA-Z_][a-zA-Z0-9_.]+)"`), fieldGroup: 1},

	// JS/Python find/update/aggregate with object literal: .find({"status": ...})
	// Also matches .findOne, .updateOne, .deleteMany, .countDocuments, etc.
	{re: regexp.MustCompile(`\.(find|findOne|find_one|findOneAndUpdate|findOneAndDelete|findOneAndReplace|updateOne|updateMany|deleteOne|deleteMany|countDocuments|count_documents|aggregate)\(\s*\{[^}]*?"([a-zA-Z_][a-zA-Z0-9_.]*)":`), fieldGroup: 2},

	// Python dict-style: {"field": ...} in find/update calls (single-quoted keys)
	{re: regexp.MustCompile(`\.(find|findOne|find_one|update_one|update_many|delete_one|delete_many|count_documents|aggregate)\(\s*\{[^}]*?'([a-zA-Z_][a-zA-Z0-9_.]*)':`), fieldGroup: 2},

	// JS unquoted keys in query objects: .find({status: ..., email: ...})
	{re: regexp.MustCompile(`\.(find|findOne|find_one|updateOne|updateMany|deleteOne|deleteMany|countDocuments|count_documents|aggregate)\(\s*\{[^}]*?([a-zA-Z_][a-zA-Z0-9_]*):`), fieldGroup: 2},

	// $match stage: {"$match": {"field": ...}}
	{re: regexp.MustCompile(`["\x60]\$match["\x60]\s*:\s*\{[^}]*?"([a-zA-Z_][a-zA-Z0-9_.]*)":`), fieldGroup: 1},

	// Sort/projection patterns: .sort({"field": 1}), .sort({field: 1})
	{re: regexp.MustCompile(`\.sort\(\s*\{[^}]*?"?([a-zA-Z_][a-zA-Z0-9_.]*)"?\s*:`), fieldGroup: 1},

	// $sort stage: {"$sort": {"field": 1}}
	{re: regexp.MustCompile(`["\x60']\$sort["\x60']\s*:\s*\{[^}]*?"([a-zA-Z_][a-zA-Z0-9_.]*)":`), fieldGroup: 1},

	// $project/$addFields stage: {"$project": {"field": 1}}
	{re: regexp.MustCompile(`["\x60']\$(?:project|addFields)["\x60']\s*:\s*\{[^}]*?"([a-zA-Z_][a-zA-Z0-9_.]*)":`), fieldGroup: 1},

	// $group _id field reference: {"$group": {"_id": "$field"}}
	{re: regexp.MustCompile(`["\x60']\$group["\x60']\s*:\s*\{[^}]*?"?\$([a-zA-Z_][a-zA-Z0-9_.]*)"?`), fieldGroup: 1},

	// $-prefixed field references in aggregation values: "$field", "$field.subfield"
	// Matches things like {"$sum": "$amount"}, "_id": "$category"
	{re: regexp.MustCompile(`:\s*["']\$([a-zA-Z_][a-zA-Z0-9_.]*)["']`), fieldGroup: 1},

	// $unwind: {"$unwind": "$field"} or {"$unwind": {"path": "$field"}}
	{re: regexp.MustCompile(`["\x60']\$unwind["\x60']\s*:\s*["']\$([a-zA-Z_][a-zA-Z0-9_.]*)["']`), fieldGroup: 1},

	// $lookup localField/foreignField: {"localField": "userId", "foreignField": "_id"}
	{re: regexp.MustCompile(`["'](?:localField|foreignField)["']\s*:\s*["']([a-zA-Z_][a-zA-Z0-9_.]*)["']`), fieldGroup: 1},
}

// fieldMatch holds a field name extracted from a query pattern on a single line.
type fieldMatch struct {
	Field string
}

// ScanLineFields checks a single line for queried field names.
// It returns all field names found in MongoDB query patterns.
func ScanLineFields(line string) []fieldMatch {
	var matches []fieldMatch
	seen := make(map[string]bool)

	for _, p := range queryFieldPatterns {
		for _, m := range p.re.FindAllStringSubmatch(line, -1) {
			field := m[p.fieldGroup]
			if !isValidFieldName(field) {
				continue
			}
			if seen[field] {
				continue
			}
			seen[field] = true
			matches = append(matches, fieldMatch{Field: field})
		}
	}

	// Also extract all keys from object literals on this line.
	// This catches multi-key queries that the primary patterns miss.
	for _, f := range extractObjectKeys(line) {
		if !seen[f] {
			seen[f] = true
			matches = append(matches, fieldMatch{Field: f})
		}
	}

	// Extract $-prefixed field references from pipeline stages (e.g. "$firstName" in arrays).
	for _, f := range extractFieldRefs(line) {
		if !seen[f] {
			seen[f] = true
			matches = append(matches, fieldMatch{Field: f})
		}
	}

	return matches
}

// objectKeyContextRe matches lines that are clearly MongoDB query contexts.
var objectKeyContextRe = regexp.MustCompile(`(?i)\.(find|findOne|find_one|findOneAndUpdate|findOneAndDelete|findOneAndReplace|updateOne|updateMany|update_one|update_many|deleteOne|deleteMany|delete_one|delete_many|countDocuments|count_documents|aggregate|sort)\(`)

// pipelineStageContextRe matches lines that contain aggregation pipeline stages.
var pipelineStageContextRe = regexp.MustCompile(`["` + "`" + `']\$(?:match|sort|project|group|addFields|set|bucket|facet|lookup|unwind)["` + "`" + `']`)

// fieldRefRe extracts $-prefixed field references that are values (not keys).
// The optional second capture group detects a trailing colon to distinguish
// operator keys ("$match":) from field references ("$firstName").
var fieldRefRe = regexp.MustCompile(`["']\$([a-zA-Z_][a-zA-Z0-9_.]*)["'](\s*:)?`)

// objectKeyRe extracts all quoted keys from an object literal.
var objectKeyRe = regexp.MustCompile(`["']([a-zA-Z_][a-zA-Z0-9_.]*)["']\s*:`)

// extractObjectKeys pulls all keys from object literals on lines that
// look like MongoDB query calls. This catches the second, third, etc. keys
// in multi-field queries like .find({"status": 1, "created_at": -1}).
func extractObjectKeys(line string) []string {
	if !objectKeyContextRe.MatchString(line) && !pipelineStageContextRe.MatchString(line) {
		return nil
	}

	var fields []string
	for _, m := range objectKeyRe.FindAllStringSubmatch(line, -1) {
		field := m[1]
		if isValidFieldName(field) {
			fields = append(fields, field)
		}
	}
	return fields
}

// extractFieldRefs pulls $-prefixed field references from pipeline stage lines.
// This catches references like "$firstName" inside arrays that the colon-prefixed
// pattern misses.
func extractFieldRefs(line string) []string {
	if !pipelineStageContextRe.MatchString(line) {
		return nil
	}

	var fields []string
	for _, m := range fieldRefRe.FindAllStringSubmatch(line, -1) {
		field := m[1]
		// If followed by ':', this is a key (operator), not a field ref.
		if m[2] != "" {
			continue
		}
		if isValidFieldName(field) {
			fields = append(fields, field)
		}
	}
	return fields
}

// isValidFieldName filters out obvious non-field strings.
func isValidFieldName(name string) bool {
	if len(name) == 0 || len(name) > 120 {
		return false
	}
	// Skip MongoDB operators
	if strings.HasPrefix(name, "$") {
		return false
	}
	// Skip common non-field identifiers
	switch name {
	case "true", "false", "null", "nil", "undefined",
		"find", "findOne", "findOneAndUpdate", "findOneAndDelete", "findOneAndReplace",
		"updateOne", "updateMany", "deleteOne", "deleteMany",
		"countDocuments", "count_documents", "aggregate",
		"sort", "limit", "skip", "projection",
		"bson", "Key", "Value",
		"from", "as", "localField", "foreignField", "let", "pipeline", "path", "preserveNullAndEmptyArrays",
		"input", "cond", "in", "then", "else", "case":
		return false
	}
	return true
}
