package analyzer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ppiankov/mongospectre/internal/atlas"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

// AtlasAuditInput carries Atlas API data and optional code scan context.
type AtlasAuditInput struct {
	ProjectID         string
	Cluster           atlas.Cluster
	SuggestedIndexes  []atlas.SuggestedIndex
	Alerts            []atlas.Alert
	AvailableVersions []string
	Collections       []mongoinspect.CollectionInfo
	Scan              *scanner.ScanResult
}

// AuditAtlas produces Atlas-only findings and correlation context for audit reports.
func AuditAtlas(input *AtlasAuditInput) []Finding {
	if input == nil {
		return nil
	}

	var findings []Finding
	findings = append(findings, detectAtlasIndexSuggestions(input.SuggestedIndexes, input.Scan)...)
	findings = append(findings, detectAtlasAlerts(input.Alerts, input.ProjectID, input.Cluster.Name)...)
	findings = append(findings, detectAtlasTierMismatch(input.Cluster, input.Collections)...)
	findings = append(findings, detectAtlasVersionBehind(input.Cluster, input.AvailableVersions)...)
	return findings
}

func detectAtlasIndexSuggestions(suggestions []atlas.SuggestedIndex, scan *scanner.ScanResult) []Finding {
	if len(suggestions) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var findings []Finding
	for _, suggestion := range suggestions {
		namespace := strings.TrimSpace(suggestion.Namespace)
		if namespace == "" || len(suggestion.IndexFields) == 0 {
			continue
		}

		nsDB, nsColl := splitNamespace(namespace)
		if nsColl == "" {
			nsColl = namespace
		}
		if nsDB == "" {
			nsDB = "atlas"
		}

		fields := normalizeFields(suggestion.IndexFields)
		if len(fields) == 0 {
			continue
		}

		dedupeKey := strings.ToLower(namespace + "|" + strings.Join(fields, ","))
		if seen[dedupeKey] {
			continue
		}
		seen[dedupeKey] = true

		matches, collectionSeen := correlateIndexSuggestion(scan, nsColl, fields)
		severity := SeverityInfo
		if len(matches) > 0 {
			severity = SeverityLow
		}

		message := fmt.Sprintf("Atlas Performance Advisor suggests index %s", renderIndexSpec(fields))
		switch {
		case len(matches) > 0:
			message += fmt.Sprintf("; matching queried code fields: %s", strings.Join(matches, ", "))
		case scan != nil && collectionSeen:
			message += "; collection is referenced in code, but suggested fields were not detected in query filters"
		case scan != nil:
			message += "; collection not detected in scanned code patterns"
		}

		findings = append(findings, Finding{
			Type:       FindingAtlasIndexSuggestion,
			Severity:   severity,
			Database:   nsDB,
			Collection: nsColl,
			Message:    message,
		})
	}

	return findings
}

func detectAtlasAlerts(alerts []atlas.Alert, projectID, clusterName string) []Finding {
	if len(alerts) == 0 {
		return nil
	}

	if clusterName == "" {
		clusterName = projectID
	}
	if clusterName == "" {
		clusterName = "atlas"
	}

	var findings []Finding
	for _, alert := range alerts {
		if !isActiveAtlasAlert(alert.Status) {
			continue
		}
		event := strings.TrimSpace(alert.EventTypeName)
		if event == "" {
			event = "UNKNOWN"
		}
		status := strings.TrimSpace(alert.Status)
		if status == "" {
			status = "OPEN"
		}
		findings = append(findings, Finding{
			Type:       FindingAtlasAlertActive,
			Severity:   atlasAlertSeverity(event),
			Database:   "atlas",
			Collection: clusterName,
			Message:    fmt.Sprintf("active Atlas alert %q (status %s)", event, strings.ToUpper(status)),
		})
	}
	return findings
}

func detectAtlasTierMismatch(cluster atlas.Cluster, collections []mongoinspect.CollectionInfo) []Finding {
	tier := atlasTier(cluster.InstanceSizeName)
	if tier <= 0 || tier > 10 {
		return nil
	}

	var totalStorage int64
	for _, c := range collections {
		totalStorage += c.StorageSize
	}

	const gb = 1024 * 1024 * 1024
	if totalStorage < 500*gb {
		return nil
	}

	clusterName := strings.TrimSpace(cluster.Name)
	if clusterName == "" {
		clusterName = "atlas"
	}

	totalGB := float64(totalStorage) / float64(gb)
	return []Finding{{
		Type:       FindingAtlasTierMismatch,
		Severity:   SeverityHigh,
		Database:   "atlas",
		Collection: clusterName,
		Message:    fmt.Sprintf("cluster tier %s may be undersized for %.1f GB storage", cluster.InstanceSizeName, totalGB),
	}}
}

