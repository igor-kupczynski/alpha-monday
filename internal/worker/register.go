package worker

import (
	"fmt"
	"log/slog"

	hatchetworker "github.com/hatchet-dev/hatchet/pkg/worker"
)

func RegisterWorkflows(w *hatchetworker.Worker, logger *slog.Logger) error {
	if w == nil {
		return fmt.Errorf("worker is nil")
	}
	if logger == nil {
		logger = slog.Default()
	}

	for _, spec := range workflowSpecs() {
		workflow := buildWorkflow(spec, logger)
		if err := w.RegisterWorkflow(workflow); err != nil {
			return fmt.Errorf("register workflow %s: %w", spec.ID, err)
		}
		logger.Info("workflow registered", "workflow", spec.ID)
	}

	return nil
}
