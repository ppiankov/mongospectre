package scanner

import (
	"regexp"
	"strings"
)

// fieldPattern pairs a regex with the capture group index for the field name.
type fieldPattern struct {
	re         *regexp.Regexp
	fieldGroup int // capture group for field name
	usage      FieldUsage
}

// queryFieldPatterns extract queried field names from MongoDB operations.
var queryFieldPatterns = []fieldPattern{
	// Go bson.M: bson.M{"status": ..., "created_at": ...}
	// Captures individual keys from bson.M/bson.D literals.
	{re: regexp.MustCompile(`bson\.[MD]\{.*?"([a-zA-Z_][a-zA-Z0-9_.]*)":`), fieldGroup: 1, usage: FieldUsageEquality},

	// Go bson.D with Key: bson.D{{Key: "status", ...}}
	{re: regexp.MustCompile(`Key:\s*"([a-zA-Z_][a-zA-Z0-9_.]+)"`), fieldGroup: 1, usage: FieldUsageEquality},

	// JS/Python find/update/aggregate with object literal: .find({"status": ...})
	// Also matches .findOne, .updateOne, .deleteMany, .countDocuments, etc.
	{
		re:         regexp.MustCompile(`\.(find|findOne|find_one|findOneAndUpdate|findOneAndDelete|findOneAndReplace|updateOne|updateMany|deleteOne|deleteMany|countDocuments|count_documents|aggregate)\(\s*\{[^}]*?"([a-zA-Z_][a-zA-Z0-9_.]*)":`),
		fieldGroup: 2,
		usage:      FieldUsageEquality,
	},

	// Python dict-style: {"field": ...} in find/update calls (single-quoted keys)
	{
		re:         regexp.MustCompile(`\.(find|findOne|find_one|update_one|update_many|delete_one|delete_many|count_documents|aggregate)\(\s*\{[^}]*?'([a-zA-Z_][a-zA-Z0-9_.]*)':`),
		fieldGroup: 2,
		usage:      FieldUsageEquality,
	},

	// JS unquoted keys in query objects: .find({status: ..., email: ...})
	{
		re:         regexp.MustCompile(`\.(find|findOne|find_one|updateOne|updateMany|deleteOne|deleteMany|countDocuments|count_documents|aggregate)\(\s*\{[^}]*?([a-zA-Z_][a-zA-Z0-9_]*):`),
		fieldGroup: 2,
		usage:      FieldUsageEquality,
	},

	// $match stage: {"$match": {"field": ...}}
	{re: regexp.MustCompile(`["\x60]\$match["\x60]\s*:\s*\{[^}]*?"([a-zA-Z_][a-zA-Z0-9_.]*)":`), fieldGroup: 1, usage: FieldUsageEquality},

	// $project/$addFields stage: {"$project": {"field": 1}}
	{re: regexp.MustCompile(`["\x60']\$(?:project|addFields)["\x60']\s*:\s*\{[^}]*?"([a-zA-Z_][a-zA-Z0-9_.]*)":`), fieldGroup: 1, usage: FieldUsageUnknown},

	// $group _id field reference: {"$group": {"_id": "$field"}}
	{re: regexp.MustCompile(`["\x60']\$group["\x60']\s*:\s*\{[^}]*?"?\$([a-zA-Z_][a-zA-Z0-9_.]*)"?`), fieldGroup: 1, usage: FieldUsageUnknown},

	// $-prefixed field references in aggregation values: "$field", "$field.subfield"
	// Matches things like {"$sum": "$amount"}, "_id": "$category"
	{re: regexp.MustCompile(`:\s*["']\$([a-zA-Z_][a-zA-Z0-9_.]*)["']`), fieldGroup: 1, usage: FieldUsageUnknown},

	// $unwind: {"$unwind": "$field"} or {"$unwind": {"path": "$field"}}
	{re: regexp.MustCompile(`["\x60']\$unwind["\x60']\s*:\s*["']\$([a-zA-Z_][a-zA-Z0-9_.]*)["']`), fieldGroup: 1, usage: FieldUsageUnknown},

	// $lookup localField/foreignField: {"localField": "userId", "foreignField": "_id"}
	{re: regexp.MustCompile(`["'](?:localField|foreignField)["']\s*:\s*["']([a-zA-Z_][a-zA-Z0-9_.]*)["']`), fieldGroup: 1, usage: FieldUsageUnknown},
}

