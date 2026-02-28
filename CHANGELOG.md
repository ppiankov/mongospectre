# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.2.13] - 2026-02-28

### Added

- Replica set configuration audit (`--replset` flag on `audit` command)
- `InspectReplicaSet` inspector method using `replSetGetStatus`, `replSetGetConfig`, and oplog metadata
- Six new findings: `SINGLE_MEMBER_REPLSET`, `EVEN_MEMBER_COUNT`, `MEMBER_UNHEALTHY`, `OPLOG_SMALL`, `NO_HIDDEN_MEMBER`, `PRIORITY_ZERO_MAJORITY`
- Atlas auto-skip: replica set audit skipped gracefully on Atlas clusters (topology managed by Atlas)
- Standalone auto-skip: replica set audit skipped when deployment is not a replica set

## [0.2.12] - 2026-02-28

### Added

- Data model anti-pattern detection with six new findings: `UNBOUNDED_ARRAY`, `DEEP_NESTING`, `LARGE_DOCUMENT`, `FIELD_NAME_COLLISION`, `EXCESSIVE_FIELD_COUNT`, `NUMERIC_FIELD_NAMES`
- Per-document metrics in schema sampling: `MaxDocSize`, `MaxFieldCount`, `ArrayLengths`

## [0.2.11] - 2026-02-28

### Added

- Index efficiency analysis with four new findings: `INDEX_BLOAT`, `WRITE_HEAVY_OVER_INDEXED`, `SINGLE_FIELD_REDUNDANT`, `LARGE_INDEX`
- `TotalIndexSize` and per-index `Size` fields extracted from `collStats`

## [0.2.10] - 2026-02-28

### Added

- Security hardening audit (`--security` flag on `audit` command)
- `InspectSecurity` inspector method using `getParameter` and `getCmdLineOpts` admin commands
- Six new findings: `AUTH_DISABLED`, `BIND_ALL_INTERFACES`, `TLS_DISABLED`, `TLS_ALLOW_INVALID_CERTS`, `AUDIT_LOG_DISABLED`, `LOCALHOST_EXCEPTION_ACTIVE`
- Atlas auto-skip: security audit skipped gracefully on Atlas clusters (hardened by default)

## [0.2.9] - 2026-02-28

### Added

- Connection string linting with 8 new URI findings (`--lint-uri` flag, default on)
- Static URI analysis runs as pre-flight check before connecting to MongoDB
- New findings: `URI_NO_AUTH`, `URI_NO_TLS`, `URI_NO_RETRY_WRITES`, `URI_PLAINTEXT_PASSWORD`, `URI_DEFAULT_AUTH_SOURCE`, `URI_SHORT_TIMEOUT`, `URI_NO_READ_PREFERENCE`, `URI_DIRECT_CONNECTION`

## [0.2.8] - 2026-02-27

### Added

- Schema sampling and field-level drift detection (`--sample` flag on `check` command)
- `SampleDocuments` inspector method using `$sample` aggregation
- Four new findings: `MISSING_FIELD`, `RARE_FIELD`, `UNDOCUMENTED_FIELD`, `TYPE_INCONSISTENCY`

## [0.2.7] - 2026-02-27

### Added

- Inactive user detection via Atlas access history API (`INACTIVE_USER`, `FAILED_AUTH_ONLY`, `INACTIVE_PRIVILEGED_USER`)
- `ListAccessLogs` Atlas Admin API method with pagination support
- Inactive user analysis runs automatically with `--audit-users` when Atlas credentials are provided

## [0.2.6] - 2026-02-26

### Fixed

- Atlas API 406 error: use versioned `Accept` header required by Atlas Admin API v2

## [0.2.5] - 2026-02-26

### Added

- Atlas API fallback for `--audit-users` on Atlas clusters where native `usersInfo` is unavailable
- `ListDatabaseUsers` Atlas Admin API method (`GET /api/atlas/v2/groups/{groupId}/databaseUsers`)
- `ATLAS_USER_NO_SCOPE` finding for Atlas users with no cluster scope restriction
- Atlas client tests (`internal/atlas/client_test.go`)

### Changed

- `--audit-users` warning message now suggests Atlas API credentials when running on Atlas
- Troubleshooting guide expanded with Atlas-specific user audit instructions

## [0.2.4] - 2026-02-25

### Fixed

- `--audit-users` now shows a prominent warning when all user queries fail due to insufficient permissions, instead of silently producing no results
- Per-database user query errors moved to `--verbose` to reduce noise

### Added

- Troubleshooting section for user audit permission requirements

## [0.2.3] - 2026-02-25

### Added

- Cluster host displayed in connection banner and text report header
- `host` field in report metadata (JSON, SpectreHub formats)
- `HostFromURI` helper for safe hostname extraction from MongoDB URIs

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
