# Alpha Monday - Low Level Design: Deployment and Ops

Date: 2026-01-30

## Overview
Deployment targets Hatchet Cloud, Scaleway Serverless Containers (API + worker), and Neon Postgres.

## Artifacts
- API container image
- Worker container image

## Environments
- dev: local database or Neon dev project
- prod: Hatchet Cloud + Scaleway + Neon

## Configuration
- DATABASE_URL
- OPENAI_API_KEY
- ALPHA_VANTAGE_API_KEY
- HATCHET credentials
- LOG_LEVEL

## Deployment Steps (high-level)
1. Build and push API image via CI (tagged).
2. Build and push Worker image via CI (tagged).
3. Run migrations as a separate job with explicit approval.
4. Deploy API to Scaleway Serverless Containers (manual approval).
5. Deploy Worker to Scaleway Serverless Containers (manual approval).
6. Configure Hatchet workflow registration and cron.

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
