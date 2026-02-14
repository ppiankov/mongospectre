# mongospectre

[![CI](https://github.com/ppiankov/mongospectre/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/mongospectre/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

MongoDB collection and index auditor. Scans codebases for collection/field references, compares with live MongoDB schema and statistics, detects drift.

## What This Is

A CLI tool that:

- Connects to MongoDB and fetches collection metadata, index definitions, and usage statistics
- Scans code repositories for MongoDB collection references (Go, Python, JS/TS, Java, C#, Ruby)
- Compares code references against live database to find missing collections, unused indexes, and drift
- Produces JSON or text audit reports

Part of the **Spectre** family — code-vs-reality drift detection tools.

## What This Is NOT

- Not a MongoDB monitoring tool (use `mongostat`/`mongotop`)
- Not a migration tool
- Not a query profiler or optimizer
- Not a backup or replication tool
- Does not modify any data — all queries are strictly read-only

## Quick Start

```bash
# Download latest release
curl -LO https://github.com/ppiankov/mongospectre/releases/latest/download/mongospectre_0.1.0_darwin_arm64.tar.gz
tar -xzf mongospectre_0.1.0_darwin_arm64.tar.gz
sudo mv mongospectre /usr/local/bin/

# Audit a cluster (no code scanning)
mongospectre audit --uri "mongodb://localhost:27017"

# Check code repo against live cluster
mongospectre check --repo ./my-app --uri "mongodb://localhost:27017"

# JSON output for CI pipelines
mongospectre audit --uri "mongodb://..." --format json
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
mongospectre audit --uri "mongodb://..." [--database mydb] [--format json|text]
```

### `check` — Code + Cluster Diff

Scans a code repository and compares collection references against live MongoDB:

| Finding | Severity | Description |
|---------|----------|-------------|
| `MISSING_COLLECTION` | high | Referenced in code, doesn't exist in DB |
| `UNUSED_COLLECTION` | medium | Exists in DB with 0 docs, not in code |
| `ORPHANED_INDEX` | low | Unused index on unreferenced collection |
| `OK` | info | Collection exists and is referenced |

```bash
mongospectre check --repo ./app --uri "mongodb://..." [--database mydb] [--format json|text] [--fail-on-missing]
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No issues or low/info only |
| 1 | Medium severity findings |
| 2 | High severity findings |

## Architecture

```
cmd/mongospectre/main.go   — CLI entry point
internal/cli/              — Cobra commands (audit, check)
internal/mongo/            — MongoDB inspector (read-only queries)
internal/scanner/          — Code repo collection reference scanner
internal/analyzer/         — Detection engines (audit + diff)
internal/reporter/         — JSON/text report output
```

### Supported Languages

The code scanner detects MongoDB collection references in:

- **Go** — `db.Collection("x")`
- **JavaScript/TypeScript** — `db.collection("x")`, `db.getCollection("x")`
- **Python** — `db["x"]`, `db.x.find(...)`, PyMongo, MongoEngine
- **Mongoose** — `mongoose.model("X", schema)`
- **Java/C#** — `GetCollection("x")`

## Building from Source

```bash
git clone https://github.com/ppiankov/mongospectre.git
cd mongospectre
make build    # produces bin/mongospectre
make test     # run tests with -race
make lint     # golangci-lint
```

## Known Limitations

- Collection references using variables (`db.Collection(collName)`) are not detected
- PyMongo dot access (`db.users.find`) requires a known operation suffix to avoid false positives
- `$indexStats` requires MongoDB 3.2+ and may not be available on all hosting providers
- No support for aggregation pipeline field analysis (planned)

## Roadmap

- [ ] Aggregation pipeline field extraction
- [ ] Configuration file for custom patterns and thresholds
- [ ] SpectreHub integration for centralized drift dashboards
- [ ] Watch mode for CI/CD integration

## License

[MIT](LICENSE)
