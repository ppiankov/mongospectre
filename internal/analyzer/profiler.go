package analyzer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

const frequentSlowQueryThreshold = 50

type sourceLocation struct {
	file   string
	line   int
	fields map[string]bool
}

func (l sourceLocation) key() string {
	return l.file + ":" + strconv.Itoa(l.line)
}

type profileLocationStats struct {
	database      string
	collection    string
	file          string
	line          int
	count         int
	totalMillis   int64
	collscanCount int
}

type profileShapeStats struct {
	database         string
	collection       string
	filterFields     []string
	sortFields       []string
	projectionFields []string
	count            int
	locations        map[string]sourceLocation
}

// CorrelateProfiler matches profiler query shapes to scanned code locations.
func CorrelateProfiler(scan *scanner.ScanResult, entries []mongoinspect.ProfileEntry) []Finding {
	if scan == nil || len(entries) == 0 {
		return nil
	}

	locationsByCollection := buildSourceLocations(scan)
	if len(locationsByCollection) == 0 {
		return nil
	}

	locationStats := make(map[string]*profileLocationStats)
	shapeStats := make(map[string]*profileShapeStats)

	for i := range entries {
		entry := &entries[i]
		collectionKey := normalizeProfileField(entry.Collection)
		if collectionKey == "" {
			continue
		}
		locations := locationsByCollection[collectionKey]
		if len(locations) == 0 {
			continue
		}

		shapeFields := profileEntryFieldSet(entry)
		matchedLocations := matchSourceLocations(locations, shapeFields)
		if len(matchedLocations) == 0 {
			continue
		}

		dbKey := normalizeProfileField(entry.Database)
		shapeKey := profileShapeKey(dbKey, collectionKey, entry)
		shape := shapeStats[shapeKey]
		if shape == nil {
			shape = &profileShapeStats{
				database:         entry.Database,
				collection:       entry.Collection,
				filterFields:     normalizeFieldList(entry.FilterFields),
				sortFields:       normalizeFieldList(entry.SortFields),
				projectionFields: normalizeFieldList(entry.ProjectionFields),
				locations:        make(map[string]sourceLocation),
			}
			shapeStats[shapeKey] = shape
		}
		shape.count++

		for _, loc := range matchedLocations {
			shape.locations[loc.key()] = loc

			locationKey := dbKey + "|" + collectionKey + "|" + loc.key()
			stat := locationStats[locationKey]
			if stat == nil {
				stat = &profileLocationStats{
					database:   entry.Database,
					collection: entry.Collection,
					file:       loc.file,
					line:       loc.line,
				}
				locationStats[locationKey] = stat
			}
			stat.count++
			stat.totalMillis += entry.DurationMillis
			if isCollectionScan(entry.PlanSummary) {
				stat.collscanCount++
			}
		}
	}

	var findings []Finding
	locationKeys := make([]string, 0, len(locationStats))
	for key := range locationStats {
		locationKeys = append(locationKeys, key)
	}
	sort.Strings(locationKeys)

	for _, key := range locationKeys {
		stat := locationStats[key]
		avgMillis := int64(0)
		if stat.count > 0 {
			avgMillis = stat.totalMillis / int64(stat.count)
		}

		findings = append(findings, Finding{
			Type:       FindingSlowQuerySource,
			Severity:   SeverityMedium,
			Database:   stat.database,
			Collection: stat.collection,
			Message: fmt.Sprintf(
				"code at %s:%d matches slow profiler queries (avg %dms across %d samples)",
				stat.file,
				stat.line,
				avgMillis,
				stat.count,
			),
		})

		if stat.collscanCount > 0 {
			findings = append(findings, Finding{
				Type:       FindingCollectionScanSource,
				Severity:   SeverityHigh,
				Database:   stat.database,
				Collection: stat.collection,
				Message: fmt.Sprintf(
					"code at %s:%d matches COLLSCAN profiler queries (%d samples)",
					stat.file,
					stat.line,
					stat.collscanCount,
				),
			})
		}
	}

	shapeKeys := make([]string, 0, len(shapeStats))
	for key := range shapeStats {
		shapeKeys = append(shapeKeys, key)
	}
	sort.Strings(shapeKeys)

	for _, key := range shapeKeys {
		shape := shapeStats[key]
		if shape.count < frequentSlowQueryThreshold {
			continue
		}
		findings = append(findings, Finding{
			Type:       FindingFrequentSlowQuery,
			Severity:   SeverityMedium,
			Database:   shape.database,
			Collection: shape.collection,
			Message: fmt.Sprintf(
				"query shape (%s) appears %d times in profiler; source: %s",
				formatShapeSummary(shape),
				shape.count,
				formatShapeSources(shape.locations),
			),
		})
	}

	return findings
}

