package worker

import (
	"log/slog"
	"time"

	hatchet "github.com/hatchet-dev/hatchet/sdks/go"
)

type workflowLogContext interface {
	WorkflowRunId() string
	StepName() string
	StepRunId() string
	RetryCount() int
}

func withWorkflowLogging[I any, O any](logger *slog.Logger, fn func(hatchet.Context, I) (O, error)) func(hatchet.Context, I) (O, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx hatchet.Context, input I) (O, error) {
		start := time.Now()
		fields := workflowLogFields(ctx)
		logger.Info("workflow step started", fields...)

		output, err := fn(ctx, input)
		duration := time.Since(start)

		if err != nil {
			logger.Error("workflow step failed", append(fields, "duration_ms", duration.Milliseconds(), "error", err)...)
			return output, err
		}

		logger.Info("workflow step completed", append(fields, "duration_ms", duration.Milliseconds())...)
		return output, nil
	}
}

func withDurableWorkflowLogging[I any, O any](logger *slog.Logger, fn func(hatchet.DurableContext, I) (O, error)) func(hatchet.DurableContext, I) (O, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx hatchet.DurableContext, input I) (O, error) {
		start := time.Now()
		fields := workflowLogFields(ctx)
		logger.Info("workflow step started", fields...)

		output, err := fn(ctx, input)
		duration := time.Since(start)

		if err != nil {
			logger.Error("workflow step failed", append(fields, "duration_ms", duration.Milliseconds(), "error", err)...)
			return output, err
		}

		logger.Info("workflow step completed", append(fields, "duration_ms", duration.Milliseconds())...)
		return output, nil
	}
}

func workflowLogFields(ctx workflowLogContext) []any {
	fields := []any{
		"workflow_run_id", ctx.WorkflowRunId(),
		"step_name", ctx.StepName(),
		"step_run_id", ctx.StepRunId(),
		"retry_count", ctx.RetryCount(),
	}
	return fields
}
