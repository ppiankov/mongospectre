package reporter

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ppiankov/mongospectre/internal/analyzer"
)

// Format specifies the output format.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// Report holds the structured audit output.
type Report struct {
	Findings    []analyzer.Finding `json:"findings"`
	MaxSeverity analyzer.Severity  `json:"maxSeverity"`
	Summary     Summary            `json:"summary"`
}

// Summary counts findings by severity.
type Summary struct {
	Total  int `json:"total"`
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
	Info   int `json:"info"`
}

// NewReport builds a report from findings.
func NewReport(findings []analyzer.Finding) Report {
	var s Summary
	for _, f := range findings {
		s.Total++
		switch f.Severity {
		case analyzer.SeverityHigh:
			s.High++
		case analyzer.SeverityMedium:
			s.Medium++
		case analyzer.SeverityLow:
			s.Low++
		case analyzer.SeverityInfo:
			s.Info++
		}
	}
	return Report{
		Findings:    findings,
		MaxSeverity: analyzer.MaxSeverity(findings),
		Summary:     s,
	}
}

// Write outputs the report in the given format.
func Write(w io.Writer, report Report, format Format) error {
	switch format {
	case FormatJSON:
		return writeJSON(w, report)
	default:
		return writeText(w, report)
	}
}

func writeJSON(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func writeText(w io.Writer, report Report) error {
	if report.Summary.Total == 0 {
		_, err := fmt.Fprintln(w, "No findings.")
		return err
	}

	severityLabel := map[analyzer.Severity]string{
		analyzer.SeverityHigh:   "HIGH",
		analyzer.SeverityMedium: "MEDIUM",
		analyzer.SeverityLow:    "LOW",
		analyzer.SeverityInfo:   "INFO",
	}

	for _, f := range report.Findings {
		label := severityLabel[f.Severity]
		loc := f.Database + "." + f.Collection
		if f.Index != "" {
			loc += "." + f.Index
		}
		fmt.Fprintf(w, "[%s] %s: %s (%s)\n", label, f.Type, f.Message, loc)
	}

	fmt.Fprintf(w, "\nSummary: %d findings (high=%d medium=%d low=%d info=%d)\n",
		report.Summary.Total, report.Summary.High, report.Summary.Medium,
		report.Summary.Low, report.Summary.Info)
	return nil
}
