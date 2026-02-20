---
name: mongospectre
description: MongoDB collection auditor — finds unused collections, orphaned indexes, missing index coverage
user-invocable: false
metadata: {"requires":{"bins":["mongospectre"]}}
---

# mongospectre — MongoDB Collection Auditor

You have access to `mongospectre`, a tool that audits MongoDB clusters for unused collections, orphaned indexes, missing index coverage, and schema drift between code and live database.

## Install

```bash
brew install ppiankov/tap/mongospectre
```

## Commands

| Command | What it does |
|---------|-------------|
| `mongospectre audit --uri <uri>` | Cluster-only analysis |
| `mongospectre check --uri <uri> --repo <path>` | Compare code references against live DB |
| `mongospectre compare --source <uri> --target <uri>` | Cross-cluster schema diff |
| `mongospectre watch --uri <uri> --interval 5m` | Continuous monitoring |
| `mongospectre init` | Scaffold config files |
| `mongospectre version` | Print version |

## Key Flags

| Flag | Applies to | Description |
|------|-----------|-------------|
| `--uri` | audit, check, watch | MongoDB connection URI (env: `MONGODB_URI`) |
| `--repo` | check | Path to code repository |
| `--format` / `-f` | audit, check, compare, watch | Output: text, json, sarif, spectrehub |
| `--database` | audit, check, watch | Specific database to target |
| `--fail-on-missing` | check | Exit 2 if MISSING_COLLECTION found |
| `--baseline` | audit, check | Previous JSON report for diff comparison |
| `--no-ignore` | audit, check | Bypass .mongospectreignore file |
| `--interval` | watch | Time between audit runs |
| `--notify` | watch | Send notifications for new/resolved findings |
| `--source`, `--target` | compare | Source and target MongoDB URIs |

## Agent Usage Pattern

```bash
mongospectre audit --uri "mongodb://localhost:27017" --format json
```

### JSON Output Structure

```json
{
  "metadata": {
    "tool": "mongospectre",
    "version": "0.3.0",
    "command": "audit",
    "timestamp": "2026-02-20T12:00:00Z"
  },
  "findings": [
    {
      "type": "UNUSED_COLLECTION",
      "severity": "medium",
      "database": "myapp",
      "collection": "legacy_sessions",
      "message": "Collection has no reads in 90 days"
    }
  ],
  "summary": {
    "total": 8,
    "high": 1,
    "medium": 4,
    "low": 2,
    "info": 1
  }
}
```

### Parsing Examples

```bash
# List unused collections
mongospectre audit --uri "$MONGODB_URI" --format json | jq '.findings[] | select(.type == "UNUSED_COLLECTION")'

# Continuous monitoring as NDJSON
mongospectre watch --uri "$MONGODB_URI" --interval 5m --format json

# Cross-cluster diff
mongospectre compare --source "$STAGING_URI" --target "$PROD_URI" --format json
```

## Cross-Tool Integration

mongospectre outputs `--format spectrehub` for integration with [spectrehub](https://github.com/ppiankov/spectrehub), which aggregates findings from all spectre tools.

## Exit Codes

- `0` — success
- `1` — error
- `2` — findings matched --fail-on criteria

## What mongospectre Does NOT Do

- Does not modify collections or indexes — read-only analysis
- Does not use ML — deterministic statistics and pattern matching
- Does not store results remotely — local output only
- Does not require admin access — works with read-only database access
