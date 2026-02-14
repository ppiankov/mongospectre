# Work Orders — mongospectre

## WO-01: Project Scaffold

**Goal:** Create Go project structure matching Spectre family conventions.

### Steps
1. `go mod init github.com/ppiankov/mongospectre`
2. Create `cmd/mongospectre/main.go` — minimal, delegates to `internal/cli`
3. Create `internal/cli/root.go` — Cobra root with version, `--uri` persistent flag
4. Create `Makefile` — build, test, lint, fmt, vet, clean (copy pattern from kafkaspectre)
5. Add `.github/workflows/ci.yml` and `release.yml` from claude-skills templates
6. Add `.gitignore` matching other spectre tools

### Acceptance
- `make build` produces `bin/mongospectre`
- `./bin/mongospectre version` prints version
- `make test` passes (even with no tests yet)

---

## WO-02: MongoDB Inspector

**Goal:** Connect to MongoDB and fetch collection metadata + index usage statistics.

### Details
Create `internal/mongo/` package:
- `inspector.go` — connect via mongo-driver/v2, query metadata
- `types.go` — CollectionInfo, IndexInfo, Config structs

### Metadata Queries (all read-only)
- Collections: `db.listCollections()` (name, type, options)
- Collection stats: `db.collection.stats()` (count, size, avgObjSize, storageSize)
- Indexes: `db.collection.getIndexes()` (key, name, unique, sparse)
- Index stats: `$indexStats` aggregation (accesses.ops, accesses.since)
- Server status: `db.serverStatus()` for version info

### Acceptance
- Connects to MongoDB with `--uri mongodb://...`
- Fetches metadata without requiring admin role
- Handles connection errors and auth failures gracefully
- Supports both standalone and replica set

---

## WO-03: Audit Command

**Goal:** Cluster-only analysis — find problems without code scanning.

### Detections
- **Unused collections**: `count = 0` or no operations in `$indexStats`
- **Unused indexes**: `accesses.ops = 0` (never queried)
- **Missing indexes**: collections with high document count but only `_id` index
- **Duplicate indexes**: overlapping key patterns on same collection
- **Oversized collections**: collections > threshold without sharding
- **Missing TTL**: collections with timestamp fields but no TTL index

### Steps
1. Create `internal/cli/audit.go` — Cobra `audit` subcommand
2. Create `internal/analyzer/audit.go` — detection logic
3. Risk scoring: high (missing collections), medium (unused indexes), low (missing TTL)
4. Reporter: JSON and text output

### Acceptance
- `mongospectre audit --uri mongodb://...` produces report
- `--format json|text` flag
- `--database` flag to scope to specific DB (default: all non-system DBs)
- Exit code reflects severity
- `make test` passes with -race

---

## WO-04: Code Scanner

**Goal:** Scan code repo for MongoDB collection/field references.

### Details
Create `internal/scanner/` package:
- `collection_scanner.go` — detect collection names from driver calls
- `query_scanner.go` — extract field names from query patterns
- `orm_scanner.go` — detect Mongoose/Motor/MongoEngine model definitions

### Patterns to Detect
- `db.collection("users")`, `db["users"]`
- `db.GetCollection("orders")`
- Mongoose: `mongoose.model("User", schema)`
- PyMongo: `db.users.find({"status": ...})`
- Go: `collection := db.Collection("products")`

### Acceptance
- Scans a Go/Python/JS project and finds collection references
- `make test` passes with -race

---

## WO-05: Check Command (Code + Cluster Diff)

**Goal:** Compare code repo references against live MongoDB.

### Detections
- **MISSING_COLLECTION**: referenced in code, doesn't exist in DB
- **UNUSED_COLLECTION**: exists in DB, not referenced in code, no operations
- **MISSING_INDEX**: code queries on fields that have no index
- **ORPHANED_INDEX**: index exists but no code queries that field pattern
- **OK**: everything matches

### Steps
1. Create `internal/cli/check.go` — Cobra `check` subcommand
2. Create `internal/analyzer/diff.go` — comparison engine
3. Add `--repo`, `--fail-on-missing` flags

