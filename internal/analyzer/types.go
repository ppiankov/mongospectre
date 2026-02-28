package analyzer

// Severity indicates the risk level of a finding.
type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
	SeverityInfo   Severity = "info"
)

// FindingType identifies the category of audit finding.
type FindingType string

const (
	FindingUnusedCollection       FindingType = "UNUSED_COLLECTION"
	FindingUnusedIndex            FindingType = "UNUSED_INDEX"
	FindingMissingIndex           FindingType = "MISSING_INDEX"
	FindingDuplicateIndex         FindingType = "DUPLICATE_INDEX"
	FindingOversizedCollection    FindingType = "OVERSIZED_COLLECTION"
	FindingMissingTTL             FindingType = "MISSING_TTL"
	FindingUnshardedLarge         FindingType = "UNSHARDED_LARGE"
	FindingMonotonicShardKey      FindingType = "MONOTONIC_SHARD_KEY"
	FindingUnbalancedChunks       FindingType = "UNBALANCED_CHUNKS"
	FindingJumboChunks            FindingType = "JUMBO_CHUNKS"
	FindingBalancerDisabled       FindingType = "BALANCER_DISABLED"
	FindingMissingCollection      FindingType = "MISSING_COLLECTION"
	FindingOrphanedIndex          FindingType = "ORPHANED_INDEX"
	FindingUnindexedQuery         FindingType = "UNINDEXED_QUERY"
	FindingSuggestIndex           FindingType = "SUGGEST_INDEX"
	FindingCompoundIndexSuggest   FindingType = "COMPOUND_INDEX_SUGGESTION"
	FindingIndexOrderWarning      FindingType = "INDEX_ORDER_WARNING"
	FindingRedundantIndex         FindingType = "REDUNDANT_INDEX"
	FindingPartialCoverage        FindingType = "PARTIAL_COVERAGE"
	FindingSlowQuerySource        FindingType = "SLOW_QUERY_SOURCE"
	FindingCollectionScanSource   FindingType = "COLLECTION_SCAN_SOURCE"
	FindingFrequentSlowQuery      FindingType = "FREQUENT_SLOW_QUERY"
	FindingAdminInDataDB          FindingType = "ADMIN_IN_DATA_DB"
	FindingDuplicateUser          FindingType = "DUPLICATE_USER"
	FindingOverprivilegedUser     FindingType = "OVERPRIVILEGED_USER"
	FindingMultipleAdminUsers     FindingType = "MULTIPLE_ADMIN_USERS"
	FindingDynamicCollection      FindingType = "DYNAMIC_COLLECTION"
	FindingValidatorMissing       FindingType = "VALIDATOR_MISSING"
	FindingValidatorStale         FindingType = "VALIDATOR_STALE"
	FindingValidatorStrictRisk    FindingType = "VALIDATOR_STRICT_RISK"
	FindingValidatorWarnOnly      FindingType = "VALIDATOR_WARN_ONLY"
	FindingFieldNotInValidator    FindingType = "FIELD_NOT_IN_VALIDATOR"
	FindingAtlasIndexSuggestion   FindingType = "ATLAS_INDEX_SUGGESTION"
	FindingAtlasAlertActive       FindingType = "ATLAS_ALERT_ACTIVE"
	FindingAtlasTierMismatch      FindingType = "ATLAS_TIER_MISMATCH"
	FindingAtlasVersionBehind     FindingType = "ATLAS_VERSION_BEHIND"
	FindingAtlasUserNoScope       FindingType = "ATLAS_USER_NO_SCOPE"
	FindingInactiveUser           FindingType = "INACTIVE_USER"
	FindingFailedAuthOnly         FindingType = "FAILED_AUTH_ONLY"
	FindingInactivePrivilegedUser FindingType = "INACTIVE_PRIVILEGED_USER"
	FindingMissingField           FindingType = "MISSING_FIELD"
	FindingRareField              FindingType = "RARE_FIELD"
	FindingUndocumentedField      FindingType = "UNDOCUMENTED_FIELD"
	FindingTypeInconsistency      FindingType = "TYPE_INCONSISTENCY"
	FindingURINoAuth              FindingType = "URI_NO_AUTH"
	FindingURINoTLS               FindingType = "URI_NO_TLS"
	FindingURINoRetryWrites       FindingType = "URI_NO_RETRY_WRITES"
	FindingURIPlaintextPassword   FindingType = "URI_PLAINTEXT_PASSWORD"
	FindingURIDefaultAuthSource   FindingType = "URI_DEFAULT_AUTH_SOURCE"
	FindingURIShortTimeout        FindingType = "URI_SHORT_TIMEOUT"
	FindingURINoReadPreference    FindingType = "URI_NO_READ_PREFERENCE"
	FindingURIDirectConnection    FindingType = "URI_DIRECT_CONNECTION"
	FindingAuthDisabled           FindingType = "AUTH_DISABLED"
	FindingBindAllInterfaces      FindingType = "BIND_ALL_INTERFACES"
	FindingTLSDisabled            FindingType = "TLS_DISABLED"
	FindingTLSAllowInvalidCerts   FindingType = "TLS_ALLOW_INVALID_CERTS"
	FindingAuditLogDisabled       FindingType = "AUDIT_LOG_DISABLED"
	FindingLocalhostException     FindingType = "LOCALHOST_EXCEPTION_ACTIVE"
	FindingIndexBloat             FindingType = "INDEX_BLOAT"
	FindingWriteHeavyOverIndexed  FindingType = "WRITE_HEAVY_OVER_INDEXED"
	FindingSingleFieldRedundant   FindingType = "SINGLE_FIELD_REDUNDANT"
	FindingLargeIndex             FindingType = "LARGE_INDEX"
	FindingOK                     FindingType = "OK"
)

// Finding represents a single audit detection result.
type Finding struct {
	Type       FindingType `json:"type"`
	Severity   Severity    `json:"severity"`
	Database   string      `json:"database"`
	Collection string      `json:"collection"`
	Index      string      `json:"index,omitempty"`
	Message    string      `json:"message"`
}

// MaxSeverity returns the highest severity found in a list of findings.
// Returns SeverityInfo if the list is empty.
func MaxSeverity(findings []Finding) Severity {
	order := map[Severity]int{
		SeverityInfo:   0,
		SeverityLow:    1,
		SeverityMedium: 2,
		SeverityHigh:   3,
	}
	max := SeverityInfo
	for _, f := range findings {
		if order[f.Severity] > order[max] {
			max = f.Severity
		}
	}
	return max
}

// ExitCode maps severity to a process exit code.
func ExitCode(s Severity) int {
	switch s {
	case SeverityHigh:
		return 2
	case SeverityMedium:
		return 1
	default:
		return 0
	}
}
