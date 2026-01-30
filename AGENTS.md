- `docs/` contains design docs (e.g., `docs/001-high-level-design.md`): both high-level design (HLD) and low-level design (LLD) and implementation plans.
- When working out of an implementation plan, use checkboxes to track progress. Also update the plan as needed when you iterate on it.

## Workflow intent
- Work should flow from HLD → LLD → tests → code.
- If you modify code, write or update tests first (to catch the failure), then update code.
- Keep HLD and LLD docs up to date with any behavioral or architectural changes.
- It's OK to iterate on the docs.
- When we make any decisions do update the LLDs and HLDs to reflect those decisions. Don't ask "should we update the docs?" Just do it.

## Commands and workflows (minimal)
- Local DB: `./scripts/db-up`, `./scripts/db-down`, `./scripts/db-reset`
- Tests: `go test ./...` (run after `./scripts/db-up`)
- DB-backed tests run in multiple packages; keep the DB running for the full test run.
