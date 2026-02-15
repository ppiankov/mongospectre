package reporter

import (
	"fmt"
	"io"
	"testing"

	"github.com/ppiankov/mongospectre/internal/analyzer"
)

func makeFindings(n int) []analyzer.Finding {
	severities := []analyzer.Severity{
		analyzer.SeverityHigh, analyzer.SeverityMedium,
		analyzer.SeverityLow, analyzer.SeverityInfo,
	}
	types := []analyzer.FindingType{
		analyzer.FindingUnusedIndex, analyzer.FindingMissingIndex,
		analyzer.FindingDuplicateIndex, analyzer.FindingUnusedCollection,
	}

	findings := make([]analyzer.Finding, n)
	for i := range findings {
		findings[i] = analyzer.Finding{
			Type:       types[i%len(types)],
			Severity:   severities[i%len(severities)],
			Database:   "testdb",
			Collection: fmt.Sprintf("coll_%d", i),
			Index:      fmt.Sprintf("idx_%d", i),
			Message:    fmt.Sprintf("finding %d description", i),
		}
	}
	return findings
}

func BenchmarkWriteJSON_500(b *testing.B) {
	report := NewReport(makeFindings(500))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Write(io.Discard, &report, FormatJSON)
	}
}

func BenchmarkWriteText_500(b *testing.B) {
	report := NewReport(makeFindings(500))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Write(io.Discard, &report, FormatText)
	}
}

func BenchmarkWriteSARIF_500(b *testing.B) {
	report := NewReport(makeFindings(500))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Write(io.Discard, &report, FormatSARIF)
	}
}

func BenchmarkWriteSpectreHub_500(b *testing.B) {
	report := NewReport(makeFindings(500))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Write(io.Discard, &report, FormatSpectreHub)
	}
}
