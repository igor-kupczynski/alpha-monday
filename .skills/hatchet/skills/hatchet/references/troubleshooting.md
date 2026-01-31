# Troubleshooting Hatchet

## Connection Issues

### Token Not Working

```bash
# Verify token is set
echo $HATCHET_CLIENT_TOKEN

# Should output your token (not empty)
```

If empty, generate a new token:
- **Cloud**: Dashboard > Settings > API Tokens
- **Self-hosted**: `hatchet token create`

### Worker Not Connecting

**Symptoms:**
- Worker hangs on startup
- "Connection refused" errors
- Timeout errors

**Solutions:**

1. Check network connectivity:
   ```bash
   # Cloud
   curl -I https://api.hatchet.run

   # Self-hosted
   curl -I http://localhost:7077
   ```

2. Verify token permissions (must have worker scope)

3. Check firewall rules (default port: 7077)

4. For self-hosted without TLS:
   ```bash
   export HATCHET_CLIENT_TLS_STRATEGY=none
   ```

### SSL/TLS Errors

**Self-hosted dev environments:**
```bash
export HATCHET_CLIENT_TLS_STRATEGY=none
```

**Self-hosted with self-signed certs:**
```bash
export HATCHET_CLIENT_TLS_ROOT_CA_FILE=/path/to/ca.crt
```

## Task Failures

### Task Timeouts

**Symptoms:**
- Task shows "timed out" in dashboard
- Task killed mid-execution

**Solution:** Increase execution timeout:

```python
from datetime import timedelta

@workflow.task(execution_timeout=timedelta(minutes=30))
def long_running_task(input, ctx):
    # Long operation
    pass
```

```typescript
const task = workflow.task({
  name: 'long-task',
  executionTimeout: '30m',  // 30 minutes
  fn: async (input) => { /* ... */ },
});
```

### Retries Exhausted

**Symptoms:**
- Task failed after N attempts
- "Max retries exceeded" in logs

**Diagnose:**
1. Check dashboard for error messages on each attempt
2. Look for patterns (transient vs persistent failures)

**Solutions:**
- Increase retry count if transient
- Add backoff to avoid rate limiting:
  ```python
  @workflow.task(
      retries=5,
      backoff_factor=2.0,
      backoff_max_seconds=120,
  )
  def flaky_task(input, ctx):
      pass
  ```

### Parent Output Not Found

**Symptoms:**
- `ctx.task_output(parent)` returns None or errors
- "Parent task not found" errors

**Solutions:**

1. Ensure parent is in `parents=[]`:
   ```python
   @workflow.task(parents=[step1])  # Must list parent here
   def step2(input, ctx):
       out = ctx.task_output(step1)
   ```

2. Check parent completed successfully (not failed/cancelled)

3. Verify output type matches expected schema

## Rate Limiting Issues

### Concurrency Limit Hit

**Symptoms:**
- Tasks queued but not running
- "Concurrency limit reached" in dashboard

**Diagnose:**
```python
# Check current limits
workflow = hatchet.workflow(
    name="MyWorkflow",
    concurrency=ConcurrencyExpression(
        expression="input.user_id",
        max_runs=5,  # <- This is the limit
    ),
)
```

**Solutions:**
- Increase `max_runs` if appropriate
- Use different concurrency keys to partition load
- Check if old runs are stuck (cancel them)

### Rate Limit Exceeded

**Symptoms:**
- Tasks rejected with rate limit error
- Bursts of failures followed by success

**Solutions:**
```python
@workflow.task(
    rate_limits=[
        RateLimit(
            dynamic_key="input.user_id",
            units=1,
            limit=100,  # Increase if needed
            duration=RateLimitDuration.MINUTE,
        )
    ]
)
def rate_limited_task(input, ctx):
    pass
```

## Dashboard Debugging

### Finding Your Workflow Run

1. Navigate to **Workflows** in sidebar
2. Click your workflow name
3. View recent runs with status (pending/running/completed/failed)
4. Click a run to see task-level details

### Reading Task Logs

1. Open a workflow run
2. Click on specific task
3. View **Logs** tab for `ctx.log()` output
4. View **Input/Output** tabs for payloads

### Identifying Stuck Runs

1. Filter by status: "Running" for more than expected time
2. Check individual task status within the run
3. Look for tasks stuck in "Pending" (may indicate worker issues)

## Worker Issues

### Worker Slots Exhausted

**Symptoms:**
- Tasks queue but don't start
- Worker shows max slots in use

**Solution:** Increase slots or add more workers:
```python
worker = hatchet.worker(
    "my-worker",
    slots=200,  # Increase from default 100
    workflows=[workflow],
)
```

### Windows Limitations

Workers on Windows have signal handling issues. Use:
- Windows Subsystem for Linux (WSL)
- Docker containers

```bash
# In WSL or Docker
python worker.py
```

## Common Mistakes

### Non-Determinism in Durable Tasks

**Wrong:**
```python
@workflow.durable_task()
async def bad_task(input, ctx: DurableContext):
    data = await fetch_from_db()  # Direct I/O breaks replay!
```

**Right:**
```python
@workflow.durable_task()
async def good_task(input, ctx: DurableContext):
    data = await ctx.aio.run_child(fetch_data, input)  # Spawned as child
```

### Forgetting to Start Worker

Tasks won't run without workers. Ensure worker is started:
```python
worker = hatchet.worker("my-worker", workflows=[workflow])
worker.start()  # Don't forget this!
```

For production, use `start_blocking()` or handle graceful shutdown.
