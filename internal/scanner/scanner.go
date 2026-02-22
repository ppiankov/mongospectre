package scanner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var supportedExtensions = map[string]bool{
	".go":   true,
	".py":   true,
	".js":   true,
	".ts":   true,
	".jsx":  true,
	".tsx":  true,
	".java": true,
	".cs":   true,
	".rb":   true,
}

// skipDirs are directory names to always skip.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".git":         true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	"dist":         true,
	"build":        true,
	"bin":          true,
}

// Scan walks a directory tree and finds all MongoDB collection references.
func Scan(repoPath string) (ScanResult, error) {
	result := ScanResult{RepoPath: repoPath}

	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExtensions[ext] {
			return nil
		}

		refs, fieldRefs, writeRefs, dynRefs, scanErr := scanFile(path, repoPath)
		if scanErr != nil {
			result.FilesSkipped++
			return nil
		}

		result.FilesScanned++
		result.Refs = append(result.Refs, refs...)
		result.FieldRefs = append(result.FieldRefs, fieldRefs...)
		result.WriteRefs = append(result.WriteRefs, writeRefs...)
		result.DynamicRefs = append(result.DynamicRefs, dynRefs...)
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("walk %s: %w", repoPath, err)
	}

	result.Collections = uniqueCollections(result.Refs)
	return result, nil
}

// scanFile reads a file, joins multi-line expressions, and returns collection refs,
// field refs, and dynamic (unresolvable variable) refs.
func scanFile(path, repoPath string) ([]CollectionRef, []FieldRef, []WriteRef, []DynamicRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer func() { _ = f.Close() }()

	relPath, _ := filepath.Rel(repoPath, path)
	if relPath == "" {
		relPath = path
	}

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, nil, nil, nil, err
	}

	joined := joinContinuationLines(lines)
	stringVars := collectStringVars(joined)

	var refs []CollectionRef
	var fieldRefs []FieldRef
	var writeRefs []WriteRef
	var dynamicRefs []DynamicRef
	seenDynamic := make(map[string]bool)

	for _, jl := range joined {
		lineMatches := ScanLine(jl.text)

		// If no literal collection match, try variable resolution.
		if len(lineMatches) == 0 {
			resolved, dynamicVars := resolveVarCollections(jl.text, stringVars)
			lineMatches = resolved
			for _, v := range dynamicVars {
				if !seenDynamic[v] {
					seenDynamic[v] = true
					dynamicRefs = append(dynamicRefs, DynamicRef{
						Variable: v,
						File:     relPath,
						Line:     jl.lineNum,
					})
				}
			}
		}

		var lineCollection string
		for _, m := range lineMatches {
			refs = append(refs, CollectionRef{
				Collection: m.Collection,
				File:       relPath,
				Line:       jl.lineNum,
				Pattern:    m.Pattern,
			})
			if lineCollection == "" {
				lineCollection = m.Collection
			}
		}

		if lineCollection != "" {
			for _, fm := range ScanLineFields(jl.text) {
				fieldRefs = append(fieldRefs, FieldRef{
					Collection:   lineCollection,
					Field:        fm.Field,
					File:         relPath,
					Line:         jl.lineNum,
					Usage:        fm.Usage,
					Direction:    fm.Direction,
					QueryContext: fm.QueryContext,
				})
			}
			if IsWriteOperation(jl.text) {
				writes := ScanLineWriteFields(jl.text)
				if len(writes) == 0 {
					// Record collection-level write intent even when field extraction fails.
					writeRefs = append(writeRefs, WriteRef{
						Collection: lineCollection,
						File:       relPath,
						Line:       jl.lineNum,
					})
				}
				for _, w := range writes {
					writeRefs = append(writeRefs, WriteRef{
						Collection: lineCollection,
						Field:      w.Field,
						ValueType:  w.ValueType,
						File:       relPath,
						Line:       jl.lineNum,
					})
				}
			}
		}
	}
	return refs, fieldRefs, writeRefs, dynamicRefs, nil
}

// joinedLine holds a possibly multi-line expression with its starting line number.
type joinedLine struct {
	text    string
	lineNum int
}

// maxJoinLines limits how many lines can be joined into a single expression.
const maxJoinLines = 5

// joinContinuationLines merges lines that are part of multi-line expressions
// by tracking parenthesis balance. Single-line expressions pass through unchanged.
func joinContinuationLines(lines []string) []joinedLine {
	var result []joinedLine
	var buf strings.Builder
	startLine := 0
	depth := 0
	joinCount := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if depth == 0 {
			buf.Reset()
			buf.WriteString(trimmed)
			startLine = i + 1
			depth = parenBalance(trimmed)
			joinCount = 0
		} else {
			buf.WriteString(" ")
			buf.WriteString(trimmed)
			depth += parenBalance(trimmed)
			joinCount++
		}

		if depth <= 0 || joinCount >= maxJoinLines {
			result = append(result, joinedLine{
				text:    buf.String(),
				lineNum: startLine,
			})
			depth = 0
		}
	}

	// Flush remaining buffer if file ends with unclosed parens.
	if depth > 0 {
		result = append(result, joinedLine{
			text:    buf.String(),
			lineNum: startLine,
		})
	}

	return result
}

// parenBalance counts the net parenthesis depth change for a line,
// skipping characters inside string literals and line comments.
func parenBalance(s string) int {
	depth := 0
	inStr := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if inStr == '`' {
				// Raw/template string — only ends with backtick.
				if c == '`' {
					inStr = 0
				}
				continue
			}
			if c == '\\' {
				i++ // skip escaped character
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '/':
			if i+1 < len(s) && s[i+1] == '/' {
				return depth // rest is a line comment
			}
		case '(':
			depth++
		case ')':
			depth--
		}
	}
	return depth
}

// uniqueCollections returns a sorted, deduplicated list of collection names.
func uniqueCollections(refs []CollectionRef) []string {
	seen := make(map[string]bool)
	for _, r := range refs {
		seen[r.Collection] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