func detectAtlasVersionBehind(cluster atlas.Cluster, available []string) []Finding {
	current := normalizeVersion(cluster.MongoDBVersion)
	if current == "" || len(available) == 0 {
		return nil
	}

	latest := ""
	for _, v := range available {
		norm := normalizeVersion(v)
		if norm == "" {
			continue
		}
		if latest == "" || compareVersion(norm, latest) > 0 {
			latest = norm
		}
	}
	if latest == "" || compareVersion(current, latest) >= 0 {
		return nil
	}

	clusterName := strings.TrimSpace(cluster.Name)
	if clusterName == "" {
		clusterName = "atlas"
	}

	return []Finding{{
		Type:       FindingAtlasVersionBehind,
		Severity:   SeverityMedium,
		Database:   "atlas",
		Collection: clusterName,
		Message:    fmt.Sprintf("cluster MongoDB version %s is behind available version %s", current, latest),
	}}
}

func correlateIndexSuggestion(scan *scanner.ScanResult, collection string, suggestedFields []string) ([]string, bool) {
	if scan == nil {
		return nil, false
	}

	collection = strings.ToLower(strings.TrimSpace(collection))
	if collection == "" {
		return nil, false
	}

	collectionSeen := false
	for _, c := range scan.Collections {
		if strings.EqualFold(c, collection) {
			collectionSeen = true
			break
		}
	}

	queriedFields := make(map[string]string)
	for _, fr := range scan.FieldRefs {
		if !strings.EqualFold(fr.Collection, collection) {
			continue
		}
		collectionSeen = true
		lower := strings.ToLower(fr.Field)
		if _, ok := queriedFields[lower]; !ok {
			queriedFields[lower] = fr.Field
		}
	}

	matches := make([]string, 0, len(suggestedFields))
	seenMatch := make(map[string]bool)
	for _, field := range suggestedFields {
		lower := strings.ToLower(field)
		matched := queriedFields[lower]
		if matched == "" || seenMatch[matched] {
			continue
		}
		seenMatch[matched] = true
		matches = append(matches, matched)
	}
	sort.Strings(matches)
	return matches, collectionSeen
}

func normalizeFields(fields []string) []string {
	out := make([]string, 0, len(fields))
	seen := make(map[string]bool)
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

func renderIndexSpec(fields []string) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, fmt.Sprintf("%s: 1", field))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func splitNamespace(namespace string) (string, string) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return "", ""
	}
	parts := strings.SplitN(namespace, ".", 2)
	if len(parts) != 2 {
		return "", namespace
	}
	return parts[0], parts[1]
}

func isActiveAtlasAlert(status string) bool {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case "", "OPEN", "TRACKING", "CREATED":
		return true
	case "CLOSED", "RESOLVED", "CANCELED", "CANCELLED", "INACTIVE":
		return false
	default:
		return true
	}
}

func atlasAlertSeverity(event string) Severity {
	event = strings.ToUpper(event)
	switch {
	case strings.Contains(event, "OUTSIDE_METRIC_THRESHOLD"):
		return SeverityHigh
	case strings.Contains(event, "NO_PRIMARY"):
		return SeverityHigh
	case strings.Contains(event, "HOST") && strings.Contains(event, "DOWN"):
		return SeverityHigh
	default:
		return SeverityMedium
	}
}

func atlasTier(instanceSize string) int {
	value := strings.ToUpper(strings.TrimSpace(instanceSize))
	if !strings.HasPrefix(value, "M") {
		return 0
	}
	n := strings.TrimPrefix(value, "M")
	for i, r := range n {
		if r < '0' || r > '9' {
			n = n[:i]
			break
		}
	}
	if n == "" {
		return 0
	}
	tier, err := strconv.Atoi(n)
	if err != nil {
		return 0
	}
	return tier
}

func normalizeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(strings.ToLower(raw), "v")
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ".")
	nums := make([]string, 0, 3)
	for _, p := range parts {
		var digits []rune
		for _, r := range p {
			if r < '0' || r > '9' {
				break
			}
			digits = append(digits, r)
		}
		if len(digits) == 0 {
			break
		}
		nums = append(nums, string(digits))
		if len(nums) == 3 {
			break
		}
	}
	if len(nums) < 2 {
		return ""
	}
	for len(nums) < 3 {
		nums = append(nums, "0")
	}
	return strings.Join(nums, ".")
}

func compareVersion(a, b string) int {
	aParts := strings.Split(normalizeVersion(a), ".")
	bParts := strings.Split(normalizeVersion(b), ".")
	for len(aParts) < 3 {
		aParts = append(aParts, "0")
	}
	for len(bParts) < 3 {
		bParts = append(bParts, "0")
	}

	for i := 0; i < 3; i++ {
		ai, _ := strconv.Atoi(aParts[i])
		bi, _ := strconv.Atoi(bParts[i])
		switch {
		case ai < bi:
			return -1
		case ai > bi:
			return 1
		}
	}
	return 0
}
