# Work Orders — mongospectre

## WO-01: Project Scaffold ✅

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

## WO-02: MongoDB Inspector ✅

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

## WO-03: Audit Command ✅

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

## WO-04: Code Scanner ✅

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

## WO-05: Check Command (Code + Cluster Diff) ✅

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

## WO-06: Tests and Release v0.1.0 ✅

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

## WO-07: Field-Level Query Scanning ✅

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

## WO-08: Config File ✅

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

## WO-09: Baseline and Ignore File ✅

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

## WO-10: SARIF Output ✅

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

## WO-11: Integration Tests ✅

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

## WO-12: Index Suggestions ✅

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

## WO-13: Diff Against Baseline ✅

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

## WO-14: Multi-Cluster Comparison ✅

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

## WO-15: Wire SARIF format to CLI flags ✅

**Goal:** Fix bug — SARIF reporter exists in `internal/reporter/` but CLI `--format` flag only accepts `text` and `json`.

### Steps
1. Add `sarif` to `--format` flag choices in `internal/cli/audit.go`, `internal/cli/check.go`, `internal/cli/compare.go`
2. Pass `sarif` format through to `reporter.Write()`
3. Update `--help` text to show `text|json|sarif`

### Acceptance
- `mongospectre audit --format sarif` produces valid SARIF 2.1.0
- `mongospectre check --format sarif` works
- `make test` passes with -race

---

## WO-16: Unit test coverage to 85% ✅

**Goal:** Bring unit test coverage from 14% to >85% on core packages.

Current state: only integration tests exist for `internal/mongo/`. Core packages (`analyzer`, `scanner`, `reporter`) need mock-based unit tests.

### Packages to cover
- `internal/mongo/` — mock mongo client for inspector error paths, empty results, malformed data
- `internal/analyzer/` — audit detection logic, diff engine, severity scoring
- `internal/scanner/` — collection/field extraction from Go/JS/Python code patterns
- `internal/reporter/` — JSON, text, SARIF output formatting

### Acceptance
- `make test` coverage >85% on analyzer, scanner, reporter
- `make test` passes with -race
- No flaky tests — all deterministic with mocks

---

## WO-17: CLI tests ⚠️ (50% — ceiling without MongoDB)

**Goal:** Test CLI layer — flag validation, error handling, exit codes.

Current state: 50% coverage on `internal/cli/`. Remaining 50% is behind MongoDB connections — requires WO-26 (podman integration tests) to reach 70%+ target.

### Test cases
- Invalid `--format` value returns error
- Missing `--uri` returns helpful error
- `--database` flag scopes correctly
- Exit code 0 when no findings, 1 for medium, 2 for high
- `--version` prints version string
- `--help` shows all subcommands

### Files
- `internal/cli/root_test.go`
- `internal/cli/audit_test.go`
- `internal/cli/check_test.go`

### Acceptance
- CLI coverage >70%
- `make test` passes with -race

---

## WO-18: Run integration tests in CI ✅

**Goal:** Wire `make test-integration` into CI workflow.

`make test-integration` exists but CI only runs `make test`. Integration tests need a MongoDB service container.

### Steps
1. Add MongoDB service container to `.github/workflows/ci.yml`
2. Add `test-integration` step after unit tests
3. Set `MONGODB_TEST_URI` env var pointing to service container

### Acceptance
- CI runs both `make test` and `make test-integration`
- Integration tests pass in CI with MongoDB 7.x service

---

## WO-19: Aggregation Pipeline Field Extraction ✅

**Goal:** Detect queried/projected fields from aggregation pipeline stages in code.

### Details
Extend `internal/scanner/query_scanner.go` to extract fields from:
- `$match` — filter fields (already partially handled for simple cases)
- `$group` — `_id` field and accumulator source fields
- `$project` / `$addFields` — included/excluded field names
- `$lookup` — `localField`, `foreignField`, `from` (as collection ref)
- `$sort` — sorted field names
- `$unwind` — unwound field path

### Patterns
```javascript
// JS/Python
db.orders.aggregate([
  { $match: { status: "active" } },
  { $lookup: { from: "users", localField: "userId", foreignField: "_id" } },
  { $group: { _id: "$category", total: { $sum: "$amount" } } },
  { $sort: { total: -1 } }
])
```
```go
// Go
pipeline := mongo.Pipeline{
    bson.D{{Key: "$match", Value: bson.M{"status": "active"}}},
    bson.D{{Key: "$group", Value: bson.M{"_id": "$region"}}},
}
```

### Steps
1. Add regex patterns for `$lookup.from` as a collection reference
2. Extract field names from `$match`, `$group._id`, `$sort` stages
3. Feed extracted fields into existing `FieldRef` pipeline for index analysis
4. `$lookup.from` should produce a `CollectionRef` (cross-collection dependency)

### Acceptance
- `mongospectre check` detects fields from aggregate pipelines
- `$lookup.from` appears as a collection reference
- Tests cover JS, Python, Go pipeline patterns
- `make test && make lint` clean

---

## WO-20: SpectreHub Integration Contract ✅

**Goal:** Define and implement a stable JSON output schema for cross-tool ingestion.