func buildSourceLocations(scan *scanner.ScanResult) map[string][]sourceLocation {
	byCollection := make(map[string]map[string]*sourceLocation)

	upsert := func(collection, file string, line int) *sourceLocation {
		collKey := normalizeProfileField(collection)
		if collKey == "" {
			return nil
		}
		if byCollection[collKey] == nil {
			byCollection[collKey] = make(map[string]*sourceLocation)
		}
		key := file + ":" + strconv.Itoa(line)
		loc := byCollection[collKey][key]
		if loc == nil {
			loc = &sourceLocation{
				file:   file,
				line:   line,
				fields: make(map[string]bool),
			}
			byCollection[collKey][key] = loc
		}
		return loc
	}

	for _, ref := range scan.Refs {
		_ = upsert(ref.Collection, ref.File, ref.Line)
	}

	for _, ref := range scan.FieldRefs {
		loc := upsert(ref.Collection, ref.File, ref.Line)
		if loc == nil {
			continue
		}
		field := normalizeProfileField(ref.Field)
		if field != "" {
			loc.fields[field] = true
		}
	}

	out := make(map[string][]sourceLocation, len(byCollection))
	for collection, locations := range byCollection {
		flat := make([]sourceLocation, 0, len(locations))
		for _, loc := range locations {
			flat = append(flat, *loc)
		}
		sort.Slice(flat, func(i, j int) bool {
			if flat[i].file == flat[j].file {
				return flat[i].line < flat[j].line
			}
			return flat[i].file < flat[j].file
		})
		out[collection] = flat
	}
	return out
}

func matchSourceLocations(locations []sourceLocation, shapeFields map[string]bool) []sourceLocation {
	if len(locations) == 0 {
		return nil
	}
	if len(shapeFields) == 0 {
		return []sourceLocation{locations[0]}
	}

	bestScore := 0
	var matched []sourceLocation
	for _, loc := range locations {
		score := overlapScore(shapeFields, loc.fields)
		if score == 0 {
			continue
		}
		if score > bestScore {
			bestScore = score
			matched = []sourceLocation{loc}
			continue
		}
		if score == bestScore {
			matched = append(matched, loc)
		}
	}

	if len(matched) > 0 {
		return matched
	}

	// Fallback to collection-level match when field extraction is incomplete.
	return []sourceLocation{locations[0]}
}

func overlapScore(shapeFields map[string]bool, sourceFields map[string]bool) int {
	if len(shapeFields) == 0 || len(sourceFields) == 0 {
		return 0
	}

	score := 0
	for shapeField := range shapeFields {
		if sourceFields[shapeField] {
			score++
			continue
		}
		for sourceField := range sourceFields {
			if strings.HasPrefix(shapeField, sourceField+".") || strings.HasPrefix(sourceField, shapeField+".") {
				score++
				break
			}
		}
	}
	return score
}

func profileEntryFieldSet(entry *mongoinspect.ProfileEntry) map[string]bool {
	set := make(map[string]bool)
	for _, field := range entry.FilterFields {
		normalized := normalizeProfileField(field)
		if normalized != "" {
			set[normalized] = true
		}
	}
	for _, field := range entry.SortFields {
		normalized := normalizeProfileField(field)
		if normalized != "" {
			set[normalized] = true
		}
	}
	for _, field := range entry.ProjectionFields {
		normalized := normalizeProfileField(field)
		if normalized != "" {
			set[normalized] = true
		}
	}
	return set
}

func profileShapeKey(database, collection string, entry *mongoinspect.ProfileEntry) string {
	return strings.Join([]string{
		database,
		collection,
		strings.Join(normalizeFieldList(entry.FilterFields), ","),
		strings.Join(normalizeFieldList(entry.SortFields), ","),
		strings.Join(normalizeFieldList(entry.ProjectionFields), ","),
	}, "|")
}

func normalizeFieldList(fields []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, field := range fields {
		normalized := normalizeProfileField(field)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func normalizeProfileField(field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	field = strings.TrimPrefix(field, "$")
	return field
}

func isCollectionScan(planSummary string) bool {
	return strings.Contains(strings.ToUpper(planSummary), "COLLSCAN")
}

func formatShapeSummary(shape *profileShapeStats) string {
	var parts []string
	if len(shape.filterFields) > 0 {
		parts = append(parts, "filter="+strings.Join(shape.filterFields, ","))
	}
	if len(shape.sortFields) > 0 {
		parts = append(parts, "sort="+strings.Join(shape.sortFields, ","))
	}
	if len(shape.projectionFields) > 0 {
		parts = append(parts, "projection="+strings.Join(shape.projectionFields, ","))
	}
	if len(parts) == 0 {
		return "no filter/sort/projection fields"
	}
	return strings.Join(parts, " ")
}

func formatShapeSources(locations map[string]sourceLocation) string {
	sources := make([]string, 0, len(locations))
	for _, loc := range locations {
		sources = append(sources, loc.key())
	}
	sort.Strings(sources)

	const maxShown = 3
	if len(sources) <= maxShown {
		return strings.Join(sources, ", ")
	}
	return strings.Join(sources[:maxShown], ", ") + fmt.Sprintf(" (+%d more)", len(sources)-maxShown)
}
