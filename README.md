# Alpha Monday

Alpha Monday is a weekly picks service with a read-only API and a worker that runs Hatchet workflows to generate picks, snapshot prices, and compute daily checkpoints.

## Components
- API: HTTP service exposing `/health`, `/latest`, `/batches`, `/batches/{id}`.
- Worker: Hatchet worker that registers workflows and executes steps.
- Postgres: Neon-hosted database.
- Orchestration: Hatchet Cloud (cron + workflow execution).
- Hosting: Scaleway Serverless Containers (API + worker).

## Prerequisites
- Scaleway account (Serverless Containers + Container Registry).
- Neon Postgres project (production database).
- Hatchet Cloud project and client token.
- OpenAI API key and Alpha Vantage API key.

## Container Images
Images are built from `Dockerfile.api` and `Dockerfile.worker`.

### Build locally (optional)
```sh
# API
docker build -f Dockerfile.api -t alpha-monday-api:local .

# Worker
docker build -f Dockerfile.worker -t alpha-monday-worker:local .
```

### CI build and push (recommended)
The `images` GitHub Actions workflow builds and pushes images on tag creation.

1. Create a Scaleway Container Registry namespace.
2. Add GitHub Actions secrets:
   - `REGISTRY` (optional, default `rg.fr-par.scw.cloud`)
   - `REGISTRY_NAMESPACE` (your Scaleway registry namespace)
   - `REGISTRY_USERNAME` (Scaleway access key)
   - `REGISTRY_PASSWORD` (Scaleway secret key)
3. Tag and push:
```sh
git tag v0.1.0
git push origin v0.1.0
```

Images will be pushed as:
- `${REGISTRY}/${REGISTRY_NAMESPACE}/alpha-monday-api:v0.1.0`
- `${REGISTRY}/${REGISTRY_NAMESPACE}/alpha-monday-worker:v0.1.0`

## Database (Neon)
1. Create a Neon project and database.
2. Allow connections from Scaleway (IP allowlist or public access).
3. Capture the connection string:
   - `DATABASE_URL=postgres://USER:PASSWORD@HOST:PORT/DB?sslmode=require`

### Migrations
Run migrations before the first deploy and on schema changes.

Using the migrate CLI via Docker:
```sh
docker run --rm \
  -v "$(pwd)/migrations:/migrations" \
  migrate/migrate \
  -path /migrations \
  -database "$DATABASE_URL" \
  up
```

## Deploy API (Scaleway Serverless Containers)
1. Create a new container service for the API image.
2. Set the container image to the tagged API image.
3. Configure environment variables:
   - `DATABASE_URL` (Neon connection string)
   - `PORT` (default 8080)
   - `LOG_LEVEL` (info, debug, warn, error)
   - `CORS_ALLOW_ORIGINS` (optional, comma-separated)
4. Configure the port to 8080 and expose it publicly.
5. Deploy the container.

Health check endpoint: `GET /health`

## Deploy Worker (Scaleway Serverless Containers)
1. Create a new container service for the worker image.
2. Set the container image to the tagged worker image.
3. Configure environment variables:
   - `DATABASE_URL`
   - `OPENAI_API_KEY`
   - `OPENAI_MODEL` (optional, default `gpt-4o-mini`)
   - `ALPHA_VANTAGE_API_KEY`
   - `HATCHET_CLIENT_TOKEN`
   - `HATCHET_CLIENT_HOST_PORT` (optional)
   - `HATCHET_WORKER_NAME` (optional, default `alpha-monday-worker`)
   - `LOG_LEVEL`
4. Deploy the container.

The worker registers workflows at startup. Keep the worker running to receive cron triggers.

## Hatchet Cron Configuration
In Hatchet Cloud, configure a cron trigger for the weekly workflow:
- Workflow ID: `weekly_pick_v1`
- Schedule: `0 9 * * 1`
- Timezone: `America/New_York`

The daily checkpoint loop is internal to the workflow; no additional cron is required.

## Interacting With The System

### API
Example requests (replace `API_BASE_URL`):
```sh
curl -s "$API_BASE_URL/health"
curl -s "$API_BASE_URL/latest"
curl -s "$API_BASE_URL/batches?limit=20"
curl -s "$API_BASE_URL/batches/<batch_id>"
```

### Manual workflow run (optional)
Use the Hatchet UI or CLI to trigger `weekly_pick_v1` if you need an out-of-band run.

## Secrets and Config
- Store secrets in Scaleway secret manager or injected environment variables.
- Do not commit `.env` files.

## Rollback
- Deploy the previous container tag in Scaleway.
- No schema rollback is performed automatically; use down migrations only if required.
