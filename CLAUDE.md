# mongospectre

MongoDB collection and index auditor. Scans codebases for collection/field references, compares with live MongoDB schema and statistics, detects drift.

## What This Is

CLI tool that connects to MongoDB, fetches collection metadata and index usage statistics, optionally scans a code repo for collection references, and produces audit reports. Part of the Spectre family (code-vs-reality drift detection).

## What This Is NOT

- Not a MongoDB monitoring tool (use mongostat/mongotop for that)
- Not a migration tool
- Not a query profiler
- Not a backup or replication tool

## Structure

```
cmd/mongospectre/main.go   — CLI entry point (Cobra)
internal/cli/              — Cobra commands (audit, check)
internal/mongo/            — collection inspector, index stats
internal/scanner/          — code repo collection reference scanner
internal/analyzer/         — diff engine (repo refs vs live collections)
internal/reporter/         — JSON/text report output
```

## Subcommands

- `audit` — cluster-only analysis: unused collections, unused indexes, missing indexes
- `check` — code repo + cluster: missing collections, schema drift, orphaned indexes

## Code Style

- Go with mongo-driver/v2 (official Go driver)
- Cobra for CLI
- All queries use read-only access (no admin required)
- Conventional commits: feat:, fix:, docs:, test:, refactor:, chore:

## Testing

- `make test` (includes -race)
- `make lint` (golangci-lint)
- Target: >85% coverage

## Anti-Patterns

- NEVER modify collections or indexes — read-only queries only
- NEVER auto-drop anything — report and recommend only
- NEVER require admin role — all queries must work with read-only access
- NEVER store credentials — use connection string or env vars only
