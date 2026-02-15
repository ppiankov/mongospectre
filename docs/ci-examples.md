# CI Integration Examples

## GitHub Actions

### Audit with SARIF upload to Security tab

```yaml
name: MongoDB Audit

on:
  schedule:
    - cron: '0 6 * * 1' # weekly Monday 6am
  workflow_dispatch:

jobs:
  audit:
    runs-on: ubuntu-latest
    services:
      mongodb:
        image: mongo:7
        ports:
          - 27017:27017
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - run: go install github.com/ppiankov/mongospectre/cmd/mongospectre@latest

      - name: Run audit
        run: mongospectre audit --uri mongodb://localhost:27017 --format sarif > results.sarif

      - name: Upload SARIF
        uses: github/codeql-action/upload-sarif@v4
        if: always()
        with:
          sarif_file: results.sarif
```

### Check (code + cluster) on pull request

```yaml
name: MongoDB Check

on:
  pull_request:
    branches: [main]

jobs:
  check:
    runs-on: ubuntu-latest
    env:
      MONGODB_URI: ${{ secrets.MONGODB_URI }}
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - run: go install github.com/ppiankov/mongospectre/cmd/mongospectre@latest

      - name: Check for drift
        run: mongospectre check --repo . --format json --fail-on-missing
```

### Watch mode in CI (exit on new findings)

```yaml
name: MongoDB Watch

on:
  workflow_dispatch:

jobs:
  watch:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    env:
      MONGODB_URI: ${{ secrets.MONGODB_URI }}
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - run: go install github.com/ppiankov/mongospectre/cmd/mongospectre@latest

      - name: Watch for drift
        run: mongospectre watch --interval 5m --exit-on-new --format json
```

## GitLab CI

### Audit job with MongoDB service

```yaml
stages:
  - audit

mongo-audit:
  stage: audit
  image: golang:1.25
  services:
    - name: mongo:7
      alias: mongodb
  variables:
    MONGODB_URI: mongodb://mongodb:27017
  script:
    - go install github.com/ppiankov/mongospectre/cmd/mongospectre@latest
    - mongospectre audit --format json > audit-report.json
  artifacts:
    paths:
      - audit-report.json
    when: always
  rules:
    - if: $CI_PIPELINE_SOURCE == "schedule"
    - if: $CI_PIPELINE_SOURCE == "web"
```

### Check job on merge requests

```yaml
mongo-check:
  stage: audit
  image: golang:1.25
  variables:
    MONGODB_URI: $MONGODB_URI
  script:
    - go install github.com/ppiankov/mongospectre/cmd/mongospectre@latest
    - mongospectre check --repo . --fail-on-missing
  rules:
    - if: $CI_MERGE_REQUEST_ID
```

## Makefile Targets

The project includes convenience targets for common operations:

```bash
# Audit a cluster
make audit URI=mongodb://localhost:27017

# Check code against a cluster
make check URI=mongodb://localhost:27017 REPO=./app

# Compare two environments
make compare SOURCE=mongodb://staging:27017 TARGET=mongodb://prod:27017
```

## CI Gating

Use `--exit-on-new` with `watch` or exit codes with `audit`/`check` to gate deployments:

| Exit Code | Meaning |
|-----------|---------|
| 0 | No findings (or info-only) |
| 1 | Medium-severity findings |
| 2 | High-severity findings |

The `--fail-on-missing` flag on `check` exits with code 2 if any `MISSING_COLLECTION` is found, regardless of other findings.
