package analyzer

import (
	"fmt"
	"time"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

const (
	rapidGrowthPct            = 50             // percentage growth threshold
	rapidGrowthAbsolute int64 = 1 << 30        // 1 GB absolute growth threshold
	approachingLimitGB  int64 = 12 * (1 << 30) // 12 GB
)

// DetectGrowth compares current collection stats against baseline stats and
// returns findings for rapid growth, index bloat, approaching limits, and
// storage reclaim opportunities. Returns nil if baseline is empty.
func DetectGrowth(current, baseline []mongoinspect.CollectionInfo, elapsed time.Duration) []Finding {
	if len(baseline) == 0 {
		return nil
	}

	baselineMap := make(map[string]*mongoinspect.CollectionInfo, len(baseline))
	for i := range baseline {
		key := baseline[i].Database + "." + baseline[i].Name
		baselineMap[key] = &baseline[i]
	}

	elapsedStr := formatElapsed(elapsed)

	var findings []Finding
	for i := range current {
		c := &current[i]
		key := c.Database + "." + c.Name
		b := baselineMap[key]

		findings = append(findings, detectRapidGrowth(c, b, elapsedStr)...)
		findings = append(findings, detectIndexGrowthOutpacingData(c, b, elapsedStr)...)
		findings = append(findings, detectApproachingLimit(c)...)
		findings = append(findings, detectStorageReclaim(c)...)
	}
	return findings
}

func detectRapidGrowth(current *mongoinspect.CollectionInfo, baseline *mongoinspect.CollectionInfo, elapsed string) []Finding {
	if baseline == nil || baseline.Size <= 0 {
		return nil
	}
	growth := current.Size - baseline.Size
	if growth <= 0 {
		return nil
	}
	pct := float64(growth) * 100 / float64(baseline.Size)
	if pct < float64(rapidGrowthPct) && growth < rapidGrowthAbsolute {
		return nil
	}
	return []Finding{{
		Type:       FindingRapidGrowth,
		Severity:   SeverityMedium,
		Database:   current.Database,
		Collection: current.Name,
		Message:    fmt.Sprintf("data size grew %.0f%% (%s → %s) in %s", pct, formatBytes(baseline.Size), formatBytes(current.Size), elapsed),
	}}
}

func detectIndexGrowthOutpacingData(current *mongoinspect.CollectionInfo, baseline *mongoinspect.CollectionInfo, elapsed string) []Finding {
	if baseline == nil {
		return nil
	}
	// Both data and index must have grown from a positive baseline.
	if baseline.Size <= 0 || baseline.TotalIndexSize <= 0 {
		return nil
	}
	dataGrowth := current.Size - baseline.Size
	indexGrowth := current.TotalIndexSize - baseline.TotalIndexSize
	if dataGrowth <= 0 || indexGrowth <= 0 {
		return nil
	}
	dataPct := float64(dataGrowth) * 100 / float64(baseline.Size)
	indexPct := float64(indexGrowth) * 100 / float64(baseline.TotalIndexSize)
	if indexPct <= dataPct {
		return nil
	}
	return []Finding{{
		Type:       FindingIndexGrowthOutpacing,
		Severity:   SeverityLow,
		Database:   current.Database,
		Collection: current.Name,
		Message:    fmt.Sprintf("index size grew %.0f%% while data grew %.0f%% in %s — possible compound index proliferation", indexPct, dataPct, elapsed),
	}}
}

func detectApproachingLimit(current *mongoinspect.CollectionInfo) []Finding {
	if current.Size < approachingLimitGB {
		return nil
	}
	return []Finding{{
		Type:       FindingApproachingLimit,
		Severity:   SeverityMedium,
		Database:   current.Database,
		Collection: current.Name,
		Message:    fmt.Sprintf("collection is %s — approaching practical single-node limits, consider sharding", formatBytes(current.Size)),
	}}
}

func detectStorageReclaim(current *mongoinspect.CollectionInfo) []Finding {
	if current.Size <= 0 || current.StorageSize <= 2*current.Size {
		return nil
	}
	return []Finding{{
		Type:       FindingStorageReclaim,
		Severity:   SeverityLow,
		Database:   current.Database,
		Collection: current.Name,
		Message:    fmt.Sprintf("storage size (%s) is >2× data size (%s) — run compact to reclaim space", formatBytes(current.StorageSize), formatBytes(current.Size)),
	}}
}

func formatElapsed(d time.Duration) string {
	days := int(d.Hours() / 24)
	switch {
	case days >= 1:
		return fmt.Sprintf("%d days", days)
	case d.Hours() >= 1:
		return fmt.Sprintf("%.0f hours", d.Hours())
	default:
		return fmt.Sprintf("%.0f minutes", d.Minutes())
	}
}
