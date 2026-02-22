package analyzer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

// Diff compares code repo references against live MongoDB collections.
func Diff(scan *scanner.ScanResult, collections []mongoinspect.CollectionInfo) []Finding {
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

	// 4b. SUGGEST_INDEX: individual field-level index suggestions for large collections
	findings = append(findings, suggestFieldIndexes(scan, collections)...)

	// 5. Smart index recommendations and index-shape quality findings.
	findings = append(findings, recommendSmartIndexes(scan, collections)...)

	// 6. VALIDATOR_*: JSON schema validator drift for code write patterns.
	findings = append(findings, detectValidatorDrift(scan, collections)...)

	// 7. DYNAMIC_COLLECTION: variable collection name could not be resolved
	for _, dr := range scan.DynamicRefs {
		findings = append(findings, Finding{
			Type:     FindingDynamicCollection,
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("collection name from variable %q could not be resolved statically (%s:%d)", dr.Variable, dr.File, dr.Line),
		})
	}

	// 8. OK: collection referenced in code and exists in DB
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
		if !isQueryableUsage(fr.Usage) {
			continue
		}
		lower := strings.ToLower(fr.Collection)
		if fieldsByCollection[lower] == nil {
			fieldsByCollection[lower] = make(map[string]bool)
		}
		fieldsByCollection[lower][fr.Field] = true
	}

	var findings []Finding
	collNames := make([]string, 0, len(fieldsByCollection))
	for collName := range fieldsByCollection {
		collNames = append(collNames, collName)
	}
	sort.Strings(collNames)

	for _, collName := range collNames {
		fields := fieldsByCollection[collName]
		coll, found := findCollection(collName, collections)
		if !found {
			continue // already reported as MISSING_COLLECTION
		}

		fieldNames := make([]string, 0, len(fields))
		for field := range fields {
			fieldNames = append(fieldNames, field)
		}
		sort.Strings(fieldNames)

		for _, field := range fieldNames {
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

// suggestFieldIndexes recommends individual field indexes for unindexed query
// fields on collections that exceed suggestMinDocs.
func suggestFieldIndexes(scan *scanner.ScanResult, collections []mongoinspect.CollectionInfo) []Finding {
	if len(scan.FieldRefs) == 0 {
		return nil
	}

	fieldsByCollection := make(map[string]map[string]bool)
	for _, fr := range scan.FieldRefs {
		if !isQueryableUsage(fr.Usage) {
			continue
		}
		lower := strings.ToLower(fr.Collection)
		if fieldsByCollection[lower] == nil {
			fieldsByCollection[lower] = make(map[string]bool)
		}
		fieldsByCollection[lower][fr.Field] = true
	}

	var findings []Finding
	collNames := make([]string, 0, len(fieldsByCollection))
	for collName := range fieldsByCollection {
		collNames = append(collNames, collName)
	}
	sort.Strings(collNames)

	for _, collName := range collNames {
		fields := fieldsByCollection[collName]
		coll, found := findCollection(collName, collections)
		if !found || coll.DocCount < suggestMinDocs {
			continue
		}

		fieldNames := make([]string, 0, len(fields))
		for field := range fields {
			fieldNames = append(fieldNames, field)
		}
		sort.Strings(fieldNames)

		for _, field := range fieldNames {
			if field == "_id" {
				continue
			}
			if isFieldIndexed(field, coll.Indexes) {
				continue
			}
			findings = append(findings, Finding{
				Type:       FindingSuggestIndex,
				Severity:   SeverityInfo,
				Database:   coll.Database,
				Collection: coll.Name,
				Message:    fmt.Sprintf("consider adding an index on field %q (collection has %d documents)", field, coll.DocCount),
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

type queryRole int

const (
	queryRoleUnknown queryRole = iota
	queryRoleEquality
	queryRoleRange
	queryRoleSort
)

type queryField struct {
	name      string
	role      queryRole
	direction int
}

type queryContext struct {
	file         string
	line         int
	queryContext string
	order        []string
	fields       map[string]queryField
}

type suggestionPattern struct {
	key       []mongoinspect.KeyField
	signature string
	frequency int
	files     map[string]bool
}

type suggestionCandidate struct {
	pattern      suggestionPattern
	replacements []string
	score        int
}

type warningCandidate struct {
	indexName string
	pattern   suggestionPattern
}

type partialCoverageCandidate struct {
	indexName string
	pattern   suggestionPattern
	prefixLen int
}

// recommendSmartIndexes suggests ESR-ordered compound indexes, highlights bad
// index ordering, surfaces redundant indexes, and reports near-covered queries.
func recommendSmartIndexes(scan *scanner.ScanResult, collections []mongoinspect.CollectionInfo) []Finding {
	if scan == nil || len(scan.FieldRefs) == 0 {
		return nil
	}

	contextsByCollection := buildQueryContexts(scan.FieldRefs)
	if len(contextsByCollection) == 0 {
		return nil
	}

	collNames := make([]string, 0, len(contextsByCollection))
	for collName := range contextsByCollection {
		collNames = append(collNames, collName)
	}
	sort.Strings(collNames)

	var findings []Finding
	for _, collName := range collNames {
		coll, found := findCollection(collName, collections)
		if !found || coll.DocCount < suggestMinDocs {
			continue
		}
		if indexStatsUnavailable(coll) {
			continue
		}

		patterns := buildSuggestionPatterns(contextsByCollection[collName])
		if len(patterns) == 0 {
			continue
		}

		findings = append(findings, detectRedundantIndexes(coll)...)
		findings = append(findings, detectIndexOrderWarnings(coll, patterns)...)
		findings = append(findings, detectPartialCoverage(coll, patterns)...)
		findings = append(findings, detectCompoundIndexSuggestions(coll, patterns)...)
	}

	return findings
}

func detectCompoundIndexSuggestions(coll mongoinspect.CollectionInfo, patterns []suggestionPattern) []Finding {
	candidates := make([]suggestionCandidate, 0, len(patterns))
	for _, pattern := range patterns {
		if isPatternCoveredByExisting(pattern.key, coll.Indexes) {
			continue
		}

		replacements := findReplacementIndexes(pattern.key, coll.Indexes)
		candidates = append(candidates, suggestionCandidate{
			pattern:      pattern,
			replacements: replacements,
			score:        pattern.frequency,
		})
	}
	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if len(candidates[i].pattern.key) != len(candidates[j].pattern.key) {
			return len(candidates[i].pattern.key) > len(candidates[j].pattern.key)
		}
		if len(candidates[i].replacements) != len(candidates[j].replacements) {
			return len(candidates[i].replacements) > len(candidates[j].replacements)
		}
		return candidates[i].pattern.signature < candidates[j].pattern.signature
	})

	if len(candidates) > suggestMaxPerColl {
		candidates = candidates[:suggestMaxPerColl]
	}

	findings := make([]Finding, 0, len(candidates))
	for _, candidate := range candidates {
		replaces := "none"
		if len(candidate.replacements) > 0 {
			replaces = strings.Join(candidate.replacements, ", ")
		}
		findings = append(findings, Finding{
			Type:       FindingCompoundIndexSuggest,
			Severity:   SeverityInfo,
			Database:   coll.Database,
			Collection: coll.Name,
			Message: fmt.Sprintf(
				"consider compound index %s (impact score %d: %d query patterns across %d files); replaces: %s",
				formatIndexSpec(candidate.pattern.key),
				candidate.score,
				candidate.pattern.frequency,
				len(candidate.pattern.files),
				replaces,
			),
		})
	}

	return findings
}

func detectIndexOrderWarnings(coll mongoinspect.CollectionInfo, patterns []suggestionPattern) []Finding {
	if len(coll.Indexes) == 0 {
		return nil
	}

	bestByIndex := make(map[string]warningCandidate)
	for _, pattern := range patterns {
		for _, idx := range coll.Indexes {
			if idx.Name == "_id_" || len(idx.Key) < len(pattern.key) {
				continue
			}
			if isKeyPrefix(pattern.key, idx.Key) {
				continue
			}
			if !sameKeySetWithDirection(idx.Key[:len(pattern.key)], pattern.key) {
				continue
			}

			current, exists := bestByIndex[idx.Name]
			if !exists || pattern.frequency > current.pattern.frequency {
				bestByIndex[idx.Name] = warningCandidate{
					indexName: idx.Name,
					pattern:   pattern,
				}
			}
		}
	}

	if len(bestByIndex) == 0 {
		return nil
	}

	indexNames := make([]string, 0, len(bestByIndex))
	for name := range bestByIndex {
		indexNames = append(indexNames, name)
	}
	sort.Strings(indexNames)

	findings := make([]Finding, 0, len(indexNames))
	for _, idxName := range indexNames {
		candidate := bestByIndex[idxName]
		findings = append(findings, Finding{
			Type:       FindingIndexOrderWarning,
			Severity:   SeverityLow,
			Database:   coll.Database,
			Collection: coll.Name,
			Index:      idxName,
			Message: fmt.Sprintf(
				"index %q has suboptimal order for observed query patterns; prefer %s (impact score %d)",
				idxName,
				formatIndexSpec(candidate.pattern.key),
				candidate.pattern.frequency,
			),
		})
	}

	return findings
}

func detectPartialCoverage(coll mongoinspect.CollectionInfo, patterns []suggestionPattern) []Finding {
	candidatesByKey := make(map[string]partialCoverageCandidate)
	for _, pattern := range patterns {
		if len(pattern.key) < 3 || isPatternCoveredByExisting(pattern.key, coll.Indexes) {
			continue
		}

		bestIdx, bestPrefix := bestCoverageIndex(pattern.key, coll.Indexes)
		if bestIdx == "" || bestPrefix != len(pattern.key)-1 {
			continue
		}

		key := bestIdx + "|" + pattern.signature
		candidatesByKey[key] = partialCoverageCandidate{
			indexName: bestIdx,
			pattern:   pattern,
			prefixLen: bestPrefix,
		}
	}
	if len(candidatesByKey) == 0 {
		return nil
	}

	candidateKeys := make([]string, 0, len(candidatesByKey))
	for key := range candidatesByKey {
		candidateKeys = append(candidateKeys, key)
	}
	sort.Slice(candidateKeys, func(i, j int) bool {
		left := candidatesByKey[candidateKeys[i]]
		right := candidatesByKey[candidateKeys[j]]
		if left.pattern.frequency != right.pattern.frequency {
			return left.pattern.frequency > right.pattern.frequency
		}
		return candidateKeys[i] < candidateKeys[j]
	})

	findings := make([]Finding, 0, len(candidateKeys))
	for _, key := range candidateKeys {
		candidate := candidatesByKey[key]
		missing := candidate.pattern.key[candidate.prefixLen:]
		findings = append(findings, Finding{
			Type:       FindingPartialCoverage,
			Severity:   SeverityInfo,
			Database:   coll.Database,
			Collection: coll.Name,
			Index:      candidate.indexName,
			Message: fmt.Sprintf(
				"index %q partially covers %d/%d ESR fields; add %s for full coverage (impact score %d)",
				candidate.indexName,
				candidate.prefixLen,
				len(candidate.pattern.key),
				formatIndexSpec(missing),
				candidate.pattern.frequency,
			),
		})
	}

	return findings
}

func detectRedundantIndexes(coll mongoinspect.CollectionInfo) []Finding {
	bestCover := make(map[string]string) // redundant index -> covering index

	for i, idx := range coll.Indexes {
		if !isReplaceableIndex(idx) {
			continue
		}
		for j, other := range coll.Indexes {
			if i == j || !isReplaceableIndex(other) {
				continue
			}
			if !isKeyPrefix(idx.Key, other.Key) || len(idx.Key) >= len(other.Key) {
				continue
			}

			coverName, ok := bestCover[idx.Name]
			if !ok {
				bestCover[idx.Name] = other.Name
				continue
			}

			cover := findIndexByName(coverName, coll.Indexes)
			if len(cover.Key) == 0 || len(other.Key) < len(cover.Key) || (len(other.Key) == len(cover.Key) && other.Name < cover.Name) {
				bestCover[idx.Name] = other.Name
			}
		}
	}

	if len(bestCover) == 0 {
		return nil
	}

	names := make([]string, 0, len(bestCover))
	for name := range bestCover {
		names = append(names, name)
	}
	sort.Strings(names)

	findings := make([]Finding, 0, len(names))
	for _, name := range names {
		findings = append(findings, Finding{
			Type:       FindingRedundantIndex,
			Severity:   SeverityLow,
			Database:   coll.Database,
			Collection: coll.Name,
			Index:      name,
			Message: fmt.Sprintf(
				"index %q is redundant; query coverage is provided by %q",
				name,
				bestCover[name],
			),
		})
	}
	return findings
}

func findReplacementIndexes(candidate []mongoinspect.KeyField, indexes []mongoinspect.IndexInfo) []string {
	type namedIndex struct {
		name    string
		keySize int
	}

	var out []namedIndex
	for _, idx := range indexes {
		if !isReplaceableIndex(idx) {
			continue
		}
		if len(idx.Key) >= len(candidate) || !isKeyPrefix(idx.Key, candidate) {
			continue
		}
		out = append(out, namedIndex{name: idx.Name, keySize: len(idx.Key)})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].keySize != out[j].keySize {
			return out[i].keySize < out[j].keySize
		}
		return out[i].name < out[j].name
	})

	names := make([]string, 0, len(out))
	for _, idx := range out {
		names = append(names, idx.name)
	}
	return names
}

func buildSuggestionPatterns(contexts map[string]*queryContext) []suggestionPattern {
	aggregated := make(map[string]suggestionPattern)

	ctxKeys := make([]string, 0, len(contexts))
	for key := range contexts {
		ctxKeys = append(ctxKeys, key)
	}
	sort.Strings(ctxKeys)

	for _, ctxKey := range ctxKeys {
		ctx := contexts[ctxKey]
		patternKey := contextToESRKey(ctx)
		if len(patternKey) < 2 {
			continue
		}
		signature := keySignature(patternKey)
		pattern, ok := aggregated[signature]
		if !ok {
			pattern = suggestionPattern{
				key:       patternKey,
				signature: signature,
				files:     make(map[string]bool),
			}
		}
		pattern.frequency++
		pattern.files[ctx.file] = true
		aggregated[signature] = pattern
	}

	patterns := make([]suggestionPattern, 0, len(aggregated))
	for _, pattern := range aggregated {
		patterns = append(patterns, pattern)
	}
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].frequency != patterns[j].frequency {
			return patterns[i].frequency > patterns[j].frequency
		}
		if len(patterns[i].key) != len(patterns[j].key) {
			return len(patterns[i].key) > len(patterns[j].key)
		}
		return patterns[i].signature < patterns[j].signature
	})
	return patterns
}

