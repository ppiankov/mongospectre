package analyzer

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// IgnoreRule matches findings to suppress.
type IgnoreRule struct {
	Type       string // finding type or "*" for any
	Database   string // database name or "*" for any
	Collection string // collection name or "*" for any
	Index      string // index name or "*"/empty for any
}

// IgnoreList holds parsed ignore rules.
type IgnoreList struct {
	Rules []IgnoreRule
}

// LoadIgnoreFile reads a .mongospectreignore file from the given directory.
// Returns an empty list if the file doesn't exist.
func LoadIgnoreFile(dir string) (IgnoreList, error) {
	path := filepath.Join(dir, ".mongospectreignore")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return IgnoreList{}, nil
		}
		return IgnoreList{}, err
	}
	defer func() { _ = f.Close() }()

	var rules []IgnoreRule
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule, ok := parseIgnoreRule(line)
		if ok {
			rules = append(rules, rule)
		}
	}
	return IgnoreList{Rules: rules}, sc.Err()
}

// parseIgnoreRule parses a single ignore rule line.
// Format: TYPE db.collection[.index]
// Examples:
//
//	UNUSED_INDEX app.legacy_users.idx_old
//	* app.audit_logs
//	MISSING_TTL app.settings
func parseIgnoreRule(line string) (IgnoreRule, bool) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return IgnoreRule{}, false
	}

	findingType := parts[0]
	target := parts[1]

	// Split target into db.collection[.index]
	segments := strings.SplitN(target, ".", 3)
	rule := IgnoreRule{Type: findingType}

	switch len(segments) {
	case 1:
		// Just a collection name (no db prefix)
		rule.Database = "*"
		rule.Collection = segments[0]
	case 2:
		rule.Database = segments[0]
		rule.Collection = segments[1]
	case 3:
		rule.Database = segments[0]
		rule.Collection = segments[1]
		rule.Index = segments[2]
	}

	return rule, true
}

// Matches checks if a finding should be suppressed by this rule.
func (r IgnoreRule) Matches(f Finding) bool {
	if r.Type != "*" && r.Type != string(f.Type) {
		return false
	}
	if r.Database != "*" && r.Database != "" && !matchGlob(r.Database, f.Database) {
		return false
	}
	if r.Collection != "*" && !matchGlob(r.Collection, f.Collection) {
		return false
	}
	if r.Index != "" && r.Index != "*" && r.Index != f.Index {
		return false
	}
	return true
}

// Filter removes findings that match any ignore rule.
// Returns the filtered list and the count of suppressed findings.
func (il IgnoreList) Filter(findings []Finding) ([]Finding, int) {
	if len(il.Rules) == 0 {
		return findings, 0
	}

	var filtered []Finding
	suppressed := 0
	for _, f := range findings {
		if il.matches(f) {
			suppressed++
		} else {
			filtered = append(filtered, f)
		}
	}
	return filtered, suppressed
}

func (il IgnoreList) matches(f Finding) bool {
	for _, r := range il.Rules {
		if r.Matches(f) {
			return true
		}
	}
	return false
}

// matchGlob checks if pattern matches value. Supports trailing "*" only.
func matchGlob(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, pattern[:len(pattern)-1])
	}
	return pattern == value
}
