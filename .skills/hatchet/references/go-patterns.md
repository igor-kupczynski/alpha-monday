# Go SDK Patterns

## Client Setup

```go
package main

import (
    "context"
    "log"

    "github.com/hatchet-dev/hatchet/sdks/go"
)

func main() {
    // Uses HATCHET_CLIENT_TOKEN from environment
    client, err := hatchet.NewClient()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()
}
```

## Type Definitions

```go
// Input/Output structs with JSON tags
type ProcessInput struct {
    UserID   string                 `json:"user_id"`
    Data     map[string]interface{} `json:"data"`
    Priority int                    `json:"priority,omitempty"`
}

type ProcessOutput struct {
    Result      string `json:"result"`
    ProcessedAt string `json:"processed_at"`
}
```

## Standalone Task (Simple)

```go
package main

import (
    "context"
    "strings"

    "github.com/hatchet-dev/hatchet/sdks/go"
)

type Input struct {
    Message string `json:"message"`
}

type Output struct {
    Result string `json:"result"`
}

func main() {
    client, _ := hatchet.NewClient()
    defer client.Close()

    task := client.NewStandaloneTask("simple-task",
        func(ctx hatchet.Context, input Input) (*Output, error) {
            return &Output{
                Result: strings.ToLower(input.Message),
            }, nil
        },
    )

    worker, _ := client.NewWorker("simple-worker",
        hatchet.WithWorkflows(task),
    )

    // Blocking start (for main process)
    worker.StartBlocking(context.Background())
}
```

## DAG Workflow with Dependencies

```go
package main

import (
    "context"
    "fmt"

    "github.com/hatchet-dev/hatchet/sdks/go"
    "github.com/hatchet-dev/hatchet/sdks/go/pkg/factory"
    "github.com/hatchet-dev/hatchet/sdks/go/pkg/factory/create"
)

type PipelineInput struct {
    Source string `json:"source"`
}

type ExtractOutput struct {
    RawData string `json:"raw_data"`
}

type TransformOutput struct {
    Transformed string `json:"transformed"`
}

type LoadOutput struct {
    Loaded bool `json:"loaded"`
}

func main() {
    client, _ := hatchet.NewClient()
    defer client.Close()

    // Create workflow
    pipeline := factory.NewWorkflow[PipelineInput, LoadOutput](
        create.WorkflowCreateOpts[PipelineInput]{
            Name: "data-pipeline",
        },
        client,
    )

    // Step 1: Extract
    extract := pipeline.NewTask("extract",
        func(ctx hatchet.Context, input PipelineInput) (ExtractOutput, error) {
            ctx.Log("Extracting data...")
            return ExtractOutput{RawData: input.Source}, nil
        },
    )

    // Step 2: Transform (depends on extract)
    transform := pipeline.NewTask("transform",
        func(ctx hatchet.Context, input PipelineInput) (TransformOutput, error) {
            var extractOut ExtractOutput
            if err := ctx.ParentOutput(extract, &extractOut); err != nil {
                return TransformOutput{}, err
            }
            ctx.Log(fmt.Sprintf("Transforming: %s", extractOut.RawData))
            return TransformOutput{
                Transformed: strings.ToUpper(extractOut.RawData),
            }, nil
        },
        hatchet.WithParents(extract),
    )

    // Step 3: Load (depends on transform)
    pipeline.NewTask("load",
        func(ctx hatchet.Context, input PipelineInput) (LoadOutput, error) {
            var transformOut TransformOutput
            if err := ctx.ParentOutput(transform, &transformOut); err != nil {
                return LoadOutput{}, err
            }
            ctx.Log(fmt.Sprintf("Loading: %s", transformOut.Transformed))
            return LoadOutput{Loaded: true}, nil
        },
        hatchet.WithParents(transform),
    )

    // Start worker
    worker, _ := client.NewWorker("pipeline-worker",
        hatchet.WithWorkflows(pipeline),
    )
    worker.StartBlocking(context.Background())
}
```

## Parallel Fan-Out

```go
type UsersOutput struct {
    Users []string `json:"users"`
}

type BatchOutput struct {
    Batch string `json:"batch"`
    Count int    `json:"count"`
}

type AggregateOutput struct {
    Total int `json:"total"`
}

func setupParallelWorkflow(client *hatchet.Client) *factory.Workflow[Input, AggregateOutput] {
    workflow := factory.NewWorkflow[Input, AggregateOutput](
        create.WorkflowCreateOpts[Input]{Name: "parallel-pipeline"},
        client,
    )

    fetchUsers := workflow.NewTask("fetch-users",
        func(ctx hatchet.Context, input Input) (UsersOutput, error) {
            return UsersOutput{Users: []string{"user1", "user2", "user3"}}, nil
        },
    )

    // These run in parallel (same parent)
    batchA := workflow.NewTask("batch-a",
        func(ctx hatchet.Context, input Input) (BatchOutput, error) {
            var users UsersOutput
            ctx.ParentOutput(fetchUsers, &users)
            return BatchOutput{Batch: "a", Count: len(users.Users[:2])}, nil
        },
        hatchet.WithParents(fetchUsers),
    )

    batchB := workflow.NewTask("batch-b",
        func(ctx hatchet.Context, input Input) (BatchOutput, error) {
            var users UsersOutput
            ctx.ParentOutput(fetchUsers, &users)
            return BatchOutput{Batch: "b", Count: len(users.Users[2:])}, nil
        },
        hatchet.WithParents(fetchUsers),
    )

    // Waits for both parallel tasks
    workflow.NewTask("aggregate",
        func(ctx hatchet.Context, input Input) (AggregateOutput, error) {
            var a, b BatchOutput
            ctx.ParentOutput(batchA, &a)
            ctx.ParentOutput(batchB, &b)
            return AggregateOutput{Total: a.Count + b.Count}, nil
        },
        hatchet.WithParents(batchA, batchB),
    )

    return workflow
}
```