func contextToESRKey(ctx *queryContext) []mongoinspect.KeyField {
	var eq, sortFields, rng []mongoinspect.KeyField
	for _, fieldKey := range ctx.order {
		field, ok := ctx.fields[fieldKey]
		if !ok {
			continue
		}
		if field.name == "_id" {
			continue
		}

		switch field.role {
		case queryRoleEquality:
			eq = append(eq, mongoinspect.KeyField{Field: field.name, Direction: 1})
		case queryRoleSort:
			dir := field.direction
			if dir != -1 {
				dir = 1
			}
			sortFields = append(sortFields, mongoinspect.KeyField{Field: field.name, Direction: dir})
		case queryRoleRange:
			rng = append(rng, mongoinspect.KeyField{Field: field.name, Direction: 1})
		}
	}

	var out []mongoinspect.KeyField
	out = append(out, eq...)
	out = append(out, sortFields...)
	out = append(out, rng...)
	return out
}

func buildQueryContexts(fieldRefs []scanner.FieldRef) map[string]map[string]*queryContext {
	contextsByCollection := make(map[string]map[string]*queryContext)

	for _, ref := range fieldRefs {
		if ref.Field == "" || !isQueryableUsage(ref.Usage) {
			continue
		}

		collectionKey := strings.ToLower(ref.Collection)
		if collectionKey == "" {
			continue
		}
		if contextsByCollection[collectionKey] == nil {
			contextsByCollection[collectionKey] = make(map[string]*queryContext)
		}

		ctxName := strings.ToLower(strings.TrimSpace(ref.QueryContext))
		ctxKey := ref.File + ":" + strconv.Itoa(ref.Line) + ":" + ctxName
		ctx, ok := contextsByCollection[collectionKey][ctxKey]
		if !ok {
			ctx = &queryContext{
				file:         ref.File,
				line:         ref.Line,
				queryContext: ctxName,
				fields:       make(map[string]queryField),
			}
			contextsByCollection[collectionKey][ctxKey] = ctx
		}
		addFieldToContext(ctx, ref)
	}

	return contextsByCollection
}

