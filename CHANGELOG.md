# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.2.2] - 2026-02-25

### Fixed

- Panic on text, 2dsphere, 2d, and hashed index key types in Atlas clusters
- Ambiguous connection error messages now include actionable troubleshooting hints
- Double-wrapped "connect: connect:" error prefix in CLI output
- Go version mismatch in go.mod causing CI test failures
- Module tidy drift (charmbracelet deps promoted from indirect to direct)
- VS Code extension TypeScript error on Diagnostic.data property

### Added

- Connection error classification with hints for timeout, auth, DNS, and refused errors
- Troubleshooting guide (`docs/troubleshooting.md`)

## [0.2.1] - 2026-02-22

### Added

- VS Code extension in `vscode-mongospectre/` with inline diagnostics, hover metadata, quick-fix ignore actions, status bar summary, and debounced refresh
- Atlas integration for index suggestions correlated with code references
- TUI mode for interactive `watch` sessions
- Notification system (Slack, webhook, email) for `watch --notify`
- Code scanner for aggregation pipeline field extraction
- Safety model section in README
- Project status section in README

### Changed

- `check --format json` now includes scanner payload (`scan`) and inspected collection metadata (`collections`) for IDE integrations

### Security

- SHA-pin all GitHub Actions in CI and release workflows
- Scope release workflow permissions to job level
- Add `go mod verify` to release workflow for supply chain integrity
- Add `-trimpath` to GoReleaser builds to strip local paths from binaries
- Fix config and export file permissions (0644 → 0600)
- Disable gocritic hugeParam to avoid unnecessary pointer indirection

## [0.2.0] - 2026-02-15

### Added

- Docker image (multi-arch amd64/arm64) published to GHCR
- Homebrew formula via `ppiankov/homebrew-tap` (`brew install ppiankov/tap/mongospectre`)
- GitHub Action (`ppiankov/mongospectre`) for CI integration with optional SARIF upload
- `init` command — scaffolds `.mongospectre.yml` and `.mongospectreignore` in current directory
- First-run UX: text report header shows version, command, MongoDB version, and database
- Exit code hints on stderr explaining what exit 1 and exit 2 mean
- Empty collections hint when audit or check finds 0 collections
- User audit findings: `USER_EXCESS_ROLES`, `USER_NO_ROLES`, `USER_ADMIN_ON_DATA_DB`
- Multi-line query detection in code scanner (spans across line breaks)
- Variable tracking for collection name references (same-file assignments)
- CI integration examples for GitHub Actions and GitLab CI (`docs/ci-examples.md`)
- CLI integration test suite (50%+ coverage)

### Changed

- Release workflow rewritten to use GoReleaser (builds, Docker images, and Homebrew formula in one step)

### Removed

- Known limitation: multi-line query patterns are now detected

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