// fieldMatch holds a field name extracted from a query pattern on a single line.
type fieldMatch struct {
	Field        string
	Usage        FieldUsage
	Direction    int
	QueryContext string
}

// ScanLineFields checks a single line for queried field names.
// It returns all field names found in MongoDB query patterns.
func ScanLineFields(line string) []fieldMatch {
	queryContext := queryContextFromLine(line)
	byField := make(map[string]fieldMatch)
	var order []string
	addMatch := func(m fieldMatch) {
		if !isValidFieldName(m.Field) {
			return
		}

		existing, ok := byField[m.Field]
		if !ok {
			if m.QueryContext == "" {
				m.QueryContext = queryContext
			}
			byField[m.Field] = m
			order = append(order, m.Field)
			return
		}

		// Keep the strongest signal if the same field is seen multiple ways.
		if fieldUsagePriority(m.Usage) > fieldUsagePriority(existing.Usage) {
			existing.Usage = m.Usage
			if m.Direction != 0 {
				existing.Direction = m.Direction
			}
		} else if existing.Direction == 0 && m.Direction != 0 {
			existing.Direction = m.Direction
		}
		if existing.QueryContext == "" {
			existing.QueryContext = m.QueryContext
		}
		byField[m.Field] = existing
	}

	for _, p := range queryFieldPatterns {
		for _, m := range p.re.FindAllStringSubmatch(line, -1) {
			field := m[p.fieldGroup]
			addMatch(fieldMatch{
				Field:        field,
				Usage:        p.usage,
				QueryContext: queryContext,
			})
		}
	}

	sortFields := extractSortFields(line)
	sortSet := make(map[string]bool, len(sortFields))
	for _, sf := range sortFields {
		sf.QueryContext = queryContext
		sortSet[sf.Field] = true
		addMatch(sf)
	}

	rangeFields := extractRangeFields(line)
	rangeSet := make(map[string]bool, len(rangeFields))
	for _, rf := range rangeFields {
		rangeSet[rf] = true
		addMatch(fieldMatch{
			Field:        rf,
			Usage:        FieldUsageRange,
			QueryContext: queryContext,
		})
	}

	// Also extract all keys from object literals on this line.
	// This catches multi-key queries that the primary patterns miss.
	for _, f := range extractObjectKeys(line) {
		if sortSet[f] {
			continue
		}
		usage := FieldUsageEquality
		if rangeSet[f] {
			usage = FieldUsageRange
		}
		addMatch(fieldMatch{Field: f, Usage: usage, QueryContext: queryContext})
	}

	// Extract $-prefixed field references from pipeline stages (e.g. "$firstName" in arrays).
	for _, f := range extractFieldRefs(line) {
		addMatch(fieldMatch{Field: f, Usage: FieldUsageUnknown, QueryContext: queryContext})
	}

	out := make([]fieldMatch, 0, len(order))
	for _, field := range order {
		out = append(out, byField[field])
	}

	return out
}

// objectKeyContextRe matches lines that are clearly MongoDB query contexts.
var objectKeyContextRe = regexp.MustCompile(`(?i)\.(find|findOne|find_one|findOneAndUpdate|findOneAndDelete|findOneAndReplace|updateOne|updateMany|update_one|update_many|deleteOne|deleteMany|delete_one|delete_many|countDocuments|count_documents|aggregate|sort)\(`)

// pipelineStageContextRe matches lines that contain aggregation pipeline stages.
var pipelineStageContextRe = regexp.MustCompile(`["` + "`" + `']\$(?:match|sort|project|group|addFields|set|bucket|facet|lookup|unwind)["` + "`" + `']`)

// fieldRefRe extracts $-prefixed field references that are values (not keys).
// The optional second capture group detects a trailing colon to distinguish
// operator keys ("$match":) from field references ("$firstName").
var fieldRefRe = regexp.MustCompile(`["']\$([a-zA-Z_][a-zA-Z0-9_.]*)["'](\s*:)?`)