### Details
SpectreHub aggregates findings from multiple Spectre tools (mongospectre, kafkaspectre, etc). Each tool must emit a common envelope format so SpectreHub can ingest without per-tool parsing.

### Schema
```json
{
  "schema": "spectre/v1",
  "tool": "mongospectre",
  "version": "0.2.0",
  "timestamp": "2026-02-15T00:00:00Z",
  "target": {
    "type": "mongodb",
    "uri_hash": "sha256:...",
    "database": "app"
  },
  "findings": [
    {
      "id": "UNUSED_INDEX",
      "severity": "medium",
      "location": "app.users.idx_old",
      "message": "index has never been used",
      "metadata": {}
    }
  ],
  "summary": { "total": 1, "high": 0, "medium": 1, "low": 0, "info": 0 }
}
```

### Steps
1. Define the `spectre/v1` envelope in `internal/reporter/spectrehub.go`
2. Add `--format spectrehub` to audit and check commands
3. Hash the URI (never include credentials in output)
4. Document the schema in `docs/spectrehub-schema.md`

### Acceptance
- `mongospectre audit --format spectrehub` produces valid envelope
- URI credentials are never present in output
- Schema is documented
- `make test && make lint` clean

---

## WO-21: Watch Mode ✅

**Goal:** Continuously monitor a MongoDB cluster and report drift as it happens.

### Details
New subcommand: `mongospectre watch` — runs `audit` on a configurable interval, compares each run against the previous, and prints only new/resolved findings.

### Behavior
- First run: full audit, prints all findings, stores as baseline in memory
- Subsequent runs: diff against previous, print only `+ [new]` and `- [resolved]`
- On finding change: print timestamp + diff line
- On no change: print nothing (quiet) or a heartbeat in verbose mode
- Ctrl+C: print final summary and exit cleanly

### Flags
- `--interval` — time between runs (default `5m`)
- `--uri`, `--database`, `--verbose`, `--timeout` — inherited from root
- `--format` — `text` (default) or `json` (NDJSON, one event per line)
- `--exit-on-new` — exit with code 2 on first new high-severity finding (for CI)

### Steps
1. Create `internal/cli/watch.go` — Cobra `watch` subcommand
2. Run `audit` in a loop with `time.Ticker`
3. Use `analyzer.DiffBaseline` to compare current vs previous findings
4. Use `reporter.WriteBaselineDiff` for text output
5. Handle SIGINT for clean shutdown

### Acceptance
- `mongospectre watch --uri mongodb://... --interval 1m` runs continuously
- Only new/resolved findings are printed after first run
- Ctrl+C prints summary and exits 0
- `--exit-on-new` exits on first new high finding
- `make test && make lint` clean

---

## WO-22: Rich Version Output ✅

**Goal:** Include build metadata in version output for debugging and support.

### Details
Current: `mongospectre dev`
Target: `mongospectre 0.2.0 (commit: abc1234, built: 2026-02-15T12:00:00Z, go: go1.25.0)`

### Steps
1. Add `commit` and `date` variables to `cmd/mongospectre/main.go` via ldflags
2. Update `Makefile` LDFLAGS to inject `main.commit` and `main.date`
3. Update `.goreleaser.yml` ldflags to match
4. Update version command to print all metadata
5. Add `--json` flag to version command for machine-readable output

### Acceptance
- `mongospectre version` shows version, commit, date, Go version
- `mongospectre version --json` outputs JSON
- GoReleaser injects correct values on release
- `make test && make lint` clean

---

## WO-23: Coverage Reporting in CI ✅

**Goal:** Upload test coverage to Codecov and display badge in README.

### Steps
1. Add `coverprofile` flag to `make test` in CI
2. Add Codecov upload step to CI workflow
3. Add coverage badge to README
4. Set coverage threshold in `codecov.yml` (target: 85%)

### Acceptance
- CI uploads coverage on every push
- README displays coverage badge
- Coverage drop below 85% fails the PR check
- `make test && make lint` clean

---

## WO-24: Benchmarks ✅

**Goal:** Add performance benchmarks and regression tracking.

### Details
Add benchmarks for the hot paths: scanner regex matching, analyzer diff engine, reporter serialization.

### Steps
1. Add `Benchmark*` functions to scanner, analyzer, reporter test files
2. Add `make bench` target to Makefile: `go test -bench=. -benchmem ./internal/...`
3. Benchmark scanner with large files (10k lines)
4. Benchmark analyzer with 1000 collections, 50 indexes each
5. Benchmark reporter JSON/text/SARIF with 500 findings

### Acceptance
- `make bench` runs all benchmarks
- Benchmarks are deterministic (no flaky timing)
- Results include allocations (`-benchmem`)
- `make test && make lint` clean

---

## WO-25: Update README ✅

**Goal:** Update README to reflect current features and version.

### Steps
1. Update Quick Start to reference latest release (not hardcoded v0.1.0)
2. Add `compare` and `watch` subcommands to usage section
3. Document `--format sarif` and SARIF upload workflow example
4. Document `.mongospectre.yml` config file format
5. Document `.mongospectreignore` file format
6. Add shell completion instructions (`mongospectre completion bash`)
7. Update architecture diagram to include new packages (config, compare)

### Acceptance
- README reflects all implemented features
- No references to old versions
- Config and ignore file formats are documented
- `make test && make lint` clean (no code changes, but verify)

