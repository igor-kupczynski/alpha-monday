# Alpha Monday - Low Level Design: Deployment and Ops

Date: 2026-01-30

## Overview
Deployment targets Hatchet Cloud, Scaleway Serverless Containers (API + worker), and Neon Postgres.

## Artifacts
- API container image
- Worker container image
- Migration job (uses `migrate` CLI with `migrations/` directory)

## Environments
- dev: local database or Neon dev project
- prod: Hatchet Cloud + Scaleway + Neon

## Configuration
- DATABASE_URL
- OPENAI_API_KEY
- ALPHA_VANTAGE_API_KEY
- HATCHET credentials
- LOG_LEVEL
- CORS_ALLOW_ORIGINS (API)
- OPENAI_MODEL (optional)
- HATCHET_WORKER_NAME (optional)
- HATCHET_CLIENT_HOST_PORT (optional)

## Containerization
- `Dockerfile.api` builds the API binary and exposes port 8080.
- `Dockerfile.worker` builds the worker binary with no exposed port.
- Images run as non-root distroless containers.
- Build args support `TARGETOS`/`TARGETARCH` for buildx.

## CI Image Builds
- `images.yml` builds/pushes tagged images on `v*` tags or manual dispatch.
- Registry secrets are injected via GitHub Actions secrets.

## Deployment Steps (high-level)
1. Build and push API image via CI (tagged).
2. Build and push Worker image via CI (tagged).
3. Run migrations as a separate job with explicit approval.
4. Deploy API to Scaleway Serverless Containers (manual approval).
5. Deploy Worker to Scaleway Serverless Containers (manual approval).
6. Configure Hatchet workflow registration and cron.

## Migrations
- Use `migrate` CLI with the `migrations/` directory.
- Run as a one-off job against Neon before the first deploy and on schema changes.

## Secrets Management
- Use provider secrets store (Scaleway) or env injection.

## Observability
- Log to stdout/stderr.
- Optional events table for audit.

## Rollback
- Roll back by redeploying previous container tags.

## TODOs
- Add alerts for workflow failures.
- Add DB backup policy.
