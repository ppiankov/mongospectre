package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ppiankov/mongospectre/internal/analyzer"
	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
	"github.com/ppiankov/mongospectre/internal/reporter"
	"github.com/ppiankov/mongospectre/internal/scanner"
)

const (
	defaultWidth         = 120
	defaultHeight        = 32
	maxDetailCodeRefRows = 25
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("81"))
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208"))
	fileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81")).
			Bold(true)
	lineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
	kindStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("248"))
)

// Input contains all data needed to render the interactive UI.
type Input struct {
	Report      reporter.Report
	Collections []mongoinspect.CollectionInfo
	Scan        *scanner.ScanResult
}

// Run launches the interactive findings explorer.
func Run(input *Input) error {
	m := newModel(input)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

type sortMode int

const (
	sortBySeverity sortMode = iota
	sortByType
	sortByCollection
)

func (s sortMode) String() string {
	switch s {
	case sortByType:
		return "type"
	case sortByCollection:
		return "collection"
	default:
		return "severity"
	}
}

func (s sortMode) next() sortMode {
	switch s {
	case sortBySeverity:
		return sortByType
	case sortByType:
		return sortByCollection
	default:
		return sortBySeverity
	}
}

type findingEntry struct {
	id      int
	finding analyzer.Finding
}

type codeRef struct {
	File   string
	Line   int
	Kind   string
	Detail string
}

type severitySummary struct {
	total  int
	high   int
	medium int
	low    int
	info   int
}

type model struct {
	metadata reporter.Metadata

	entries  []findingEntry
	filtered []findingEntry

	collections map[string]mongoinspect.CollectionInfo
	codeRefs    map[string][]codeRef
	dynamicRefs []codeRef

	sortMode sortMode

	table  table.Model
	filter textinput.Model
	detail viewport.Model

	filtering  bool
	detailMode bool

	status string
	width  int
	height int
}

func newModel(input *Input) *model {
	entries := make([]findingEntry, len(input.Report.Findings))
	for i := range input.Report.Findings {
		entries[i] = findingEntry{
			id:      i,
			finding: input.Report.Findings[i],
		}
	}

	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "SEV", Width: 8},
			{Title: "TYPE", Width: 24},
			{Title: "COLLECTION", Width: 28},
			{Title: "MESSAGE", Width: 56},
		}),
		table.WithRows(nil),
		table.WithHeight(16),
		table.WithFocused(true),
	)
	tableStyles := table.DefaultStyles()
	tableStyles.Header = tableStyles.Header.Bold(true)
	tableStyles.Selected = tableStyles.Selected.
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("62")).
		Bold(true)
	t.SetStyles(tableStyles)

	filterInput := textinput.New()
	filterInput.Prompt = ""
	filterInput.Placeholder = "collection/type/severity"
	filterInput.CharLimit = 128
	filterInput.Width = 64

	vp := viewport.New(100, 18)

	codeRefs, dynamicRefs := buildCodeRefs(input.Scan)

	m := &model{
		metadata:    input.Report.Metadata,
		entries:     entries,
		collections: buildCollectionLookup(input.Collections),
		codeRefs:    codeRefs,
		dynamicRefs: dynamicRefs,
		sortMode:    sortBySeverity,
		table:       t,
		filter:      filterInput,
		detail:      vp,
		status:      "Use j/k or arrows to navigate. Enter details, / filter, s sort, e export, q quit.",
		width:       defaultWidth,
		height:      defaultHeight,
	}

	m.refreshRows()
	m.resizeLayout()
	return m
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.resizeLayout()
		return m, nil
	case tea.KeyMsg:
		if m.detailMode {
			return m.updateDetailKey(typed)
		}
		return m.updateListKey(typed)
	default:
		if m.detailMode {
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
}

