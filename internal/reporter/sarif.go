package reporter

import (
	"encoding/json"
	"io"

	"github.com/ppiankov/mongospectre/internal/analyzer"
)

// FormatSARIF is the SARIF output format constant.
const FormatSARIF Format = "sarif"

// SARIF v2.1.0 types — minimal subset for GitHub Security integration.

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string                     `json:"name"`
	Version        string                     `json:"version,omitempty"`
	InformationURI string                     `json:"informationUri,omitempty"`
	Rules          []sarifReportingDescriptor `json:"rules,omitempty"`
}

type sarifReportingDescriptor struct {
	ID               string             `json:"id"`
	ShortDescription sarifMessage       `json:"shortDescription"`
	DefaultConfig    sarifDefaultConfig `json:"defaultConfiguration,omitempty"`
}

type sarifDefaultConfig struct {
	Level string `json:"level"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation *sarifPhysicalLocation `json:"physicalLocation,omitempty"`
	LogicalLocations []sarifLogicalLocation `json:"logicalLocations,omitempty"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifLogicalLocation struct {
	FullyQualifiedName string `json:"fullyQualifiedName"`
	Kind               string `json:"kind,omitempty"`
}

// sarifRules defines the SARIF rule descriptors for each finding type.
var sarifRules = map[analyzer.FindingType]sarifReportingDescriptor{
	analyzer.FindingUnusedCollection:     {ID: "UNUSED_COLLECTION", ShortDescription: sarifMessage{Text: "Collection exists but is unused"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingUnusedIndex:          {ID: "UNUSED_INDEX", ShortDescription: sarifMessage{Text: "Index has never been used"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingMissingIndex:         {ID: "MISSING_INDEX", ShortDescription: sarifMessage{Text: "Collection may need additional indexes"}, DefaultConfig: sarifDefaultConfig{Level: "error"}},
	analyzer.FindingDuplicateIndex:       {ID: "DUPLICATE_INDEX", ShortDescription: sarifMessage{Text: "Index is a prefix duplicate of another"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingOversizedCollection:  {ID: "OVERSIZED_COLLECTION", ShortDescription: sarifMessage{Text: "Collection has a very high document count"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingMissingTTL:           {ID: "MISSING_TTL", ShortDescription: sarifMessage{Text: "Timestamp collection without TTL index"}, DefaultConfig: sarifDefaultConfig{Level: "note"}},
	analyzer.FindingUnshardedLarge:       {ID: "UNSHARDED_LARGE", ShortDescription: sarifMessage{Text: "Large collection without sharding"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingMonotonicShardKey:    {ID: "MONOTONIC_SHARD_KEY", ShortDescription: sarifMessage{Text: "Shard key is monotonic and may create hot shards"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingUnbalancedChunks:     {ID: "UNBALANCED_CHUNKS", ShortDescription: sarifMessage{Text: "Chunk distribution is heavily skewed"}, DefaultConfig: sarifDefaultConfig{Level: "error"}},
	analyzer.FindingJumboChunks:          {ID: "JUMBO_CHUNKS", ShortDescription: sarifMessage{Text: "Jumbo chunks cannot be split automatically"}, DefaultConfig: sarifDefaultConfig{Level: "error"}},
	analyzer.FindingBalancerDisabled:     {ID: "BALANCER_DISABLED", ShortDescription: sarifMessage{Text: "Chunk balancer is disabled"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingMissingCollection:    {ID: "MISSING_COLLECTION", ShortDescription: sarifMessage{Text: "Collection referenced in code does not exist"}, DefaultConfig: sarifDefaultConfig{Level: "error"}},
	analyzer.FindingOrphanedIndex:        {ID: "ORPHANED_INDEX", ShortDescription: sarifMessage{Text: "Index on unreferenced collection with zero usage"}, DefaultConfig: sarifDefaultConfig{Level: "note"}},
	analyzer.FindingUnindexedQuery:       {ID: "UNINDEXED_QUERY", ShortDescription: sarifMessage{Text: "Queried field has no covering index"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingSuggestIndex:         {ID: "SUGGEST_INDEX", ShortDescription: sarifMessage{Text: "Consider adding an index for queried field"}, DefaultConfig: sarifDefaultConfig{Level: "none"}},
	analyzer.FindingSlowQuerySource:      {ID: "SLOW_QUERY_SOURCE", ShortDescription: sarifMessage{Text: "Code location correlates with slow profiler queries"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingCollectionScanSource: {ID: "COLLECTION_SCAN_SOURCE", ShortDescription: sarifMessage{Text: "Code location correlates with profiler COLLSCAN queries"}, DefaultConfig: sarifDefaultConfig{Level: "error"}},
	analyzer.FindingFrequentSlowQuery:    {ID: "FREQUENT_SLOW_QUERY", ShortDescription: sarifMessage{Text: "Repeated slow query shape in profiler"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingValidatorMissing:     {ID: "VALIDATOR_MISSING", ShortDescription: sarifMessage{Text: "Collection writes are not constrained by a validator"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingValidatorStale:       {ID: "VALIDATOR_STALE", ShortDescription: sarifMessage{Text: "Validator field type does not match code write type"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingValidatorStrictRisk:  {ID: "VALIDATOR_STRICT_RISK", ShortDescription: sarifMessage{Text: "Strict/error validator can reject writes on schema drift"}, DefaultConfig: sarifDefaultConfig{Level: "note"}},
	analyzer.FindingValidatorWarnOnly:    {ID: "VALIDATOR_WARN_ONLY", ShortDescription: sarifMessage{Text: "Validator is warn-only and does not reject invalid writes"}, DefaultConfig: sarifDefaultConfig{Level: "none"}},
	analyzer.FindingFieldNotInValidator:  {ID: "FIELD_NOT_IN_VALIDATOR", ShortDescription: sarifMessage{Text: "Code writes a field missing from validator properties"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingAtlasIndexSuggestion: {ID: "ATLAS_INDEX_SUGGESTION", ShortDescription: sarifMessage{Text: "Atlas Performance Advisor recommends an index"}, DefaultConfig: sarifDefaultConfig{Level: "note"}},
	analyzer.FindingAtlasAlertActive:     {ID: "ATLAS_ALERT_ACTIVE", ShortDescription: sarifMessage{Text: "Atlas project has active alerts"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingAtlasTierMismatch:    {ID: "ATLAS_TIER_MISMATCH", ShortDescription: sarifMessage{Text: "Atlas cluster tier may be undersized for current data footprint"}, DefaultConfig: sarifDefaultConfig{Level: "error"}},
	analyzer.FindingAtlasVersionBehind:   {ID: "ATLAS_VERSION_BEHIND", ShortDescription: sarifMessage{Text: "Atlas cluster version is behind available project LTS versions"}, DefaultConfig: sarifDefaultConfig{Level: "warning"}},
	analyzer.FindingOK:                   {ID: "OK", ShortDescription: sarifMessage{Text: "Collection exists and is referenced"}, DefaultConfig: sarifDefaultConfig{Level: "none"}},
}

func writeSARIF(w io.Writer, report *Report) error {
	// Collect unique rules used in findings.
	usedRules := make(map[analyzer.FindingType]bool)
	for _, f := range report.Findings {
		usedRules[f.Type] = true
	}
	var rules []sarifReportingDescriptor
	for ft := range usedRules {
		if r, ok := sarifRules[ft]; ok {
			rules = append(rules, r)
		}
	}

	// Build results.
	var results []sarifResult
	for _, f := range report.Findings {
		r := sarifResult{
			RuleID:  string(f.Type),
			Level:   severityToSARIFLevel(f.Severity),
			Message: sarifMessage{Text: f.Message},
		}

		// Add logical location (database.collection).
		loc := sarifLocation{
			LogicalLocations: []sarifLogicalLocation{{
				FullyQualifiedName: logicalName(&f),
				Kind:               "object",
			}},
		}
		r.Locations = []sarifLocation{loc}
		results = append(results, r)
	}

	log := sarifLog{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           "mongospectre",
					Version:        report.Metadata.Version,
					InformationURI: "https://github.com/ppiankov/mongospectre",
					Rules:          rules,
				},
			},
			Results: results,
		}},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

func severityToSARIFLevel(s analyzer.Severity) string {
	switch s {
	case analyzer.SeverityHigh:
		return "error"
	case analyzer.SeverityMedium:
		return "warning"
	case analyzer.SeverityLow:
		return "note"
	default:
		return "none"
	}
}

func logicalName(f *analyzer.Finding) string {
	name := f.Database + "." + f.Collection
	if f.Index != "" {
		name += "." + f.Index
	}
	return name
}