func addFieldToContext(ctx *queryContext, ref scanner.FieldRef) {
	fieldName := strings.TrimSpace(ref.Field)
	if fieldName == "" {
		return
	}
	fieldKey := strings.ToLower(fieldName)
	role := usageToRole(ref.Usage)
	direction := 0
	if role == queryRoleSort {
		direction = ref.Direction
		if direction != -1 {
			direction = 1
		}
	}

	existing, ok := ctx.fields[fieldKey]
	if !ok {
		ctx.order = append(ctx.order, fieldKey)
		ctx.fields[fieldKey] = queryField{
			name:      fieldName,
			role:      role,
			direction: direction,
		}
		return
	}

	if rolePriority(role) > rolePriority(existing.role) {
		existing.role = role
		if direction != 0 {
			existing.direction = direction
		}
	} else if existing.role == queryRoleSort && existing.direction == 0 && direction != 0 {
		existing.direction = direction
	}
	ctx.fields[fieldKey] = existing
}

func usageToRole(usage scanner.FieldUsage) queryRole {
	switch usage {
	case scanner.FieldUsageEquality:
		return queryRoleEquality
	case scanner.FieldUsageRange:
		return queryRoleRange
	case scanner.FieldUsageSort:
		return queryRoleSort
	default:
		return queryRoleUnknown
	}
}

