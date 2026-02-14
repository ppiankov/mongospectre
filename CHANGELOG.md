# Changelog

All notable changes to this project will be documented in this file.

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
