# Python SDK Patterns

## Input Validation with Pydantic

```python
from hatchet_sdk import Hatchet, Context
from pydantic import BaseModel, Field
from typing import Optional

hatchet = Hatchet()

class ProcessInput(BaseModel):
    user_id: str
    data: dict
    priority: Optional[int] = Field(default=1, ge=1, le=10)

class ProcessOutput(BaseModel):
    result: str
    processed_at: str

@hatchet.task(name="ProcessData", input_validator=ProcessInput)
def process_data(input: ProcessInput, ctx: Context) -> dict:
    # input is validated Pydantic model
    return {
        "result": f"Processed {input.user_id}",
        "processed_at": "2025-01-28T12:00:00Z"
    }
```

## DAG Workflow with Dependencies

```python
from hatchet_sdk import Hatchet, Context
from datetime import timedelta

hatchet = Hatchet()
pipeline = hatchet.workflow(name="DataPipeline")

@pipeline.task(execution_timeout=timedelta(seconds=30))
def extract(input, ctx: Context):
    ctx.log("Extracting data...")
    return {"raw_data": input.source}

@pipeline.task(parents=[extract])
def transform(input, ctx: Context):
    extract_out = ctx.task_output(extract)
    ctx.log(f"Transforming: {extract_out['raw_data']}")
    return {"transformed": extract_out["raw_data"].upper()}

@pipeline.task(parents=[transform])
def load(input, ctx: Context):
    transform_out = ctx.task_output(transform)
    ctx.log(f"Loading: {transform_out['transformed']}")
    return {"loaded": True}

# Register and start worker
worker = hatchet.worker("pipeline-worker", workflows=[pipeline])
worker.start()

# Trigger the workflow
result = pipeline.run({"source": "raw input data"})
```

## Parallel Fan-Out

```python
pipeline = hatchet.workflow(name="ParallelPipeline")

@pipeline.task()
def fetch_users(input, ctx: Context):
    return {"users": ["user1", "user2", "user3"]}

# These run in parallel (same parent)
@pipeline.task(parents=[fetch_users])
def process_batch_a(input, ctx: Context):
    users = ctx.task_output(fetch_users)["users"][:2]
    return {"batch": "a", "processed": len(users)}

@pipeline.task(parents=[fetch_users])
def process_batch_b(input, ctx: Context):
    users = ctx.task_output(fetch_users)["users"][2:]
    return {"batch": "b", "processed": len(users)}

# Waits for both parallel tasks
@pipeline.task(parents=[process_batch_a, process_batch_b])
def aggregate(input, ctx: Context):
    a = ctx.task_output(process_batch_a)
    b = ctx.task_output(process_batch_b)
    return {"total": a["processed"] + b["processed"]}
```

## Durable Task with Human-in-the-Loop

```python
from hatchet_sdk import Hatchet, DurableContext
from datetime import timedelta

hatchet = Hatchet()
approval_flow = hatchet.workflow(name="ApprovalFlow")

@approval_flow.durable_task()
async def await_approval(input, ctx: DurableContext):
    # Send notification (as child task to avoid non-determinism)
    await ctx.aio.run_child(send_notification, {
        "user": input.approver,
        "request_id": input.request_id
    })

    # Wait for approval event (up to 7 days)
    event = await ctx.aio_wait_for(
        f"approval:{input.request_id}",
        expression="payload.status in ['approved', 'rejected']",
        timeout=timedelta(days=7)
    )

    if event.payload["status"] == "approved":
        await ctx.aio.run_child(execute_action, {"request_id": input.request_id})
        return {"status": "completed"}
    else:
        return {"status": "rejected"}

@hatchet.task(name="SendNotification")
def send_notification(input, ctx: Context):
    # Send email/slack notification
    pass

@hatchet.task(name="ExecuteAction")
def execute_action(input, ctx: Context):
    # Perform the approved action
    pass
```

## Error Handling and Retries

```python
from hatchet_sdk import Hatchet, Context
from datetime import timedelta

hatchet = Hatchet()

@hatchet.task(
    name="RetryableTask",
    retries=3,
    backoff_factor=2.0,       # Exponential backoff
    backoff_max_seconds=60,   # Cap at 60s between retries
    execution_timeout=timedelta(minutes=5),
)
def retryable_task(input, ctx: Context):
    # Will retry up to 3 times with exponential backoff
    # Retry delays: ~2s, ~4s, ~8s (capped at 60s)
    response = call_external_api(input.url)
    if not response.ok:
        raise Exception(f"API failed: {response.status}")
    return {"data": response.json()}
```

## Async Tasks

```python
from hatchet_sdk import Hatchet, Context
import asyncio

hatchet = Hatchet()

@hatchet.task(name="AsyncTask")
async def async_task(input, ctx: Context):
    # Async operations supported
    result = await fetch_data_async(input.url)
    await asyncio.sleep(1)  # Non-durable sleep
    return {"result": result}
```

## Context Methods

```python
@workflow.task()
def task_with_context(input, ctx: Context):
    # Logging (visible in dashboard)
    ctx.log("Processing started")
    ctx.log(f"Input: {input}")

    # Get parent task output
    parent_out = ctx.task_output(parent_task)

    # Access workflow run metadata
    run_id = ctx.workflow_run_id()

    # Refresh deadline (for long tasks)
    ctx.refresh_timeout(timedelta(minutes=5))

    return {"done": True}
```