func rolePriority(role queryRole) int {
	switch role {
	case queryRoleSort:
		return 4
	case queryRoleRange:
		return 3
	case queryRoleEquality:
		return 2
	default:
		return 1
	}
}

func isQueryableUsage(usage scanner.FieldUsage) bool {
	switch usage {
	case scanner.FieldUsageUnknown:
		return false
	default:
		// Empty usage (no explicit annotation) is treated as queryable because
		// a FieldRef that exists means code references that field in a query context.
		return true
	}
}

func indexStatsUnavailable(coll mongoinspect.CollectionInfo) bool {
	hasSecondary := false
	hasStats := false
	for _, idx := range coll.Indexes {
		if idx.Name == "_id_" {
			continue
		}
		hasSecondary = true
		if idx.Stats != nil {
			hasStats = true
			break
		}
	}
	return hasSecondary && !hasStats
}

func isPatternCoveredByExisting(candidate []mongoinspect.KeyField, indexes []mongoinspect.IndexInfo) bool {
	for _, idx := range indexes {
		if len(idx.Key) == 0 {
			continue
		}
		if isKeyPrefix(candidate, idx.Key) {
			return true
		}
	}
	return false
}

func bestCoverageIndex(candidate []mongoinspect.KeyField, indexes []mongoinspect.IndexInfo) (string, int) {
	bestIdx := ""
	bestPrefix := 0

	for _, idx := range indexes {
		if idx.Name == "_id_" || len(idx.Key) == 0 {
			continue
		}
		prefixLen := commonPrefixLen(idx.Key, candidate)
		if prefixLen > bestPrefix {
			bestPrefix = prefixLen
			bestIdx = idx.Name
			continue
		}
		if prefixLen == bestPrefix && prefixLen > 0 && (bestIdx == "" || idx.Name < bestIdx) {
			bestIdx = idx.Name
		}
	}

	return bestIdx, bestPrefix
}