---

---

## Phase 2: Security Audit + Test Infrastructure

---

## WO-26: CLI Integration Tests with MongoDB in CI ✅

**Goal:** Run CLI commands against a real MongoDB to cover the 50% of CLI code behind connections.

### Problem
CLI coverage is capped at 50% — all code past `NewInspector()` (audit analysis, report writing, ignore/baseline filtering, exit codes, compare formatting, watch baseline diffing) is unreachable without a live MongoDB.

### Approach
Two execution modes:
- **CI (primary)**: GitHub Actions workflow uses `services:` to spin up `mongo:7` as a service container. Tests read `MONGODB_TEST_URI` env var. No podman/docker needed in the test code itself.
- **Local (optional)**: Developer runs `podman run -d -p 27017:27017 mongo:7` manually, then `MONGODB_TEST_URI=mongodb://localhost:27017 make test-cli-integration`.

Guard with `//go:build integration` tag so `make test` never requires MongoDB.

### Steps
1. Create `internal/cli/cli_integration_test.go` guarded by build tag
2. Read `MONGODB_TEST_URI` from env (fail fast if unset)
3. Seed test data: create collections, indexes, insert docs with known patterns
4. Test audit end-to-end: connect → inspect → analyze → report (text, json, sarif, spectrehub)
5. Test check end-to-end: scan repo + audit → diff → report
6. Test compare: seed two databases, compare them
7. Test watch: run with `--interval 1s --exit-on-new`, verify baseline diff
8. Test ignore file: create `.mongospectreignore`, verify suppression
9. Test baseline: save JSON report, re-run with `--baseline`, verify diff output
10. Add `test-cli-integration` job to `.github/workflows/ci.yml` with `services: mongodb`
11. Add `make test-cli-integration` target: `go test -race -tags integration ./internal/cli/`

### CI Workflow Addition
```yaml
test-cli-integration:
  runs-on: ubuntu-latest
  services:
    mongodb:
      image: mongo:7
      ports:
        - 27017:27017
  env:
    MONGODB_TEST_URI: mongodb://localhost:27017
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
    - run: make test-cli-integration
```

### Acceptance
- CLI coverage >70% with integration tests
- CI runs integration tests automatically via service container
- `make test` still works without MongoDB (build tag)
- Local dev can run with `podman run` + env var
- No flaky timing — use generous timeouts, poll for readiness

---

## WO-27: Auth Database User Audit ✅

**Goal:** Detect risky MongoDB user configurations — the auth db / data db split, forgotten admin users, and weak credentials.

### Problem
MongoDB users live in three places, often simultaneously:
1. **`admin` database** — authenticates against admin, used for cluster-wide ops
2. **Application database** (e.g., `myapp`) — authenticates against the data db directly
3. **Both** — same username created in admin AND data db (common misconfiguration)

The hairy scenarios:
- A second user is created during initial setup, given `root` or `dbOwner` role "temporarily", then forgotten
- That forgotten user has a simple password (`admin123`, `mongodb`, `password`) because it was "just for testing"
- The user in the data db has admin-level privileges when it only needs `readWrite`
- Nobody audits who has access because `db.getUsers()` is per-database and easy to miss

### Detections
- **ADMIN_IN_DATA_DB**: user with `dbAdmin`, `dbOwner`, or `root` role exists in a non-admin database
- **DUPLICATE_USER**: same username exists in both `admin` db and application db (auth confusion risk)
- **OVERPRIVILEGED_USER**: user has `root`, `dbOwner`, `userAdminAnyDatabase`, or `clusterAdmin` but only uses `readWrite` operations
- **STALE_USER**: user exists but has no `authenticationRestrictions` and no recent auth activity (if `$currentOp`/audit log available)
- **MULTIPLE_ADMIN_USERS**: more than one user with cluster-admin roles (common "forgot the second one" scenario)

### Inspector Changes
1. Add `InspectUsers(ctx, database)` to `internal/mongo/inspector.go`
2. Query `db.getUsers()` on `admin` db and each application db
3. Return `UserInfo{Username, Database, Roles[], Mechanisms[], CustomData, AuthRestrictions}`
4. Query `admin.system.users` for cross-database user enumeration

### Analyzer Changes
1. Create `internal/analyzer/users.go` — user audit rules
2. Cross-reference users across databases (admin vs app db)
3. Flag overprivileged roles relative to actual collection operations
4. Flag duplicate usernames across auth sources

### CLI Changes
1. Add `--audit-users` flag to `audit` command (opt-in — requires userAdmin role to list users)
2. User findings appear alongside collection findings in all output formats

### Constraints
- **Read-only**: never modify users, passwords, or roles
- **No password testing**: do NOT attempt to authenticate with common passwords — that's intrusion, not auditing. Report the risk pattern (admin role + data-db auth source) and let the user decide
- **Opt-in**: `--audit-users` is required because `getUsers` needs elevated privileges

### Acceptance
- `mongospectre audit --audit-users --uri ...` reports user configuration risks
- Duplicate users across admin/app databases are detected
- Overprivileged users in non-admin databases are flagged
- Multiple cluster-admin users are reported
- Tests cover all detection patterns (mock user data)
- `make test && make lint` clean

---