## Event-Triggered Task

```go
task := client.NewStandaloneTask("event-handler",
    func(ctx hatchet.Context, input EventInput) (*Output, error) {
        ctx.Log(fmt.Sprintf("Handling event for user: %s", input.UserID))
        return &Output{Processed: true}, nil
    },
    hatchet.WithWorkflowEvents("user:created", "user:updated"),
)

// Push events from your API
client.Event().Push(context.Background(), "user:created", map[string]interface{}{
    "user_id": "123",
})
```

## Cron-Triggered Task

```go
task := client.NewStandaloneTask("daily-cleanup",
    func(ctx hatchet.Context, input Input) (*Output, error) {
        // Runs daily at 9am
        return &Output{Cleaned: true}, nil
    },
    hatchet.WithCrons("0 9 * * *"),
)
```

## Error Handling and Retries

```go
import "time"

task := client.NewStandaloneTask("retryable-task",
    func(ctx hatchet.Context, input Input) (*Output, error) {
        resp, err := http.Get(input.URL)
        if err != nil {
            return nil, err // Will retry
        }
        if resp.StatusCode != 200 {
            return nil, fmt.Errorf("API returned %d", resp.StatusCode)
        }
        return &Output{Success: true}, nil
    },
    hatchet.WithRetries(3),
    hatchet.WithExecutionTimeout(5*time.Minute),
)
```

## Context Methods

```go
func taskHandler(ctx hatchet.Context, input Input) (*Output, error) {
    // Logging (visible in dashboard)
    ctx.Log("Processing started")
    ctx.Log(fmt.Sprintf("Input: %+v", input))

    // Get parent task output
    var parentOut ParentOutput
    if err := ctx.ParentOutput(parentTask, &parentOut); err != nil {
        return nil, err
    }

    // Access workflow run metadata
    runID := ctx.WorkflowRunID()
    ctx.Log(fmt.Sprintf("Run ID: %s", runID))

    // Refresh deadline for long operations
    ctx.RefreshTimeout(10 * time.Minute)

    return &Output{Done: true}, nil
}
```

## Worker Configuration

```go
worker, err := client.NewWorker("my-worker",
    hatchet.WithWorkflows(workflow1, workflow2, task1),
    hatchet.WithSlots(100),         // Concurrent task capacity
    hatchet.WithDurableSlots(1000), // Durable task capacity
    hatchet.WithLabels(map[string]string{
        "environment": "production",
        "region":      "us-west-2",
    }),
)
if err != nil {
    log.Fatal(err)
}

// Blocking start (recommended for main process)
err = worker.StartBlocking(context.Background())

// Or non-blocking with manual shutdown
worker.Start()
defer worker.Stop()
```

## Graceful Shutdown

```go
func main() {
    client, _ := hatchet.NewClient()
    defer client.Close()

    worker, _ := client.NewWorker("my-worker",
        hatchet.WithWorkflows(workflow),
    )

    // Handle shutdown signals
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigCh
        log.Println("Shutting down...")
        cancel()
    }()

    if err := worker.StartBlocking(ctx); err != nil && err != context.Canceled {
        log.Fatal(err)
    }
}
```

## Running Workflows Programmatically

```go
// Sync (wait for result)
result, err := workflow.Run(context.Background(), PipelineInput{
    Source: "raw data",
})
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Result: %+v\n", result)

// Async (fire and forget)
runRef, err := workflow.RunNoWait(context.Background(), PipelineInput{
    Source: "raw data",
})
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Started run: %s\n", runRef.RunID)
```

## Rate Limiting

```go
import "github.com/hatchet-dev/hatchet/sdks/go/pkg/types"

task := workflow.NewTask("rate-limited",
    func(ctx hatchet.Context, input Input) (Output, error) {
        return Output{}, nil
    },
    hatchet.WithRateLimits([]types.RateLimit{
        {
            Key:      "api-calls",
            Units:    1,
            Limit:    100,
            Duration: types.RateLimitDurationMinute,
        },
    }),
)
```

## Concurrency Control

```go
workflow := factory.NewWorkflow[Input, Output](
    create.WorkflowCreateOpts[Input]{
        Name: "controlled-workflow",
        Concurrency: &types.WorkflowConcurrency{
            Expression:    "input.user_id",
            MaxRuns:       5,
            LimitStrategy: types.ConcurrencyLimitStrategyGroupRoundRobin,
        },
    },
    client,
)
```
