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

		refs, fieldRefs, scanErr := scanFile(path, repoPath)
		if scanErr != nil {
			result.FilesSkipped++
			return nil
		}

		result.FilesScanned++
		result.Refs = append(result.Refs, refs...)
		result.FieldRefs = append(result.FieldRefs, fieldRefs...)
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("walk %s: %w", repoPath, err)
	}

	result.Collections = uniqueCollections(result.Refs)
	return result, nil
}

// scanFile reads a file line by line and returns collection and field references found.
func scanFile(path, repoPath string) ([]CollectionRef, []FieldRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	relPath, _ := filepath.Rel(repoPath, path)
	if relPath == "" {
		relPath = path
	}

	var refs []CollectionRef
	var fieldRefs []FieldRef
	sc := bufio.NewScanner(f)
	lineNum := 0

	// Track which collection is in scope for field extraction.
	// On a given line, the collection comes from the same line's ScanLine match.
	for sc.Scan() {
		lineNum++
		line := sc.Text()

		var lineCollection string
		for _, m := range ScanLine(line) {
			refs = append(refs, CollectionRef{
				Collection: m.Collection,
				File:       relPath,
				Line:       lineNum,
				Pattern:    m.Pattern,
			})
			if lineCollection == "" {
				lineCollection = m.Collection
			}
		}

		// Extract queried fields if we have a collection context on this line.
		if lineCollection != "" {
			for _, fm := range ScanLineFields(line) {
				fieldRefs = append(fieldRefs, FieldRef{
					Collection: lineCollection,
					Field:      fm.Field,
					File:       relPath,
					Line:       lineNum,
				})
			}
		}
	}
	return refs, fieldRefs, sc.Err()
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
