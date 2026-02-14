# Changelog

All notable changes to this project will be documented in this file.

## [0.1.1] - 2026-02-14

### Added

- `compare` command — detect drift between two MongoDB clusters (staging vs prod)
- Field-level query scanning — extract queried fields from Go, JS, Python code
- `UNINDEXED_QUERY` finding — field queried in code with no covering index
- `SUGGEST_INDEX` finding — recommend indexes for unindexed queried fields
- SARIF output format (`--format sarif`) for GitHub Security tab integration
- Config file support (`.mongospectre.yml`) with thresholds, exclusions, defaults
- Ignore file (`.mongospectreignore`) for suppressing known-ok findings
- Baseline diff (`--baseline report.json`) to show new/resolved/unchanged findings
- Integration tests with testcontainers (`make test-integration`)
- `MONGODB_URI` environment variable fallback for `--uri` flag
- `--timeout` flag for connection and operation timeout
- `--verbose` flag for detailed output
- `--no-ignore` flag to bypass ignore file
- Report metadata (version, command, timestamp, MongoDB version)
- Mongoose model-to-collection pluralization in scanner

### Fixed

- golangci-lint v2 config compatibility (`linters-settings` → `linters.settings`, `exclude-dirs` → `run.exclude-dirs`)

## [0.1.0] - 2026-02-14

### Added

- `audit` command — cluster-only analysis for unused collections, unused indexes, missing indexes, duplicate indexes, oversized collections, and missing TTL indexes
- `check` command — compare code repository collection references against live MongoDB
- MongoDB inspector with read-only metadata queries (collections, indexes, index stats, server info)
- Code scanner supporting Go, Python, JavaScript/TypeScript, Java, C#, Ruby
- JSON and text report output formats
- Exit codes reflecting finding severity (0=ok, 1=medium, 2=high)
- `--database` flag to scope analysis to a specific database
- `--fail-on-missing` flag for CI pipelines