func (m *model) updateListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	}

	if m.filtering {
		switch msg.String() {
		case "enter", "esc":
			m.filtering = false
			m.filter.Blur()
			m.status = fmt.Sprintf("Filter applied (%d findings)", len(m.filtered))
			return m, nil
		}
		prev := m.filter.Value()
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		if prev != m.filter.Value() {
			m.refreshRows()
		}
		return m, cmd
	}

	switch msg.String() {
	case "/":
		m.filtering = true
		m.filter.Focus()
		m.status = "Filter mode: type to narrow findings, then Enter/Esc."
		return m, nil
	case "s":
		m.sortMode = m.sortMode.next()
		m.refreshRows()
		m.status = fmt.Sprintf("Sorted by %s", m.sortMode.String())
		return m, nil
	case "e":
		path, err := m.exportFiltered()
		if err != nil {
			m.status = fmt.Sprintf("export failed: %v", err)
		} else {
			m.status = fmt.Sprintf("Exported %d findings to %s", len(m.filtered), path)
		}
		return m, nil
	case "enter":
		if _, ok := m.selectedEntry(); !ok {
			return m, nil
		}
		m.detailMode = true
		m.setDetailContent()
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *model) updateDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc", "b", "enter":
		m.detailMode = false
		m.status = "Back to findings list"
		return m, nil
	}
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

func (m *model) resizeLayout() {
	if m.width <= 0 {
		m.width = defaultWidth
	}
	if m.height <= 0 {
		m.height = defaultHeight
	}

	usable := m.width - 8
	if usable < 72 {
		usable = 72
	}

	sevWidth := 8
	typeWidth := 24
	collWidth := 28
	msgWidth := usable - sevWidth - typeWidth - collWidth
	if msgWidth < 20 {
		msgWidth = 20
	}

	cols := []table.Column{
		{Title: "SEV", Width: sevWidth},
		{Title: "TYPE", Width: typeWidth},
		{Title: "COLLECTION", Width: collWidth},
		{Title: "MESSAGE", Width: msgWidth},
	}
	m.table.SetColumns(cols)

	tableHeight := m.height - 10
	if tableHeight < 8 {
		tableHeight = 8
	}
	m.table.SetHeight(tableHeight)

	filterWidth := m.width - 28
	if filterWidth < 24 {
		filterWidth = 24
	}
	m.filter.Width = filterWidth

	m.detail.Width = m.width - 4
	if m.detail.Width < 48 {
		m.detail.Width = 48
	}
	m.detail.Height = m.height - 6
	if m.detail.Height < 8 {
		m.detail.Height = 8
	}
	if m.detailMode {
		m.setDetailContent()
	}
}

func (m *model) refreshRows() {
	query := strings.TrimSpace(m.filter.Value())

	filtered := make([]findingEntry, 0, len(m.entries))
	for i := range m.entries {
		if matchesFilter(&m.entries[i].finding, query) {
			filtered = append(filtered, m.entries[i])
		}
	}

	sortEntries(filtered, m.sortMode)
	m.filtered = filtered

	rows := make([]table.Row, 0, len(filtered))
	for i := range filtered {
		f := filtered[i].finding
		rows = append(rows, table.Row{
			strings.ToUpper(string(f.Severity)),
			string(f.Type),
			tableCollectionLabel(&f),
			truncateText(f.Message, 140),
		})
	}
	m.table.SetRows(rows)
	if len(rows) == 0 {
		m.table.SetCursor(0)
		return
	}
	if m.table.Cursor() >= len(rows) {
		m.table.SetCursor(len(rows) - 1)
	}
}

func (m *model) selectedEntry() (findingEntry, bool) {
	if len(m.filtered) == 0 {
		return findingEntry{}, false
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.filtered) {
		return findingEntry{}, false
	}
	return m.filtered[idx], true
}

func (m *model) setDetailContent() {
	entry, ok := m.selectedEntry()
	if !ok {
		m.detail.SetContent("No finding selected.")
		return
	}
	m.detail.SetContent(m.renderDetail(&entry.finding))
	m.detail.GotoTop()
}

func (m *model) View() string {
	if m.detailMode {
		return m.detailView()
	}
	return m.listView()
}

func (m *model) listView() string {
	summary := summarizeEntries(m.filtered)
	header := fmt.Sprintf(
		"mongospectre interactive | findings %d/%d | high:%d medium:%d low:%d info:%d | sort:%s",
		len(m.filtered), len(m.entries), summary.high, summary.medium, summary.low, summary.info, m.sortMode.String(),
	)

	filterLabel := "Filter (/): "
	if m.filtering {
		filterLabel = "Filter (editing): "
	}
	filterRow := sectionStyle.Render(filterLabel) + m.filter.View()

	body := m.table.View()
	if len(m.filtered) == 0 {
		body = warnStyle.Render("No findings match the current filter.")
	}

	footer := statusStyle.Render(m.status)

	return strings.Join([]string{
		headerStyle.Render(header),
		filterRow,
		body,
		footer,
	}, "\n")
}