### Acceptance
- `mongospectre check --repo ./app --uri mongodb://...` produces report
- JSON output compatible with spectrehub contract

---

## WO-06: Tests and Release v0.1.0

**Goal:** Full test suite and tagged release.

### Steps
1. Unit tests for inspector (mock mongo client), analyzer, scanner, reporter
2. Coverage >85% on analyzer, scanner, reporter
3. GoReleaser config — linux/darwin/windows, amd64/arm64
4. README: description, install, usage, architecture, license
5. Tag v0.1.0

### Acceptance
- `make test` passes with -race
- `make lint` clean
- `gh release list` shows v0.1.0
- spectrehub can ingest mongospectre JSON output

---

## WO-07: Field-Level Query Scanning

**Goal:** Detect queried fields from code and cross-reference with existing indexes.

### Details
Create `internal/scanner/query_scanner.go`:
- Extract field names from query/filter objects in driver calls
- Go: `bson.M{"status": ..., "created_at": ...}`, `bson.D{{Key: "status", ...}}`
- JS/TS: `.find({status: ..., email: ...})`, `.findOne({_id: ...})`
- Python: `.find({"status": ...})`, `.aggregate([{"$match": {"field": ...}}])`
- Mongoose: schema field definitions (`new Schema({name: String, email: String})`)

### Integration
- Add `FieldRef` type to scanner (collection, field, file, line)
- Extend `analyzer.Diff` to compare queried fields against index key patterns
- New finding: `UNINDEXED_QUERY` — code queries a field that has no covering index

### Acceptance
- `mongospectre check --repo ./app --uri ...` reports unindexed query patterns
- Tests cover Go, JS, Python query patterns
- Coverage >85% on query scanner
- `make test && make lint` clean

---

## WO-08: Config File

**Goal:** Support `.mongospectre.yml` for persistent configuration.

### Config Schema
```yaml
uri: mongodb://localhost:27017
database: myapp
thresholds:
  oversized_docs: 1000000      # default: 1M
  index_usage_days: 30         # flag unused if no ops in N days
exclude:
  collections:
    - "system.*"
    - "migrations"
  databases:
    - "local"
    - "config"
defaults:
  format: json
  verbose: false
  timeout: 30s
```

### Steps
1. Load config from `.mongospectre.yml` in CWD, then `~/.mongospectre.yml`
2. CLI flags override config file values
3. Environment variables override config file (MONGODB_URI already works)
4. Precedence: flags > env > config file > defaults

### Acceptance
- Config file is auto-detected from CWD
- All threshold values are configurable
- `mongospectre audit` works with zero flags if config file provides URI
- `make test && make lint` clean

---

## WO-09: Baseline and Ignore File

**Goal:** Allow suppressing known-ok findings so reports stay actionable.

### Details
Create `.mongospectreignore` file format:
```
# Ignore unused index on legacy collection
UNUSED_INDEX app.legacy_users.idx_old_email

# Ignore all findings for a collection
* app.audit_logs

# Ignore missing TTL on config collection
MISSING_TTL app.settings
```

### Steps
1. Parse `.mongospectreignore` from CWD (one rule per line, `#` comments)
2. Match rules against findings by type, database, collection, index
3. Support glob patterns (`*` for any type, `app.*` for any collection in db)
4. Suppressed findings still appear in `--verbose` output but not in report
5. Add `--no-ignore` flag to bypass ignore file

### Acceptance
- Findings matching ignore rules are excluded from report and exit code
- `--verbose` shows suppressed count
- Invalid ignore rules produce warnings, not errors
- `make test && make lint` clean

---

## WO-10: SARIF Output

**Goal:** Emit SARIF format for GitHub Security tab integration.

### Details
- SARIF v2.1.0 schema (same as Trivy, CodeQL)
- Map finding types to SARIF `reportingDescriptor` rules
- Map severities to SARIF levels: high→error, medium→warning, low→note, info→none
- For `check` command: include file locations from scanner refs as SARIF `location` objects
- For `audit` command: use logical locations (database.collection)

### Steps
1. Add `--format sarif` to audit and check commands
2. Create `internal/reporter/sarif.go` — SARIF writer
3. Update CI workflow example in README