## WO-28: Multi-Line Query Detection ✅

**Goal:** Detect collection references and query patterns that span multiple lines.

### Problem
Current scanner uses line-by-line regex. Multi-line patterns are missed:
```go
coll := db.Collection(
    "users",
)
```
```python
db.get_collection(
    name="orders"
)
```

### Steps
1. Pre-join continuation lines before regex matching in scanner
2. Handle Go backtick strings, Python triple-quotes, JS template literals
3. Handle chained method calls split across lines

### Acceptance
- Multi-line `db.Collection(...)` calls detected in Go, Python, JS
- No false positives from comments or strings
- `make test && make lint` clean

---

## WO-29: Variable Collection Name Tracking ✅

**Goal:** Detect when collection names come from variables instead of string literals.

### Problem
```go
const usersCollection = "users"
db.Collection(usersCollection)
```
Current scanner only finds string literals. Variable references are missed entirely.

### Steps
1. Track `const` and `var` assignments of string literals in Go
2. Track Python module-level string assignments
3. Track JS `const`/`let` string assignments
4. Resolve variable references in collection calls to their string values

### Constraints
- Only resolve single-hop assignments (const → use). No dataflow analysis.
- Skip dynamic/computed names — report them as `DYNAMIC_COLLECTION` info finding instead

### Acceptance
- `const coll = "users"; db.Collection(coll)` resolves to "users"
- Dynamic names produce an info-level finding
- `make test && make lint` clean

---

## WO-30: Docker / CI Integration Examples ✅

**Goal:** Provide ready-to-use CI examples for GitHub Actions, GitLab CI, and Makefile targets.

### Steps
1. Add `docs/ci-examples.md` with GitHub Actions workflow (mongo service + audit)
2. Add GitLab CI example with mongo service
3. Add Makefile targets: `make audit`, `make check`, `make compare`
4. Document `--format sarif` upload to GitHub Security tab
5. Document `--exit-on-new` for CI gating

### Acceptance
- GitHub Actions example works with copy-paste
- SARIF upload to GitHub Security tab documented
- `make audit URI=mongodb://...` works

---

## Phase 3: Distribution + Adoption (v0.2.x)

---

## WO-31: Docker Image ✅

**Goal:** Publish a container image so users can run mongospectre without installing Go or downloading binaries.

**Details:**
- `docker run ghcr.io/ppiankov/mongospectre audit --uri mongodb://host.docker.internal:27017`
- Multi-arch: linux/amd64, linux/arm64
- Minimal base image: `gcr.io/distroless/static-debian12` (no shell, no package manager, ~2MB base)
- Build in release workflow alongside binary artifacts
- Tag strategy: `latest`, semver (`v0.2.0`), git SHA

**Steps:**
1. Create `Dockerfile` — multi-stage build (Go builder → distroless)
2. Add `docker-build` and `docker-push` to Makefile
3. Add container build + push to `.github/workflows/release.yml`
4. Add `docker-compose.yml` for local testing (mongospectre + mongo:7 sidecar)
5. Update README install section with `docker run` example

**Dockerfile:**
```dockerfile
FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /mongospectre ./cmd/mongospectre

FROM gcr.io/distroless/static-debian12
COPY --from=builder /mongospectre /mongospectre
ENTRYPOINT ["/mongospectre"]
```

**Acceptance:**
- `docker run ghcr.io/ppiankov/mongospectre version` works
- Image size < 20MB
- Multi-arch manifest published on release
- `make test && make lint` clean

---

## WO-32: Homebrew Formula ✅

**Goal:** `brew install ppiankov/tap/mongospectre` for macOS and Linux users.

**Steps:**
1. Create `ppiankov/homebrew-tap` repo (if not exists)
2. Add `Formula/mongospectre.rb` — download from GitHub release, verify SHA256
3. Add GoReleaser homebrew tap config to `.goreleaser.yml`
4. Auto-publish formula on release via GoReleaser
5. Update README with `brew install` instructions

**Formula template:**
```ruby
class Mongospectre < Formula
  desc "MongoDB collection and index auditor"
  homepage "https://github.com/ppiankov/mongospectre"
  # GoReleaser fills in url + sha256 automatically
end
```

**Acceptance:**
- `brew install ppiankov/tap/mongospectre && mongospectre version` works
- Formula auto-updates on new releases
- README shows brew install as primary macOS method

---

## WO-33: GitHub Action ✅

**Goal:** Reusable GitHub Action so teams add mongospectre to CI in one YAML block instead of the multi-step setup in ci-examples.md.

**Usage:**
```yaml
- uses: ppiankov/mongospectre-action@v1
  with:
    command: check
    uri: ${{ secrets.MONGODB_URI }}
    repo: .
    format: sarif
    sarif-upload: true
```

**Steps:**
1. Create `ppiankov/mongospectre-action` repo
2. `action.yml` — composite action that downloads binary + runs command
3. Inputs: `command`, `uri`, `repo`, `format`, `database`, `fail-on-missing`, `audit-users`, `sarif-upload`
4. Auto-upload SARIF when `sarif-upload: true` and `format: sarif`
5. Set exit code from mongospectre (0/1/2 pass through to CI)
6. Cache binary between runs using actions/cache

