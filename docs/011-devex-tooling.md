# Alpha Monday - DevEx Tooling (Makefile)

Date: 2026-01-31
Status: draft

## Goals
- Provide a single, discoverable entrypoint for local development tasks.
- Keep local DB operations consistent with existing scripts.
- Make common tasks easy to learn via dynamic help.

## Non-goals (v1)
- Replace scripts/ as the source of truth for DB operations.
- Introduce a new task runner dependency beyond make.

## Decision
Use a top-level Makefile with a dynamic `help` target and a minimal set of standard tasks.

## Targets
- `help`: default target that prints available targets and descriptions.
- `db-up`: start local Postgres (wraps `./scripts/db-up`).
- `db-down`: stop local Postgres (wraps `./scripts/db-down`).
- `db-reset`: reset local Postgres volume (wraps `./scripts/db-reset`).
- `test`: run `go test ./...` (depends on `db-up`).
- `fmt`: run `go fmt ./...`.
- `fmt-check`: verify Go formatting without modifying files.
- `lint`: run `go vet` and `staticcheck` (depends on `fmt-check`).

## Notes
- The Makefile wraps scripts to keep behavior consistent across local development.
- Test targets assume the default `DATABASE_URL` unless overridden by the environment.
- Go toolchain is pinned in `go.mod` (Go 1.25.6).
- CI runs `make lint` and `go test ./...` (without `make test`) to avoid docker compose.
- The hatchet skill is vendored as a git subtree at `.skills/hatchet`; update with:

```bash
git subtree pull --prefix .skills/hatchet https://github.com/igor-kupczynski/hatchet-skill main --squash
```
- Install `staticcheck` once with:

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
```