func (m *model) detailView() string {
	entry, ok := m.selectedEntry()
	title := "Finding Detail"
	if ok {
		title = fmt.Sprintf(
			"Finding Detail | %s | %s",
			entry.finding.Type, strings.ToUpper(string(entry.finding.Severity)),
		)
	}

	return strings.Join([]string{
		headerStyle.Render(title),
		m.detail.View(),
		statusStyle.Render("Up/Down scroll, PgUp/PgDn page, b or Esc back, q quit"),
	}, "\n")
}

func summarizeEntries(entries []findingEntry) severitySummary {
	var out severitySummary
	for i := range entries {
		out.total++
		switch entries[i].finding.Severity {
		case analyzer.SeverityHigh:
			out.high++
		case analyzer.SeverityMedium:
			out.medium++
		case analyzer.SeverityLow:
			out.low++
		default:
			out.info++
		}
	}
	return out
}

func matchesFilter(f *analyzer.Finding, query string) bool {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		string(f.Type),
		string(f.Severity),
		f.Database,
		f.Collection,
		f.Index,
		f.Message,
	}, " "))
	for _, token := range strings.Fields(query) {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func sortEntries(entries []findingEntry, mode sortMode) {
	sort.SliceStable(entries, func(i, j int) bool {
		a := entries[i].finding
		b := entries[j].finding
		aCollection := strings.ToLower(a.Collection)
		bCollection := strings.ToLower(b.Collection)

		switch mode {
		case sortByType:
			if a.Type != b.Type {
				return a.Type < b.Type
			}
		case sortByCollection:
			if !strings.EqualFold(a.Collection, b.Collection) {
				return aCollection < bCollection
			}
		default:
			if severityRank(a.Severity) != severityRank(b.Severity) {
				return severityRank(a.Severity) < severityRank(b.Severity)
			}
		}

		if !strings.EqualFold(a.Collection, b.Collection) {
			return aCollection < bCollection
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return entries[i].id < entries[j].id
	})
}

func severityRank(sev analyzer.Severity) int {
	switch sev {
	case analyzer.SeverityHigh:
		return 0
	case analyzer.SeverityMedium:
		return 1
	case analyzer.SeverityLow:
		return 2
	default:
		return 3
	}
}

func tableCollectionLabel(f *analyzer.Finding) string {
	loc := findingLocation(f)
	if loc == "" {
		return "-"
	}
	return loc
}

func findingLocation(f *analyzer.Finding) string {
	var loc string
	switch {
	case f.Database != "" && f.Collection != "":
		loc = f.Database + "." + f.Collection
	case f.Collection != "":
		loc = f.Collection
	case f.Database != "":
		loc = f.Database
	}
	if f.Index != "" {
		if loc == "" {
			loc = f.Index
		} else {
			loc += "." + f.Index
		}
	}
	return loc
}

func truncateText(s string, max int) string {
	if max <= 3 {
		return s
	}
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func buildCollectionLookup(collections []mongoinspect.CollectionInfo) map[string]mongoinspect.CollectionInfo {
	lookup := make(map[string]mongoinspect.CollectionInfo, len(collections)*2)
	for i := range collections {
		c := collections[i]
		fullKey := strings.ToLower(c.Database + "." + c.Name)
		lookup[fullKey] = c
		shortKey := strings.ToLower(c.Name)
		if _, exists := lookup[shortKey]; !exists {
			lookup[shortKey] = c
		}
	}
	return lookup
}

func (m *model) lookupCollection(f *analyzer.Finding) (mongoinspect.CollectionInfo, bool) {
	if f.Collection == "" {
		return mongoinspect.CollectionInfo{}, false
	}
	if f.Database != "" {
		fullKey := strings.ToLower(f.Database + "." + f.Collection)
		if c, ok := m.collections[fullKey]; ok {
			return c, true
		}
	}
	shortKey := strings.ToLower(f.Collection)
	c, ok := m.collections[shortKey]
	return c, ok
}

func lookupIndex(c *mongoinspect.CollectionInfo, indexName string) (mongoinspect.IndexInfo, bool) {
	if indexName == "" {
		return mongoinspect.IndexInfo{}, false
	}
	for i := range c.Indexes {
		if c.Indexes[i].Name == indexName {
			return c.Indexes[i], true
		}
	}
	return mongoinspect.IndexInfo{}, false
}

func (m *model) renderDetail(f *analyzer.Finding) string {
	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "%s\n", sectionStyle.Render("Overview"))
	_, _ = fmt.Fprintf(&b, "Type: %s\n", f.Type)
	_, _ = fmt.Fprintf(&b, "Severity: %s\n", strings.ToUpper(string(f.Severity)))
	_, _ = fmt.Fprintf(&b, "Location: %s\n", tableCollectionLabel(f))
	_, _ = fmt.Fprintf(&b, "Message: %s\n", f.Message)

	if coll, ok := m.lookupCollection(f); ok {
		_, _ = fmt.Fprintf(&b, "\n%s\n", sectionStyle.Render("Collection Context"))
		_, _ = fmt.Fprintf(
			&b,
			"Documents: %d | Storage: %s | Indexes: %d\n",
			coll.DocCount,
			formatBytes(coll.StorageSize),
			len(coll.Indexes),
		)

		if idx, ok := lookupIndex(&coll, f.Index); ok {
			_, _ = fmt.Fprintf(&b, "Index definition: %s", formatIndexKey(idx.Key))
			if idx.Unique {
				_, _ = fmt.Fprintf(&b, " unique")
			}
			if idx.Sparse {
				_, _ = fmt.Fprintf(&b, " sparse")
			}
			if idx.TTL != nil {
				_, _ = fmt.Fprintf(&b, " ttl=%ds", *idx.TTL)
			}
			_, _ = fmt.Fprintln(&b)
		}

		if coll.Validator != nil && coll.Validator.Schema.Properties != nil {
			_, _ = fmt.Fprintf(
				&b,
				"Validator fields: %d (level=%s action=%s)\n",
				len(coll.Validator.Schema.Properties),
				coll.Validator.ValidationLevel,
				coll.Validator.ValidationAction,
			)
		}
	}

	refs := m.refsForFinding(f)
	_, _ = fmt.Fprintf(&b, "\n%s\n", sectionStyle.Render("Code References"))
	if len(refs) == 0 {
		_, _ = fmt.Fprintln(&b, "No scanner references captured for this finding.")
	} else {
		limit := len(refs)
		if limit > maxDetailCodeRefRows {
			limit = maxDetailCodeRefRows
		}
		for i := 0; i < limit; i++ {
			_, _ = fmt.Fprintf(&b, "- %s\n", formatCodeRef(refs[i]))
		}
		if len(refs) > limit {
			_, _ = fmt.Fprintf(&b, "... and %d more\n", len(refs)-limit)
		}
	}

	_, _ = fmt.Fprintf(&b, "\n%s\n", sectionStyle.Render("Suggested Fix"))
	_, _ = fmt.Fprintln(&b, suggestionForFinding(f))

	return b.String()
}

func (m *model) refsForFinding(f *analyzer.Finding) []codeRef {
	if f.Type == analyzer.FindingDynamicCollection {
		return m.dynamicRefs
	}
	if f.Collection == "" {
		return nil
	}
	return m.codeRefs[strings.ToLower(f.Collection)]
}

func buildCodeRefs(scan *scanner.ScanResult) (map[string][]codeRef, []codeRef) {
	byCollection := make(map[string][]codeRef)
	if scan == nil {
		return byCollection, nil
	}

	for i := range scan.Refs {
		ref := scan.Refs[i]
		key := strings.ToLower(ref.Collection)
		byCollection[key] = append(byCollection[key], codeRef{
			File:   ref.File,
			Line:   ref.Line,
			Kind:   "collection",
			Detail: string(ref.Pattern),
		})
	}

	for i := range scan.FieldRefs {
		ref := scan.FieldRefs[i]
		key := strings.ToLower(ref.Collection)
		byCollection[key] = append(byCollection[key], codeRef{
			File:   ref.File,
			Line:   ref.Line,
			Kind:   "query",
			Detail: ref.Field,
		})
	}

	for i := range scan.WriteRefs {
		ref := scan.WriteRefs[i]
		key := strings.ToLower(ref.Collection)
		detail := ref.Field
		if detail == "" {
			detail = "(collection write)"
		}
		if ref.ValueType != "" {
			detail += " " + ref.ValueType
		}
		byCollection[key] = append(byCollection[key], codeRef{
			File:   ref.File,
			Line:   ref.Line,
			Kind:   "write",
			Detail: detail,
		})
	}

	dynamic := make([]codeRef, 0, len(scan.DynamicRefs))
	for i := range scan.DynamicRefs {
		ref := scan.DynamicRefs[i]
		dynamic = append(dynamic, codeRef{
			File:   ref.File,
			Line:   ref.Line,
			Kind:   "dynamic",
			Detail: ref.Variable,
		})
	}

	sortRefs := func(refs []codeRef) {
		sort.SliceStable(refs, func(i, j int) bool {
			if refs[i].File != refs[j].File {
				return refs[i].File < refs[j].File
			}
			if refs[i].Line != refs[j].Line {
				return refs[i].Line < refs[j].Line
			}
			if refs[i].Kind != refs[j].Kind {
				return refs[i].Kind < refs[j].Kind
			}
			return refs[i].Detail < refs[j].Detail
		})
	}

	for key := range byCollection {
		sortRefs(byCollection[key])
	}
	sortRefs(dynamic)

	return byCollection, dynamic
}

func formatCodeRef(ref codeRef) string {
	location := fileStyle.Render(ref.File) + ":" + lineStyle.Render(fmt.Sprintf("%d", ref.Line))
	detail := ref.Kind
	if ref.Detail != "" {
		detail += " " + ref.Detail
	}
	return location + "  " + kindStyle.Render(detail)
}

func formatIndexKey(keys []mongoinspect.KeyField) string {
	if len(keys) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(keys))
	for i := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", keys[i].Field, keys[i].Direction))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func formatBytes(size int64) string {
	if size <= 0 {
		return "0 B"
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KB", "MB", "GB", "TB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}
	value := float64(size) / float64(div)
	return fmt.Sprintf("%.1f %s", value, suffixes[exp])
}

func suggestionForFinding(f *analyzer.Finding) string {
	switch f.Type {
	case analyzer.FindingMissingIndex:
		return "Create one or more indexes that match your top query predicates and sort order."
	case analyzer.FindingUnindexedQuery:
		return "Add an index that starts with the queried field, then re-run explain plans."
	case analyzer.FindingSuggestIndex:
		return "Validate this suggested index against production query patterns before creating it."
	case analyzer.FindingUnusedCollection:
		return "Confirm with owners, then archive or drop the collection if it is no longer needed."
	case analyzer.FindingUnusedIndex, analyzer.FindingOrphanedIndex:
		return "Check index usage over time, then drop the index if query coverage is still unnecessary."
	case analyzer.FindingDuplicateIndex:
		return "Keep the broader index and remove redundant prefix indexes where safe."
	case analyzer.FindingMissingCollection:
		return "Create the missing collection or update code references to the correct collection name."
	case analyzer.FindingMissingTTL:
		return "Add a TTL index on the timestamp field if documents should expire automatically."
	case analyzer.FindingOversizedCollection:
		return "Partition data (archival, bucketing, or sharding) and verify growth controls."
	case analyzer.FindingValidatorMissing:
		return "Add a JSON Schema validator to enforce expected document structure."
	case analyzer.FindingValidatorStale, analyzer.FindingFieldNotInValidator:
		return "Update validator schema to reflect current write patterns and required fields."
	case analyzer.FindingValidatorStrictRisk:
		return "Review strict validator mode and ensure all writers conform before rollout."
	case analyzer.FindingValidatorWarnOnly:
		return "Switch validator action/level from warning to enforcement after cleanup."
	case analyzer.FindingDynamicCollection:
		return "Resolve dynamic collection names at compile-time or add explicit allowlists."
	case analyzer.FindingAdminInDataDB, analyzer.FindingOverprivilegedUser, analyzer.FindingMultipleAdminUsers:
		return "Reduce user privileges to least privilege and isolate administrative access."
	case analyzer.FindingDuplicateUser:
		return "Consolidate duplicate usernames across databases and remove stale accounts."
	default:
		return "Review this finding with application owners, then apply and validate the minimal safe fix."
	}
}

func (m *model) exportFiltered() (string, error) {
	findings := make([]analyzer.Finding, len(m.filtered))
	for i := range m.filtered {
		findings[i] = m.filtered[i].finding
	}
	report := reporter.NewReport(findings)
	report.Metadata = m.metadata
	report.Metadata.Timestamp = time.Now().UTC().Format(time.RFC3339)

	filename := fmt.Sprintf("mongospectre-findings-%s.json", time.Now().UTC().Format("20060102-150405"))
	path := filepath.Clean(filename)

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal export: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", fmt.Errorf("write export: %w", err)
	}

	return path, nil
}