**Acceptance:**
- Action works in a test workflow against a mongo:7 service
- SARIF auto-upload to GitHub Security tab works
- Exit codes propagate correctly for CI gating

---

## WO-34: Example Configs ✅

**Goal:** Copy-paste-ready config files so users don't have to read docs to get started.

**Steps:**
1. Create `docs/examples/.mongospectre.yml` — annotated config with all options
2. Create `docs/examples/.mongospectreignore` — common ignore patterns
3. Create `docs/examples/docker-compose.yml` — mongospectre + mongo:7 for local testing
4. Add `mongospectre init` command that copies example configs to CWD

**`docs/examples/.mongospectre.yml`:**
```yaml
# MongoDB connection URI (also settable via MONGODB_URI env or --uri flag)
uri: mongodb://localhost:27017

# Scope to specific database (omit to scan all non-system databases)
# database: myapp

# Analyzer thresholds
thresholds:
  oversized_docs: 1000000      # collections > 1M docs flagged as oversized
  index_usage_days: 30         # indexes with 0 ops in N days flagged as unused

# Exclude patterns (databases and collections that should never be reported)
exclude:
  databases:
    - local
    - config
    - admin
  collections:
    - "system.*"
    - "*.migrations"

# Default flag values
defaults:
  format: text
  verbose: false
  timeout: 30s
```

**`docs/examples/.mongospectreignore`:**
```
# Suppress known-OK findings — one rule per line
# Format: FINDING_TYPE database.collection[.index]

# Legacy indexes kept for rollback compatibility
UNUSED_INDEX myapp.users.idx_old_email
UNUSED_INDEX myapp.orders.idx_legacy_status

# System collections managed by frameworks
* myapp.schema_migrations
* myapp.__sessions

# Collections with intentionally no indexes beyond _id
MISSING_INDEX myapp.audit_log
```

**Acceptance:**
- Example configs are valid and parseable
- `mongospectre init` creates `.mongospectre.yml` and `.mongospectreignore` in CWD
- README links to examples

---

## WO-35: First-Run Experience ✅

**Goal:** Make `mongospectre audit --uri mongodb://localhost:27017` useful on first run, even against an unfamiliar database.

**Problem:** Currently, a first-time user points mongospectre at their cluster and gets a wall of findings with no guidance on what to fix first. Or worse — an empty database returns nothing and the user thinks the tool is broken.

**Improvements:**
1. **Summary header** — before findings, print a 3-line summary: databases scanned, collections found, indexes found, total findings by severity
2. **Empty database message** — if no collections found, print "No collections found in database X. If this is unexpected, check --database flag or URI." instead of silent exit
3. **First finding hint** — on first `[HIGH]` finding, append a one-line hint: "Run with --verbose for details, or --format json to pipe to jq"
4. **Connection banner** — on verbose, print "Connected to MongoDB X.Y.Z at host:port (N databases, M collections)" before scanning
5. **Exit code explanation** — on non-zero exit, print "Exit code 2: high-severity findings detected. Use --baseline to track progress." to stderr

**Steps:**
1. Add summary header to text reporter in `internal/reporter/text.go`
2. Add empty-result message to audit/check CLI commands
3. Add connection banner to verbose path
4. Add exit code hint to stderr in CLI error handling

**Constraints:**
- JSON/SARIF/SpectreHub formats unchanged — hints only in text mode
- No behavioral changes — same findings, same exit codes

**Acceptance:**
- First run against a real cluster shows summary + findings + actionable hints
- Empty database produces a helpful message instead of silence
- `--format json` output is unchanged
- `make test && make lint` clean

---

---

## Phase 4: Advanced Analysis

---

## WO-36: Smart Index Recommendation Engine

**Goal:** Suggest optimal compound indexes by analyzing query patterns from code, not just flag unindexed fields.

### Problem
WO-12 adds basic `SUGGEST_INDEX` findings ("field X is queried but not indexed"). Real MongoDB performance tuning requires compound index design — field order matters, equality-sort-range (ESR) rule, covered queries, and avoiding redundant indexes.