func commonPrefixLen(a, b []mongoinspect.KeyField) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i].Field != b[i].Field || a[i].Direction != b[i].Direction {
			return i
		}
	}
	return n
}

func sameKeySetWithDirection(a, b []mongoinspect.KeyField) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[string]int, len(a))
	for _, key := range a {
		counts[keySignature([]mongoinspect.KeyField{key})]++
	}
	for _, key := range b {
		sig := keySignature([]mongoinspect.KeyField{key})
		if counts[sig] == 0 {
			return false
		}
		counts[sig]--
	}
	for _, v := range counts {
		if v != 0 {
			return false
		}
	}
	return true
}

func isReplaceableIndex(idx mongoinspect.IndexInfo) bool {
	if idx.Name == "_id_" || idx.Unique || idx.Sparse || idx.TTL != nil || len(idx.Key) == 0 {
		return false
	}
	for _, key := range idx.Key {
		if key.Direction != 1 && key.Direction != -1 {
			return false
		}
	}
	return true
}

func findIndexByName(name string, indexes []mongoinspect.IndexInfo) mongoinspect.IndexInfo {
	for _, idx := range indexes {
		if idx.Name == name {
			return idx
		}
	}
	return mongoinspect.IndexInfo{}
}

func keySignature(keys []mongoinspect.KeyField) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key.Field+":"+strconv.Itoa(key.Direction))
	}
	return strings.Join(parts, ",")
}

func formatIndexSpec(keys []mongoinspect.KeyField) string {
	return formatKeyFields(keys)
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
