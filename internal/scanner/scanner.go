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

		refs, scanErr := scanFile(path, repoPath)
		if scanErr != nil {
			return nil // skip unreadable files
		}

		result.FilesScanned++
		result.Refs = append(result.Refs, refs...)
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("walk %s: %w", repoPath, err)
	}

	result.Collections = uniqueCollections(result.Refs)
	return result, nil
}

// scanFile reads a file line by line and returns collection references found.
func scanFile(path, repoPath string) ([]CollectionRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	relPath, _ := filepath.Rel(repoPath, path)
	if relPath == "" {
		relPath = path
	}

	var refs []CollectionRef
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		for _, m := range ScanLine(line) {
			refs = append(refs, CollectionRef{
				Collection: m.Collection,
				File:       relPath,
				Line:       lineNum,
				Pattern:    m.Pattern,
			})
		}
	}
	return refs, scanner.Err()
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
