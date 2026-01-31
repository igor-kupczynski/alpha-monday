package worker

import (
	"log/slog"
	"time"

	hatchetworker "github.com/hatchet-dev/hatchet/pkg/worker"
)

func WorkflowLogger(logger *slog.Logger) hatchetworker.MiddlewareFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx hatchetworker.HatchetContext, next func(hatchetworker.HatchetContext) error) error {
		start := time.Now()
		fields := workflowLogFields(ctx)
		logger.Info("workflow step started", fields...)

		err := next(ctx)
		duration := time.Since(start)

		if err != nil {
			logger.Error("workflow step failed", append(fields, "duration_ms", duration.Milliseconds(), "error", err)...)
			return err
		}

		logger.Info("workflow step completed", append(fields, "duration_ms", duration.Milliseconds())...)
		return nil
	}
}

func workflowLogFields(ctx hatchetworker.HatchetContext) []any {
	fields := []any{
		"workflow_run_id", ctx.WorkflowRunId(),
		"step_name", ctx.StepName(),
		"step_run_id", ctx.StepRunId(),
		"retry_count", ctx.RetryCount(),
	}
	return fields
}
