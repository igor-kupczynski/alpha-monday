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

func buildWorkflow(spec workflowSpec, logger *slog.Logger) *hatchetworker.WorkflowJob {
	if logger == nil {
		logger = slog.Default()
	}

	steps := make([]*hatchetworker.WorkflowStep, 0, len(spec.Steps))
	var previous *hatchetworker.WorkflowStep
	for _, step := range spec.Steps {
		current := hatchetworker.Fn(noopStep(logger, step.ID)).SetName(step.ID)
		if previous != nil {
			current.AddParents(previous.Name)
		}
		steps = append(steps, current)
		previous = current
	}

	job := &hatchetworker.WorkflowJob{
		Name:  spec.ID,
		Steps: steps,
	}

	if spec.Schedule != "" {
		job.On = hatchetworker.Cron(spec.Schedule)
	}

	return job
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
