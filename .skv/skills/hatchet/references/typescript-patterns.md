# TypeScript SDK Patterns

## Type-Safe Workflow Definition

```typescript
import { HatchetClient } from '@hatchet-dev/typescript-sdk/v1';

const hatchet = HatchetClient.init();

// Define types for inputs and outputs
interface ProcessInput {
  userId: string;
  data: Record<string, unknown>;
}

interface ProcessOutput {
  result: string;
  processedAt: string;
}

// Type-safe task
export const processData = hatchet.task<ProcessInput, ProcessOutput>({
  name: 'process-data',
  fn: (input) => ({
    result: `Processed ${input.userId}`,
    processedAt: new Date().toISOString(),
  }),
});
```

## DAG Workflow with Dependencies

```typescript
import { HatchetClient } from '@hatchet-dev/typescript-sdk/v1';

const hatchet = HatchetClient.init();

interface PipelineInput {
  source: string;
}

const pipeline = hatchet.workflow<PipelineInput>({
  name: 'data-pipeline',
});

const extract = pipeline.task({
  name: 'extract',
  fn: (input) => {
    console.log('Extracting...');
    return { rawData: input.source };
  },
});

const transform = pipeline.task({
  name: 'transform',
  parents: [extract],
  fn: async (input, ctx) => {
    const extracted = await ctx.parentOutput(extract);
    console.log(`Transforming: ${extracted.rawData}`);
    return { transformed: extracted.rawData.toUpperCase() };
  },
});

const load = pipeline.task({
  name: 'load',
  parents: [transform],
  fn: async (input, ctx) => {
    const transformed = await ctx.parentOutput(transform);
    console.log(`Loading: ${transformed.transformed}`);
    return { loaded: true };
  },
});

// Worker setup
const worker = await hatchet.worker('pipeline-worker', {
  workflows: [pipeline],
});
await worker.start();

// Trigger
const result = await pipeline.run({ source: 'raw input' });
```

## Parallel Fan-Out

```typescript
const parallel = hatchet.workflow({ name: 'parallel-pipeline' });

const fetchUsers = parallel.task({
  name: 'fetch-users',
  fn: () => ({ users: ['user1', 'user2', 'user3'] }),
});

// These run in parallel
const batchA = parallel.task({
  name: 'batch-a',
  parents: [fetchUsers],
  fn: async (input, ctx) => {
    const { users } = await ctx.parentOutput(fetchUsers);
    return { batch: 'a', count: users.slice(0, 2).length };
  },
});

const batchB = parallel.task({
  name: 'batch-b',
  parents: [fetchUsers],
  fn: async (input, ctx) => {
    const { users } = await ctx.parentOutput(fetchUsers);
    return { batch: 'b', count: users.slice(2).length };
  },
});

// Waits for both
const aggregate = parallel.task({
  name: 'aggregate',
  parents: [batchA, batchB],
  fn: async (input, ctx) => {
    const a = await ctx.parentOutput(batchA);
    const b = await ctx.parentOutput(batchB);
    return { total: a.count + b.count };
  },
});
```

## Event-Driven Workflow

```typescript
const eventWorkflow = hatchet.workflow({
  name: 'event-handler',
  onEvents: ['user:created', 'user:updated'],
});

eventWorkflow.task({
  name: 'handle-event',
  fn: (input) => {
    console.log(`Handling event for user: ${input.userId}`);
    return { processed: true };
  },
});

// Push events from your API
await hatchet.event.push('user:created', { userId: '123' });
```

## Worker Configuration

```typescript
const worker = await hatchet.worker('my-worker', {
  workflows: [workflow1, workflow2],
  slots: 100,           // Concurrent task capacity
  durableSlots: 1000,   // Durable task capacity
  labels: {
    environment: 'production',
    region: 'us-west-2',
  },
});

// Graceful shutdown
process.on('SIGTERM', async () => {
  await worker.stop();
  process.exit(0);
});

await worker.start();
```

## Error Handling

```typescript
const retryable = hatchet.task({
  name: 'retryable-task',
  retries: 3,
  executionTimeout: '5m',
  fn: async (input) => {
    const response = await fetch(input.url);
    if (!response.ok) {
      throw new Error(`API failed: ${response.status}`);
    }
    return await response.json();
  },
});
```

## Context Usage

```typescript
workflow.task({
  name: 'contextful-task',
  parents: [parentTask],
  fn: async (input, ctx) => {
    // Logging
    ctx.log('Processing started');

    // Get parent output
    const parentOut = await ctx.parentOutput(parentTask);

    // Access run metadata
    const runId = ctx.workflowRunId();

    return { done: true };
  },
});
```

## Standalone Task (No Workflow)

```typescript
// For simple, independent tasks
const simpleTask = hatchet.task({
  name: 'simple-task',
  fn: (input: { message: string }) => ({
    result: input.message.toLowerCase(),
  }),
});

// Run directly (ensure a worker has registered this task first)
const result = await simpleTask.run({ message: 'HELLO' });
console.log(result); // { result: 'hello' }
```
