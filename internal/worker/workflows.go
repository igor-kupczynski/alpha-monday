package worker

import (
	"log/slog"

	hatchetworker "github.com/hatchet-dev/hatchet/pkg/worker"
)

const (
	WeeklyPickWorkflowID   = "weekly_pick_v1"
	DailyCheckpointStepID  = "daily_checkpoint_v1"
	StepGeneratePicksID    = "generate_picks"
	StepSnapshotPricesID   = "snapshot_initial_prices"
	StepPersistBatchID     = "persist_batch"
	weeklyPickCronSchedule = "0 9 * * 1"
)

// WeeklyPickState is the workflow state stored by Hatchet for the weekly workflow.
type WeeklyPickState struct {
	BatchID               string      `json:"batch_id"`
	RunDate               string      `json:"run_date"`
	BenchmarkSymbol       string      `json:"benchmark_symbol"`
	BenchmarkInitialPrice string      `json:"benchmark_initial_price"`
	Picks                 []PickState `json:"picks"`
}

type PickState struct {
	PickID       string `json:"pick_id"`
	Ticker       string `json:"ticker"`
	Action       string `json:"action"`
	Reasoning    string `json:"reasoning"`
	InitialPrice string `json:"initial_price"`
}

type workflowSpec struct {
	ID       string
	Schedule string
	Steps    []stepSpec
}

type stepSpec struct {
	ID string
}

func workflowSpecs() []workflowSpec {
	return []workflowSpec{
		{
			ID:       WeeklyPickWorkflowID,
			Schedule: weeklyPickCronSchedule,
			Steps: []stepSpec{
				{ID: StepGeneratePicksID},
				{ID: StepSnapshotPricesID},
				{ID: StepPersistBatchID},
				{ID: DailyCheckpointStepID},
			},
		},
	}
}

func buildWorkflow(spec workflowSpec, logger *slog.Logger, stepDeps *Steps) *hatchetworker.WorkflowJob {
	if logger == nil {
		logger = slog.Default()
	}

	workflowSteps := make([]*hatchetworker.WorkflowStep, 0, len(spec.Steps))
	var previous *hatchetworker.WorkflowStep
	handlers := stepHandlers(stepDeps, logger)
	for _, step := range spec.Steps {
		handler := handlers[step.ID]
		if handler == nil {
			handler = noopStep(logger, step.ID)
		}
		current := hatchetworker.Fn(handler).SetName(step.ID)
		if previous != nil {
			current.AddParents(previous.Name)
		}
		workflowSteps = append(workflowSteps, current)
		previous = current
	}

	job := &hatchetworker.WorkflowJob{
		Name:  spec.ID,
		Steps: workflowSteps,
	}

	if spec.Schedule != "" {
		job.On = hatchetworker.Cron(spec.Schedule)
	}

	return job
}

func stepHandlers(steps *Steps, logger *slog.Logger) map[string]any {
	handlers := map[string]any{}
	if steps != nil {
		handlers[StepGeneratePicksID] = steps.GeneratePicks
		handlers[StepSnapshotPricesID] = steps.SnapshotInitialPrices
		handlers[StepPersistBatchID] = steps.PersistBatch
	}
	handlers[DailyCheckpointStepID] = noopStep(logger, DailyCheckpointStepID)
	return handlers
}

func noopStep(logger *slog.Logger, stepName string) func(hatchetworker.HatchetContext) error {
	return func(ctx hatchetworker.HatchetContext) error {
		if logger == nil {
			logger = slog.Default()
		}
		logger.Info("step stub", "step", stepName)
		return nil
	}
}
