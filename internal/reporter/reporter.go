package reporter

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/ppiankov/mongospectre/internal/analyzer"
)

// Format specifies the output format.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// Metadata holds context about how and when the report was generated.
type Metadata struct {
	Version        string `json:"version"`
	Command        string `json:"command"`
	Timestamp      string `json:"timestamp"`
	Database       string `json:"database,omitempty"`
	MongoDBVersion string `json:"mongodbVersion,omitempty"`
	RepoPath       string `json:"repoPath,omitempty"`
	URIHash        string `json:"uriHash,omitempty"`
}

// Report holds the structured audit output.
type Report struct {
	Metadata    Metadata           `json:"metadata"`
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
		Metadata: Metadata{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Findings:    findings,
		MaxSeverity: analyzer.MaxSeverity(findings),
		Summary:     s,
	}
}

// Write outputs the report in the given format.
func Write(w io.Writer, report *Report, format Format) error {
	switch format {
	case FormatJSON:
		return writeJSON(w, report)
	case FormatSARIF:
		return writeSARIF(w, report)
	case FormatSpectreHub:
		return writeSpectreHub(w, report)
	default:
		return writeText(w, report)
	}
}

// WriteBaselineDiff outputs a baseline comparison summary.
func WriteBaselineDiff(w io.Writer, diff []analyzer.BaselineFinding) {
	var newCount, resolvedCount, unchangedCount int
	for _, f := range diff {
		switch f.Status {
		case analyzer.StatusNew:
			newCount++
			_, _ = fmt.Fprintf(w, "+ [%s] %s: %s\n", f.Status, f.Type, f.Message)
		case analyzer.StatusResolved:
			resolvedCount++
			_, _ = fmt.Fprintf(w, "- [%s] %s: %s\n", f.Status, f.Type, f.Message)
		default:
			unchangedCount++
		}
	}
	_, _ = fmt.Fprintf(w, "\nBaseline diff: %d new, %d resolved, %d unchanged\n\n",
		newCount, resolvedCount, unchangedCount)
}

func writeJSON(w io.Writer, report *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func writeText(w io.Writer, report *Report) error {
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
		if _, err := fmt.Fprintf(w, "[%s] %s: %s (%s)\n", label, f.Type, f.Message, loc); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(w, "\nSummary: %d findings (high=%d medium=%d low=%d info=%d)\n",
		report.Summary.Total, report.Summary.High, report.Summary.Medium,
		report.Summary.Low, report.Summary.Info)
	return err
}