### Detections
- **COMPOUND_INDEX_SUGGESTION**: recommend multi-field index based on co-occurring fields in queries
- **INDEX_ORDER_WARNING**: existing compound index has suboptimal field order (violates ESR rule)
- **REDUNDANT_INDEX**: index A is a prefix of index B (already partially covered by WO-03's DUPLICATE_INDEX, but this is smarter)
- **PARTIAL_COVERAGE**: index covers 2/3 fields in a query pattern — adding one more field would make it a covered query

### Logic
1. Group all `FieldRef`s by collection
2. Identify field sets that co-occur in the same query context (same function, same file, same `find`/`aggregate` call)
3. Apply ESR rule: equality fields first, sort fields second, range fields last
4. Check if existing indexes already cover the combination (prefix match)
5. Score recommendations by frequency (how many code locations use this pattern)
6. Deduplicate against existing indexes

### Output
```
[INFO] COMPOUND_INDEX_SUGGESTION app.orders — {status:1, created_at:-1, amount:1}
  Covers 5 query patterns across 3 files
  Replaces: status_1 (prefix), status_1_created_at_-1 (prefix)
```

### Constraints
- Never suggest more than 5 indexes per collection
- Never suggest indexes for collections with <1000 documents
- Recommendations are advisory only — never auto-create
- Skip if `$indexStats` is unavailable (some hosted providers block it)

### Acceptance
- `mongospectre check` produces compound index suggestions with ESR ordering
- Suggestions include which existing indexes they would replace
- Frequency-based scoring ranks suggestions by impact
- `make test && make lint` clean

---

## WO-37: Schema Sampling & Field-Level Drift Detection

**Goal:** Sample documents from each collection to detect field-level drift between code and live data.

### Problem
Current drift detection is collection-level only. Code might reference `user.address.zipCode` but the actual documents use `user.address.zip`. This is invisible to the current tool.

### Approach
1. Sample N documents per collection (default: 100, configurable)
2. Build a field frequency map: `{fieldPath: count}` for each collection
3. Compare code `FieldRef`s against the frequency map
4. Flag fields referenced in code but absent from sampled documents

### Detections
- **MISSING_FIELD**: code references `orders.shippingAddress` but 0/100 sampled docs have it
- **RARE_FIELD**: code references `users.middleName` but only 3/100 docs have it (nullable or new field)
- **UNDOCUMENTED_FIELD**: field exists in 95/100 docs but no code references it (potential dead data)
- **TYPE_INCONSISTENCY**: field `users.age` is `int` in 80 docs but `string` in 20 docs

### Inspector Changes
1. Add `SampleDocuments(ctx, database, collection, n)` to inspector
2. Return `FieldMap{Path: string, Count: int, Types: []string}`
3. Use `$sample` aggregation stage for random sampling
4. Recursively flatten nested documents to dot-notation paths

### Analyzer Changes
1. Create `internal/analyzer/schema.go` — field drift detection
2. Cross-reference `FieldRef`s from scanner against `FieldMap` from sampling
3. Configurable thresholds: `rare_field_threshold: 0.1` (10% presence = rare)

### CLI Changes
1. Add `--sample` flag to `check` command (default: 100, 0 to disable)
2. Schema findings appear alongside existing findings in all formats

### Constraints
- Read-only: uses `$sample`, never modifies data
- Sampling is approximate — not a guarantee of field presence
- Nested arrays are flattened with `[]` notation: `orders.items[].sku`
- Skip collections with 0 documents

### Acceptance
- `mongospectre check --repo ./app --uri ... --sample 100` reports field-level drift
- Missing, rare, and undocumented fields are detected
- Type inconsistencies flagged with percentages
- Sampling is configurable and can be disabled
- `make test && make lint` clean

---

## WO-38: Slow Query Correlation

**Goal:** Read MongoDB profiler data and correlate slow queries with code locations.

### Problem
MongoDB's profiler (`system.profile`) logs slow queries, but developers must manually match them to code. mongospectre already knows which code locations issue which queries — connecting the two identifies exactly where performance problems originate.

### Approach
1. Read `system.profile` collection (profiler level 1 or 2 must be enabled)
2. Extract query shapes (collection, filter fields, sort fields, projection)
3. Match profiler query shapes against code scanner `FieldRef`s
4. Report code locations that generate slow queries

### Inspector Changes
1. Add `ReadProfiler(ctx, database, limit)` to inspector
2. Return `ProfileEntry{Collection, FilterFields, SortFields, Duration, Timestamp, PlanSummary}`
3. Read from `db.system.profile` with limit (default: 1000 most recent)
4. Parse `command.filter`, `command.sort`, `command.projection` from profiler docs

### Detections
- **SLOW_QUERY_SOURCE**: code at `app/handlers/orders.go:42` generates queries averaging 850ms on `orders` collection
- **COLLECTION_SCAN_SOURCE**: code at `app/models/user.go:15` generates COLLSCAN (no index used) — links to profiler entry
- **FREQUENT_SLOW_QUERY**: same query shape appears 50+ times in profiler — high-impact optimization target

### CLI Changes
1. Add `--profile` flag to `check` command (reads profiler, default: off)
2. Add `--profile-limit` flag (default: 1000)

### Constraints
- Read-only: never modifies profiler settings
- Profiler must be enabled by the user (`db.setProfilingLevel(1)`)
- If profiler is disabled or empty, skip gracefully with a hint
- Never log query values — only field names and shapes

### Acceptance
- `mongospectre check --repo ./app --uri ... --profile` correlates slow queries to code
- COLLSCAN queries linked to source files
- Works when profiler is disabled (graceful skip)
- `make test && make lint` clean

---

## WO-39: Sharding Analysis

**Goal:** Detect sharding misconfigurations and recommend shard key improvements.

### Problem
Sharded clusters have unique failure modes: hot shard keys, jumbo chunks, unbalanced distributions, and collections that should be sharded but aren't. None of these are visible in current audit output.

### Inspector Changes
1. Add `InspectSharding(ctx)` to inspector
2. Query `config.shards` for shard list
3. Query `config.collections` for shard keys
4. Query `config.chunks` for chunk distribution
5. Query `sh.status()` equivalent for balancer state
6. Return `ShardInfo{Key, Chunks, Distribution, Balancer}`

### Detections
- **UNSHARDED_LARGE**: collection >10GB with no shard key — candidate for sharding
- **MONOTONIC_SHARD_KEY**: shard key is `{_id: 1}` or `{created_at: 1}` (monotonically increasing = hot shard)
- **UNBALANCED_CHUNKS**: one shard has >2x chunks compared to others
- **JUMBO_CHUNKS**: chunks exceeding max size that can't be split
- **BALANCER_DISABLED**: chunk balancer is off (intentional or accidental?)
- **SHARD_KEY_CARDINALITY**: shard key has low cardinality (e.g., `{status: 1}` with only 3 values)

### CLI Changes
1. Add `--sharding` flag to `audit` command (opt-in — requires config database access)
2. Sharding findings appear alongside collection findings

### Constraints
- Read-only: never modify shard configurations
- Requires access to `config` database (available on mongos or config servers)
- Gracefully skip if not a sharded cluster (standalone or replica set)
- Chunk distribution analysis limits to first 10000 chunks per collection

### Acceptance
- `mongospectre audit --uri mongodb://mongos:27017 --sharding` reports shard issues
- Detects monotonic shard keys, unbalanced chunks, jumbo chunks
- Gracefully handles non-sharded deployments
- `make test && make lint` clean

---

## WO-40: MongoDB Atlas API Integration

**Goal:** Pull cloud-specific metrics and recommendations from MongoDB Atlas for richer audit context.

### Problem
Self-hosted MongoDB exposes all metadata via database commands. Atlas restricts some commands but offers a REST API with additional metrics: real-time performance data, auto-scaling events, alert history, and Atlas-specific recommendations.

### Approach
1. Accept Atlas API keys via `--atlas-public-key` and `--atlas-private-key` (or env vars)
2. Query Atlas Admin API for project/cluster metadata
3. Enrich audit findings with Atlas-specific context
4. Add Atlas-only detections

### Atlas API Queries
- `GET /groups/{groupId}/clusters/{clusterName}` — cluster config (tier, version, region)
- `GET /groups/{groupId}/clusters/{clusterName}/performanceAdvisor/suggestedIndexes` — Atlas index suggestions
- `GET /groups/{groupId}/alerts` — active alerts
- `GET /groups/{groupId}/clusters/{clusterName}/metrics` — real-time metrics (connections, opcounters)

### Detections
- **ATLAS_INDEX_SUGGESTION**: Atlas Performance Advisor recommends an index — cross-reference with code patterns
- **ATLAS_ALERT_ACTIVE**: cluster has unacknowledged alerts (tickets open, connections high, etc.)
- **ATLAS_TIER_MISMATCH**: M10 tier but 500GB data — likely needs upgrade
- **ATLAS_VERSION_BEHIND**: cluster running MongoDB X but latest is Y

### CLI Changes
1. Add `--atlas-public-key`, `--atlas-private-key`, `--atlas-project`, `--atlas-cluster` flags
2. Environment variables: `ATLAS_PUBLIC_KEY`, `ATLAS_PRIVATE_KEY`, `ATLAS_PROJECT_ID`
3. Atlas findings merge with standard audit findings

### Constraints
- Read-only: never modify Atlas configuration
- Atlas API keys are optional — tool works without them
- Never log or output API keys
- Rate limit API calls (Atlas has per-minute limits)

### Acceptance
- `mongospectre audit --uri ... --atlas-public-key ... --atlas-private-key ...` includes Atlas findings
- Atlas index suggestions correlated with code patterns
- Works without Atlas keys (graceful skip)
- `make test && make lint` clean

---

## Phase 5: Developer Experience

---

## WO-41: Schema Validation Drift Detection

**Goal:** Compare MongoDB JSON Schema validators on collections against code expectations.

### Problem
MongoDB supports JSON Schema validation on collections (`db.createCollection("users", {validator: {$jsonSchema: {...}}})}`). When code evolves but validators aren't updated, inserts fail silently or validators become stale.

### Inspector Changes
1. Add `GetValidators(ctx, database)` to inspector
2. Query `db.listCollections({}, {nameOnly: false})` to extract `options.validator`
3. Return `ValidatorInfo{Collection, Schema, ValidationLevel, ValidationAction}`

### Detections
- **VALIDATOR_MISSING**: code writes to collection with no schema validator (risk: unconstrained writes)
- **VALIDATOR_STALE**: validator requires field `email` as `string` but code writes it as `{address: ..., verified: ...}` (object)
- **VALIDATOR_STRICT_RISK**: validation action is `error` + level is `strict` — all inserts fail on mismatch
- **VALIDATOR_WARN_ONLY**: validation action is `warn` — violations logged but not rejected (might be intentional)
- **FIELD_NOT_IN_VALIDATOR**: code writes field `preferences` but validator's `properties` doesn't include it (will fail if `additionalProperties: false`)

### Analyzer Changes
1. Create `internal/analyzer/validator.go` — compare code field refs against validator schemas
2. Parse JSON Schema `properties`, `required`, `additionalProperties`
3. Flag mismatches between code write patterns and validator constraints

### Acceptance
- `mongospectre check --repo ./app --uri ...` reports validator drift
- Detects missing validators, stale field types, strict vs warn modes
- `make test && make lint` clean

---

## WO-42: Interactive TUI

**Goal:** Terminal UI for exploring findings, drilling into collections, and navigating code references.

### Problem
Large reports (100+ findings) are hard to scan in text or JSON output. An interactive TUI lets users filter, sort, and drill into findings without leaving the terminal.

### Technology
- Use `charmbracelet/bubbletea` for TUI framework
- Use `charmbracelet/lipgloss` for styling
- Use `charmbracelet/bubbles` for tables, text inputs, viewports

### Features
- **Finding list**: sortable table of findings with severity, type, collection, message
- **Detail view**: expand a finding to see full context (index definition, code locations, suggested fix)
- **Filter bar**: type to filter by collection, severity, or finding type
- **Code view**: show scanner refs (file:line) with syntax highlighting
- **Severity summary**: header bar showing count by severity (similar to `htop`)
- **Export**: `e` key exports current filtered view to JSON

### CLI Changes
1. Add `--interactive` / `-i` flag to `audit` and `check` commands
2. Default to interactive mode when stdout is a TTY and finding count > 20
3. `--no-interactive` to force non-interactive mode

### Constraints
- TUI is optional — default behavior unchanged for CI/scripting
- No network calls from TUI — operates on already-fetched data
- Fallback to text output if terminal doesn't support TUI

### Acceptance
- `mongospectre audit --uri ... -i` launches interactive TUI
- Findings are filterable and sortable
- Detail view shows actionable context
- Non-TTY environments get normal text output
- `make test && make lint` clean

---

## WO-43: VS Code Extension

**Goal:** Show mongospectre findings inline in VS Code as diagnostics on collection references.

### Problem
Developers see mongospectre findings in CI or terminal reports but must manually map them back to code locations. An IDE extension surfaces findings exactly where the code is.

### Technology
- VS Code extension in TypeScript
- Language Server Protocol (LSP) for diagnostics
- Runs `mongospectre check --format json` in the background

### Features
- **Inline diagnostics**: squiggly underlines on `db.Collection("missing_collection")` with finding message
- **Hover info**: hover on collection name shows index info, document count, last query time
- **Quick fix**: suggest adding to `.mongospectreignore` for known-OK findings
- **Status bar**: show finding count (e.g., "mongospectre: 3 issues")
- **Auto-refresh**: re-run on file save (debounced)

### Structure
```
vscode-mongospectre/
  src/extension.ts     — activate, register providers
  src/runner.ts        — execute mongospectre CLI, parse JSON output
  src/diagnostics.ts   — map findings to VS Code Diagnostic objects
  src/hover.ts         — collection info on hover
  package.json         — extension manifest
```

### Configuration
```json
{
  "mongospectre.uri": "mongodb://localhost:27017",
  "mongospectre.database": "myapp",
  "mongospectre.autoRefresh": true,
  "mongospectre.binaryPath": "mongospectre"
}
```

### Constraints
- Extension calls the CLI binary — no Go code in the extension
- Requires `mongospectre` binary installed (or bundled)
- Findings cached until next run (no live MongoDB connection from extension)

### Acceptance
- Install from VS Code marketplace or VSIX
- Collection references show inline diagnostics
- Hover shows collection metadata
- Works with Go, Python, JavaScript, TypeScript files
- Extension published to VS Code marketplace

---

## WO-44: Webhook / Slack Notifications for Watch Mode

**Goal:** Send alerts to Slack, webhooks, or email when `watch` mode detects new findings.

### Problem
`watch` mode prints to stdout, which is only useful if someone is watching the terminal. For production monitoring, findings should push to team communication channels.

### Configuration
```yaml
# .mongospectre.yml
notifications:
  - type: slack
    webhook_url: ${SLACK_WEBHOOK_URL}
    on: [new_high, new_medium]
  - type: webhook
    url: https://api.example.com/alerts
    method: POST
    headers:
      Authorization: "Bearer ${ALERT_TOKEN}"
    on: [new_high]
  - type: email
    smtp_host: smtp.gmail.com
    smtp_port: 587
    to: ["team@example.com"]
    on: [new_high, resolved]
```

### Features
- **Slack**: post formatted message with finding details, severity color, and link to dashboard
- **Webhook**: POST JSON payload with finding data to arbitrary URL
- **Email**: SMTP-based email alerts (simple HTML template)
- **Filters**: `on` field controls which events trigger notifications (new_high, new_medium, new_low, resolved)
- **Rate limiting**: max 1 notification per finding per interval (avoid spam on flapping)
- **Dry run**: `--notify-dry-run` logs what would be sent without sending

### CLI Changes
1. Add `--notify` flag to `watch` command (reads from config or flag)
2. Add `--notify-dry-run` for testing
3. Environment variable expansion in webhook URLs and secrets (`${VAR}` syntax)

### Constraints
- Never include MongoDB URI or credentials in notification payloads
- Notification failures logged but don't stop watch loop
- Secrets must come from environment variables, not plaintext in config

### Acceptance
- `mongospectre watch --uri ... --notify` sends Slack messages on new findings
- Webhook integration works with generic endpoints
- Rate limiting prevents notification spam
- Dry run mode logs payloads without sending
- `make test && make lint` clean

---

## Non-Goals

- No schema enforcement or migrations
- No data modification or deletion
- No web UI (TUI in WO-42 is terminal-only)
- No password brute-forcing or credential testing (WO-27 reports patterns, not passwords)