### Acceptance
- `mongospectre check --format sarif` produces valid SARIF 2.1.0
- SARIF uploads successfully via `codeql-action/upload-sarif`
- `make test && make lint` clean

---

## WO-11: Integration Tests

**Goal:** Test MongoDB inspector against a real instance using testcontainers.

### Details
- Use `testcontainers-go` to spin up MongoDB 7.x in tests
- Test full inspector lifecycle: connect, list databases, list collections, get stats, get indexes, get index stats, get server version
- Test against both standalone and replica set configurations
- Guard with `//go:build integration` tag so `make test` skips them by default

### Steps
1. Add `testcontainers-go` to dev dependencies
2. Create `internal/mongo/inspector_integration_test.go`
3. Seed test data: create collections, insert documents, create indexes
4. Verify all inspector methods return correct data
5. Add `make test-integration` target to Makefile

### Acceptance
- `make test-integration` passes with a real MongoDB
- Inspector coverage reaches >80%
- Tests are isolated (each test gets a fresh container)
- `make test` still works without Docker

---

## WO-12: Index Suggestions

**Goal:** Recommend indexes based on observed query patterns from code scanning.

### Details
Requires WO-07 (field-level scanning) as prerequisite.

### Logic
- Group queried fields by collection
- For each collection, identify field combinations that appear together in queries
- Check if an existing index covers the combination (prefix match)
- If not, suggest a compound index with the most selective fields first
- Never suggest indexes that duplicate existing ones

### Output
New finding type: `SUGGEST_INDEX` with severity `info`
```
[INFO] app.orders — consider index {status:1, created_at:-1} to cover 3 query patterns
```

### Constraints
- Suggestions are advisory only — never auto-create indexes
- Limit to top 5 suggestions per collection to avoid noise
- Skip collections with <1000 documents (indexes not worth it)

### Acceptance
- `mongospectre check` includes index suggestions when field scanning is available
- Suggestions don't duplicate existing indexes
- `make test && make lint` clean

---

## WO-13: Diff Against Baseline

**Goal:** Compare current run against a previous report to show new/resolved findings.

### Details
- Save a report as baseline: `mongospectre audit --format json > baseline.json`
- Compare: `mongospectre audit --baseline baseline.json`
- Output shows: new findings (not in baseline), resolved (in baseline but not current), unchanged

### Steps
1. Add `--baseline` flag to audit and check commands
2. Create `internal/analyzer/baseline.go` — load and diff reports
3. Match findings by (type, database, collection, index) tuple
4. Add status field to findings in diff mode: `new`, `resolved`, `unchanged`
5. Text output uses `+`/`-` prefixes for new/resolved

### Acceptance
- New findings are highlighted in output
- Resolved findings show as resolved (not silently dropped)
- Exit code reflects only current findings (not resolved ones)
- `make test && make lint` clean

---

## WO-14: Multi-Cluster Comparison

**Goal:** Compare schemas across environments (e.g., staging vs production).

### Details
- Compare two MongoDB clusters to detect environment drift
- New subcommand: `mongospectre compare --source <uri> --target <uri>`

### Detections
- **MISSING_IN_TARGET**: collection/index exists in source but not target
- **MISSING_IN_SOURCE**: collection/index exists in target but not source
- **INDEX_DRIFT**: same collection has different indexes across environments
- **SCHEMA_DRIFT**: field presence differs (based on sampled documents)

### Steps
1. Create `internal/cli/compare.go` — Cobra `compare` subcommand
2. Create `internal/analyzer/compare.go` — cross-cluster diff engine
3. Add `--source-db`, `--target-db` flags for scoping
4. Reuse inspector to fetch metadata from both clusters

### Constraints
- Both connections are read-only
- Document sampling for schema comparison limited to 100 docs per collection
- No data transfer between clusters

### Acceptance
- `mongospectre compare --source mongodb://staging --target mongodb://prod` produces diff
- Handles clusters with different database names via `--source-db`/`--target-db`
- `make test && make lint` clean

---

## Non-Goals

- No schema enforcement or migrations
- No data modification or deletion
- No sharding management
- No web UI
