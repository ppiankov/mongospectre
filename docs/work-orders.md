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

## Non-Goals

- No schema enforcement or migrations
- No data modification or deletion
- No aggregation pipeline analysis (future)
- No sharding management
- No web UI
