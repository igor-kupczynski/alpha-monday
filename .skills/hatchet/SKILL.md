---
name: hatchet
description: Build and debug Hatchet workflows (Python/TypeScript/Go). DAGs, events, cron, rate limiting, durable tasks.
version: 1.0.0
---

# Hatchet Workflow Development

Hatchet is a platform for background tasks and durable execution built on PostgreSQL. You write tasks in Python, TypeScript, or Go and run them on workers in your infrastructure.

## Cloud vs Self-Hosted

| Aspect | Hatchet Cloud | Self-Hosted |
|--------|---------------|-------------|
| Setup | Sign up, get token | Docker + PostgreSQL |
| Pricing | Free tier to Enterprise | Free (MIT license) |
| Compliance | SOC 2, HIPAA (Enterprise) | Your responsibility |
| TLS | Always enabled | Optional |

Start with Cloud for prototyping, self-host for production control. See [references/sdk-setup.md](references/sdk-setup.md) for installation.

## Simple Task Pattern

**Python:**
```python
from hatchet_sdk import Hatchet, Context
from pydantic import BaseModel

hatchet = Hatchet()

class Input(BaseModel):
    message: str

@hatchet.task(name="SimpleTask", input_validator=Input)
def simple_task(input: Input, ctx: Context) -> dict:
    return {"result": input.message.lower()}

worker = hatchet.worker("my-worker", workflows=[simple_task])
worker.start()
```

**TypeScript:**
```typescript
import { HatchetClient } from '@hatchet/v1';

const hatchet = HatchetClient.init();

export const simpleTask = hatchet.task({
  name: 'simple-task',
  fn: (input: { Message: string }) => ({
    result: input.Message.toLowerCase(),
  }),
});

const worker = await hatchet.worker('my-worker', { workflows: [simpleTask] });
await worker.start();
```

**Go:**
```go
task := client.NewStandaloneTask("simple-task",
    func(ctx hatchet.Context, input Input) (*Output, error) {
        return &Output{Result: strings.ToLower(input.Message)}, nil
    },
)
worker, _ := client.NewWorker("my-worker", hatchet.WithWorkflows(task))
worker.StartBlocking(context.Background())
```

## DAG Workflows (Dependencies)

Define task dependencies with `parents`:

**Python:**
```python
dag = hatchet.workflow(name="DAGWorkflow")

@dag.task()
def step1(input, ctx: Context):
    return {"value": input.data * 2}

@dag.task(parents=[step1])
def step2(input, ctx: Context):
    step1_out = ctx.task_output(step1)
    return {"result": step1_out["value"] + 10}
```

**TypeScript:**
```typescript
const step1 = dag.task({
  name: 'step1',
  fn: (input) => ({ value: input.data * 2 }),
});

dag.task({
  name: 'step2',
  parents: [step1],
  fn: async (input, ctx) => {
    const out = await ctx.parentOutput(step1);
    return { result: out.value + 10 };
  },
});
```

See [references/python-patterns.md](references/python-patterns.md), [references/typescript-patterns.md](references/typescript-patterns.md), and [references/go-patterns.md](references/go-patterns.md) for complete examples.

## Event-Driven Workflows

Trigger workflows from events:

```python
event_workflow = hatchet.workflow(
    name="EventWorkflow",
    on_events=["user:create", "order:*"],  # Wildcards supported
)

# Push events from your API
hatchet.event.push("user:create", {"user_id": "123"})
```

## Scheduled/Cron Runs

```python
from datetime import datetime, timedelta

# Cron trigger (defined at workflow level)
cron_workflow = hatchet.workflow(
    name="CronWorkflow",
    on_crons=["0 9 * * *"],  # Daily at 9am
)

# One-time scheduled run
schedule = my_workflow.schedule(datetime(2025, 3, 14, 15, 9, 26))

# Manage schedules
hatchet.scheduled.list()
hatchet.scheduled.delete(scheduled_id=schedule.metadata.id)
```

## Concurrency & Rate Limiting

```python
from hatchet_sdk import ConcurrencyExpression, RateLimit, RateLimitDuration

workflow = hatchet.workflow(
    name="ControlledWorkflow",
    concurrency=ConcurrencyExpression(
        expression="input.user_id",  # Limit per user
        max_runs=5,
    ),
)

@workflow.task(
    rate_limits=[
        RateLimit(
            dynamic_key="input.user_id",
            units=1,
            limit=10,
            duration=RateLimitDuration.MINUTE,
        )
    ]
)
def rate_limited_task(input, ctx):
    pass
```

## Durable Execution

For long-running tasks that survive crashes:

```python
@workflow.durable_task()
async def orchestrator(input, ctx: DurableContext):
    # Durable sleep (survives restarts)
    await ctx.aio_sleep_for(duration=timedelta(hours=24))

    # Spawn child tasks (required for I/O)
    result = await ctx.aio.run_child(process_task, {"data": input.data})

    return result
```

**Key rules:**
- Only use methods on DurableContext (no direct I/O)
- Spawn I/O operations as child tasks
- Use DAGs when workflow shape is known upfront

## Workflow Execution

```python
# Sync (wait for result)
result = my_workflow.run({"message": "hello"})

# Async (fire and forget)
run_ref = my_workflow.run_no_wait({"message": "hello"})
```

## Debugging Checklist

1. **Worker not connecting**: Check `HATCHET_CLIENT_TOKEN` is set
2. **Task timeouts**: Increase `execution_timeout` on task decorator
3. **Rate limit errors**: Check concurrency config, view queue in dashboard
4. **Parent output missing**: Ensure parent is in `parents=[]` list
5. **Self-hosted TLS**: Set `HATCHET_CLIENT_TLS_STRATEGY=none` if no TLS

See [references/troubleshooting.md](references/troubleshooting.md) for detailed solutions.

## Resources

- Docs: https://docs.hatchet.run
- Dashboard: https://cloud.hatchet.run (Cloud) or http://localhost:8080 (self-hosted)
