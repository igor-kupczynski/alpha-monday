.DEFAULT_GOAL := help

.PHONY: help db-up db-down db-reset test fmt fmt-check lint

help: ## Show this help.
	@awk 'BEGIN {FS=":.*##"; printf "\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

db-up: ## Start local Postgres via docker compose.
	./scripts/db-up

db-down: ## Stop local Postgres and remove containers.
	./scripts/db-down

db-reset: ## Reset local Postgres volume and restart.
	./scripts/db-reset

test: db-up ## Run tests (brings up local Postgres if needed).
	go test ./...

fmt: ## Format Go code.
	go fmt ./...

fmt-check: ## Verify Go formatting without modifying files.
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then echo "Unformatted files:"; echo "$$unformatted"; exit 1; fi

lint: fmt-check ## Run fast static checks (go vet, staticcheck).
	@command -v staticcheck >/dev/null 2>&1 || { echo "staticcheck missing; install with: go install honnef.co/go/tools/cmd/staticcheck@latest"; exit 1; }
	go vet ./...
	staticcheck ./...
