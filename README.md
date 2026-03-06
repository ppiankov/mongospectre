# mongospectre

[![CI](https://github.com/ppiankov/mongospectre/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/mongospectre/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ppiankov/mongospectre)](https://goreportcard.com/report/github.com/ppiankov/mongospectre)
[![ANCC](https://img.shields.io/badge/ANCC-compliant-brightgreen)](https://ancc.dev)

**mongospectre** — MongoDB collection and index auditor. Part of [SpectreHub](https://github.com/ppiankov/spectrehub).

## What it is

- Connects to MongoDB and fetches collection metadata, index definitions, and usage statistics
- Scans code repositories for collection and field references across Go, Python, JS/TS, Java, C#, Ruby
- Extracts queried fields from aggregation pipelines
- Compares code references against live database to find drift, unused indexes, and missing collections
- Outputs text, JSON, SARIF, and SpectreHub formats

## What it is NOT

- Not a MongoDB monitoring tool — use mongostat/mongotop for that
- Not a migration tool or query profiler
- Not a backup or replication tool
- Does not modify any data — strictly read-only

## Quick start

### Homebrew

```sh
brew tap ppiankov/tap
brew install mongospectre
```

### From source

```sh
git clone https://github.com/ppiankov/mongospectre.git
cd mongospectre
make build
```

### Usage

```sh
mongospectre audit --uri "mongodb://localhost:27017" --db mydb
```

## CLI commands

| Command | Description |
|---------|-------------|
| `mongospectre audit` | Audit MongoDB for unused indexes and collection drift |
| `mongospectre check` | Compare code references against live database |
| `mongospectre watch` | Continuous drift detection |
| `mongospectre version` | Print version |

## SpectreHub integration

mongospectre feeds MongoDB drift findings into [SpectreHub](https://github.com/ppiankov/spectrehub) for unified visibility across your infrastructure.

```sh
spectrehub collect --tool mongospectre
```

## Safety

mongospectre operates in **read-only mode**. It inspects and reports — never modifies, deletes, or alters your data.

## Documentation

| Document | Contents |
|----------|----------|
| [CLI Reference](docs/cli-reference.md) | Full command reference, flags, and configuration |

## License

MIT — see [LICENSE](LICENSE).

---

Built by [Obsta Labs](https://obstalabs.dev)
