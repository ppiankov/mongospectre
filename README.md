# mongospectre

[![CI](https://github.com/ppiankov/mongospectre/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/mongospectre/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

MongoDB collection and index auditor. Scans codebases for collection/field references, compares with live MongoDB schema and statistics, detects drift.

## What This Is

A CLI tool that:

- Connects to MongoDB and fetches collection metadata, index definitions, and usage statistics
- Scans code repositories for MongoDB collection and field references (Go, Python, JS/TS, Java, C#, Ruby)
- Extracts queried fields from aggregation pipelines (`$match`, `$sort`, `$group`, `$lookup`, `$project`, `$unwind`)
- Compares code references against live database to find missing collections, unused indexes, unindexed queries, and drift
- Produces text, JSON, SARIF, or SpectreHub reports
- Watches clusters continuously for drift detection

Part of the **Spectre** family — code-vs-reality drift detection tools.

## What This Is NOT

- Not a MongoDB monitoring tool (use `mongostat`/`mongotop`)
- Not a migration tool
- Not a query profiler or optimizer
- Not a backup or replication tool
- Does not modify any data — all queries are strictly read-only

## Quick Start

```bash
# Homebrew
brew install ppiankov/tap/mongospectre

# Go install
go install github.com/ppiankov/mongospectre/cmd/mongospectre@latest

# Docker
docker run --rm ghcr.io/ppiankov/mongospectre:latest audit --uri "mongodb://host.docker.internal:27017"

# Or download a release binary
curl -LO https://github.com/ppiankov/mongospectre/releases/latest/download/mongospectre_$(uname -s | tr A-Z a-z)_$(uname -m).tar.gz
tar -xzf mongospectre_*.tar.gz
sudo mv mongospectre /usr/local/bin/
```

```bash
# Scaffold config files
mongospectre init

# Audit a cluster (no code scanning)
mongospectre audit --uri "mongodb://localhost:27017"

# Check code repo against live cluster
mongospectre check --repo ./my-app --uri "mongodb://localhost:27017"

# SARIF output for GitHub Security integration
mongospectre audit --uri "mongodb://..." --format sarif > results.sarif

# Continuous monitoring
mongospectre watch --uri "mongodb://..." --interval 5m
```

## Usage

### `audit` — Cluster-Only Analysis

Inspects MongoDB without code scanning. Detects:

| Finding | Severity | Description |
|---------|----------|-------------|
| `UNUSED_COLLECTION` | medium | Collection has 0 documents |
| `UNUSED_INDEX` | medium | Index has never been queried |
| `MISSING_INDEX` | high | Large collection with only `_id` index |
| `DUPLICATE_INDEX` | low | Index key is a prefix of another index |
| `OVERSIZED_COLLECTION` | low | Collection exceeds 10 GB |
| `MISSING_TTL` | low | Timestamp field indexed without TTL |

```bash
mongospectre audit --uri "mongodb://..." [--database mydb] [--format text|json|sarif|spectrehub]
```

### `check` — Code + Cluster Diff

Scans a code repository and compares collection references against live MongoDB:

| Finding | Severity | Description |
|---------|----------|-------------|
| `MISSING_COLLECTION` | high | Referenced in code, doesn't exist in DB |
| `UNINDEXED_QUERY` | medium | Queried field has no covering index |
| `UNUSED_COLLECTION` | medium | Exists in DB with 0 docs, not in code |
| `SUGGEST_INDEX` | info | Consider adding an index for queried field |
| `ORPHANED_INDEX` | low | Unused index on unreferenced collection |
| `OK` | info | Collection exists and is referenced |

```bash
mongospectre check --repo ./app --uri "mongodb://..." [--database mydb] [--format text|json|sarif|spectrehub] [--fail-on-missing]
```

### `compare` — Cross-Cluster Schema Diff

Compares schemas between two MongoDB clusters (e.g., staging vs production):

```bash
mongospectre compare --source "mongodb://staging:27017" --target "mongodb://prod:27017" [--format text|json]
```

### `watch` — Continuous Monitoring

Runs `audit` on a configurable interval and prints only new/resolved findings:

```bash
mongospectre watch --uri "mongodb://..." --interval 5m [--format text|json] [--exit-on-new]
```

- First run: full audit with all findings
- Subsequent runs: prints only `+ [new]` and `- [resolved]` changes
- `--exit-on-new`: exit with code 2 on first new high-severity finding (for CI)
- `--format json`: outputs NDJSON events (one per line)
- Ctrl+C: prints summary and exits cleanly

### `init` — Scaffold Config Files

Creates starter `.mongospectre.yml` and `.mongospectreignore` in the current directory:

```bash
mongospectre init
```

Skips files that already exist. See `docs/examples/` for annotated templates.

### Docker

```bash
# Run audit against a MongoDB instance
docker run --rm ghcr.io/ppiankov/mongospectre:latest audit --uri "mongodb://host:27017"

# Local development with docker-compose (includes mongo:7 sidecar)
docker compose up
```

Multi-arch images (amd64/arm64) are published to `ghcr.io/ppiankov/mongospectre` on every release.

### GitHub Action

```yaml
- uses: ppiankov/mongospectre@v0.2.0
  with:
    command: audit
    uri: ${{ secrets.MONGODB_URI }}
    args: "--database mydb --format json"
    upload-sarif: "true"  # optional: upload to GitHub Security tab
```

See `action/action.yml` for all inputs and outputs. More CI examples in `docs/ci-examples.md`.

### Baseline Comparison

Compare current findings against a previous report to track drift over time:

```bash
# Save baseline
mongospectre audit --uri "mongodb://..." --format json > baseline.json

# Compare later
mongospectre audit --uri "mongodb://..." --baseline baseline.json
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No issues or low/info only |
| 1 | Medium severity findings |
| 2 | High severity findings |

## Configuration

### `.mongospectre.yml`

Optional config file in the working directory:

```yaml
uri: mongodb://localhost:27017
defaults:
  verbose: false
  timeout: 30s
  database: myapp
```

CLI flags override config file values. The `MONGODB_URI` environment variable also works.

### `.mongospectreignore`

Suppress specific findings:

```
# Ignore all findings for a collection
mydb.audit_logs

# Ignore a specific finding type
UNUSED_INDEX mydb.users.idx_legacy

# Ignore by pattern
UNUSED_COLLECTION mydb.tmp_*
```

## Output Formats

| Format | Flag | Description |
|--------|------|-------------|
| text | `--format text` | Human-readable (default) |
| json | `--format json` | Structured JSON report |
| sarif | `--format sarif` | SARIF v2.1.0 for GitHub Security |
| spectrehub | `--format spectrehub` | SpectreHub `spectre/v1` envelope |

### SARIF Upload Example

```yaml
# .github/workflows/audit.yml
- run: mongospectre audit --uri "$MONGODB_URI" --format sarif > mongospectre.sarif
- uses: github/codeql-action/upload-sarif@v4
  with:
    sarif_file: mongospectre.sarif
```

## Architecture

```
cmd/mongospectre/main.go   — CLI entry point
internal/cli/              — Cobra commands (audit, check, compare, watch)
internal/config/           — YAML config and ignore file loading
internal/mongo/            — MongoDB inspector (read-only queries)
internal/scanner/          — Code repo collection + field reference scanner
internal/analyzer/         — Detection engines (audit, diff, compare, baseline)
internal/reporter/         — Text/JSON/SARIF/SpectreHub report output
```

### Supported Languages

The code scanner detects MongoDB collection references in:

- **Go** — `db.Collection("x")`, `bson.M{"field": ...}`, `bson.D{{Key: "field", ...}}`
- **JavaScript/TypeScript** — `db.collection("x")`, `db.getCollection("x")`
- **Python** — `db["x"]`, `db.x.find(...)`, PyMongo, MongoEngine
- **Mongoose** — `mongoose.model("X", schema)` (auto-pluralizes)
- **Java/C#** — `GetCollection("x")`

### Aggregation Pipeline Analysis

The scanner extracts fields from aggregation pipeline stages:

- `$match` — filter fields
- `$sort` — sort key fields
- `$project` / `$addFields` — projected fields
- `$group` — `_id` and accumulator field references (`$field`)
- `$unwind` — path field
- `$lookup` — `localField`, `foreignField`, and `from` (as collection reference)

## Building from Source

```bash
git clone https://github.com/ppiankov/mongospectre.git
cd mongospectre
make build    # produces bin/mongospectre
make test     # run tests with -race
make lint     # golangci-lint
make bench    # run benchmarks
```

## Known Limitations

- Variable tracking is limited to same-file assignments (`collName := "users"` then `db.Collection(collName)`)
- PyMongo dot access (`db.users.find`) requires a known operation suffix to avoid false positives
- `$indexStats` requires MongoDB 3.2+ and may not be available on all hosting providers

## License

[MIT](LICENSE)
