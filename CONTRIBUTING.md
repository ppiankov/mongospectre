# Contributing to mongospectre

## Getting Started

```bash
git clone https://github.com/ppiankov/mongospectre.git
cd mongospectre
make deps
make build
make test
```

## Branch Strategy

- `main` is the release branch — always passing CI
- Feature branches: `feat/description`
- Bug fix branches: `fix/description`
- Open a PR against `main` for all changes

## Commit Conventions

Format: `type: concise imperative statement`

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `perf`, `ci`, `build`

- One line, max 72 characters
- Lowercase after colon, no trailing period
- Body (optional, blank line separated) explains **why**, not what

## Code Style

- Go with `gofmt` formatting
- Run `make lint` before submitting (golangci-lint)
- Run `make test` — all tests must pass with `-race`
- Coverage target: >85% on analyzer, scanner, reporter packages

## Pull Request Process

1. Create a feature branch from `main`
2. Make focused, small commits
3. Ensure `make lint && make test` pass
4. Open PR with a clear description of what and why
5. Address review feedback

## Rules

- All queries must be **read-only** — never modify MongoDB data
- Never commit secrets, credentials, or `.env` files
- Never force push to `main`
- Tests are mandatory for all new code

## Reporting Issues

Open an issue at https://github.com/ppiankov/mongospectre/issues with:
- What you expected
- What happened
- Steps to reproduce
- MongoDB version and `mongospectre version` output
