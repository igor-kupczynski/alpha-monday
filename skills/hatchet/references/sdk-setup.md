# Hatchet SDK Setup

## Installation

**Python:**
```bash
pip install hatchet-sdk
```

**TypeScript (V1):**
```bash
npm install @hatchet-dev/typescript-sdk
# or
pnpm add @hatchet-dev/typescript-sdk
```

**Go:**
```bash
go get github.com/hatchet-dev/hatchet/sdks/go
```

## Environment Variables

Required for all deployments:
```bash
export HATCHET_CLIENT_TOKEN="your-api-token"
```

For self-hosted endpoints (match your server config):
```bash
# gRPC host/port
export HATCHET_CLIENT_HOST_PORT="localhost:7077"

# API host/port (TypeScript SDK)
export HATCHET_CLIENT_API_URL="http://localhost:8080"

# API host/port (Go SDK)
export HATCHET_CLIENT_SERVER_URL="http://localhost:8080"
```

For self-hosted without TLS (dev only):
```bash
export HATCHET_CLIENT_TLS_STRATEGY=none
```

## Hatchet Cloud Quickstart

1. Sign up at https://cloud.onhatchet.run
2. Create a new tenant (or use default)
3. Go to **Settings > API Tokens**
4. Generate a token and set `HATCHET_CLIENT_TOKEN`
5. Start coding with the SDK

## Self-Hosted Quickstart

Requires Docker installed locally.

**Install CLI:**
```bash
curl -fsSL https://install.hatchet.run/install.sh | bash
```

**Start local server:**
```bash
hatchet server start
```

This starts Hatchet with PostgreSQL in Docker. Dashboard available at http://localhost:8080.

**Get token:**
```bash
hatchet token create
```

## Client Initialization

**Python:**
```python
from hatchet_sdk import Hatchet

# Uses HATCHET_CLIENT_TOKEN from environment
hatchet = Hatchet()

# Or explicit config
hatchet = Hatchet(
    token="your-token",
    host="localhost:7077",  # For self-hosted
)
```

**TypeScript (V1):**
```typescript
import { HatchetClient } from '@hatchet-dev/typescript-sdk/v1';

// Uses HATCHET_CLIENT_TOKEN from environment
const hatchet = HatchetClient.init();
```

**Go:**
```go
import "github.com/hatchet-dev/hatchet/sdks/go"

// Uses HATCHET_CLIENT_TOKEN from environment
client, err := hatchet.NewClient()

// Or with options
client, err := hatchet.NewClient(
    hatchet.WithToken("your-token"),
    hatchet.WithHostPort("localhost:7077"),
)
```

## Verify Connection

Run a simple worker to test:

```python
from hatchet_sdk import Hatchet

hatchet = Hatchet()

@hatchet.task(name="HealthCheck")
def health_check(input, ctx):
    return {"status": "ok"}

worker = hatchet.worker("health-worker", workflows=[health_check])
worker.start()  # Should connect without errors
```