// objectKeyRe extracts object keys from query object literals.
// The optional (\$?) prefix captures dollar signs so callers can skip operators.
var objectKeyRe = regexp.MustCompile(`["']?(\$?)([a-zA-Z_][a-zA-Z0-9_.]*)["']?\s*:`)

// sortCallRe extracts the sort object body in .sort({...}) calls.
var sortCallRe = regexp.MustCompile(`\.sort\(\s*\{([^}]*)\}`)

// sortStageRe extracts the sort object body in {"$sort": {...}} pipeline stages.
var sortStageRe = regexp.MustCompile(`["` + "`" + `']\$sort["` + "`" + `']\s*:\s*\{([^}]*)\}`)

// sortPairRe extracts "field: direction" pairs from sort object bodies.
var sortPairRe = regexp.MustCompile(`["']?([a-zA-Z_][a-zA-Z0-9_.]*)["']?\s*:\s*(-?1)\b`)

// rangeFieldRe extracts fields used with range-like operators.
var rangeFieldRe = regexp.MustCompile(`["']?([a-zA-Z_][a-zA-Z0-9_.]*)["']?\s*:\s*\{\s*["']?\$(?:gt|gte|lt|lte|ne|nin|in|regex|not)\b`)

// queryContextRe extracts the primary call name for grouping query contexts.
var queryContextRe = regexp.MustCompile(`\.(findOneAndUpdate|findOneAndDelete|findOneAndReplace|findOne|find_one|find|updateOne|updateMany|update_one|update_many|deleteOne|deleteMany|delete_one|delete_many|countDocuments|count_documents|aggregate|sort)\(`)

// extractObjectKeys pulls all keys from object literals on lines that
// look like MongoDB query calls. This catches the second, third, etc. keys
// in multi-field queries like .find({"status": 1, "created_at": -1}).
func extractObjectKeys(line string) []string {
	if !objectKeyContextRe.MatchString(line) && !pipelineStageContextRe.MatchString(line) {
		return nil
	}

	var fields []string
	for _, m := range objectKeyRe.FindAllStringSubmatch(line, -1) {
		// Skip $-prefixed operators (e.g. "$gt", "$match", "$unwind").
		if m[1] == "$" {
			continue
		}
		field := m[2]
		if isValidFieldName(field) {
			fields = append(fields, field)
		}
	}
	return fields
}

// extractSortFields extracts sort keys and directions from query lines.
func extractSortFields(line string) []fieldMatch {
	var fields []fieldMatch
	seen := make(map[string]bool)

	appendSegmentMatches := func(re *regexp.Regexp) {
		for _, segment := range re.FindAllStringSubmatch(line, -1) {
			body := segment[1]
			for _, m := range sortPairRe.FindAllStringSubmatch(body, -1) {
				field := m[1]
				if !isValidFieldName(field) || seen[field] {
					continue
				}
				direction := 1
				if strings.HasPrefix(m[2], "-") {
					direction = -1
				}
				seen[field] = true
				fields = append(fields, fieldMatch{
					Field:     field,
					Usage:     FieldUsageSort,
					Direction: direction,
				})
			}
		}
	}

	appendSegmentMatches(sortCallRe)
	appendSegmentMatches(sortStageRe)
	return fields
}

// extractRangeFields extracts query fields constrained by range-like operators.
func extractRangeFields(line string) []string {
	var fields []string
	seen := make(map[string]bool)
	for _, m := range rangeFieldRe.FindAllStringSubmatch(line, -1) {
		field := m[1]
		if !isValidFieldName(field) || seen[field] {
			continue
		}
		seen[field] = true
		fields = append(fields, field)
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

func fieldUsagePriority(u FieldUsage) int {
	switch u {
	case FieldUsageSort:
		return 4
	case FieldUsageRange:
		return 3
	case FieldUsageEquality:
		return 2
	default:
		return 1
	}
}

func queryContextFromLine(line string) string {
	if m := queryContextRe.FindStringSubmatch(line); len(m) > 1 {
		return strings.ToLower(m[1])
	}
	if pipelineStageContextRe.MatchString(line) {
		return "aggregate"
	}
	return "query"
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
