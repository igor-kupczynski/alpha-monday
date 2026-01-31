package worker

import (
	"fmt"
	"log/slog"

	"github.com/hatchet-dev/hatchet/pkg/client/types"
	hatchet "github.com/hatchet-dev/hatchet/sdks/go"
)

const (
	WeeklyPickWorkflowID           = "weekly_pick_v1"
	DailyCheckpointWorkflowID      = "daily_checkpoint_v1"
	StepGeneratePicksID            = "generate_picks"
	StepSnapshotPricesID           = "snapshot_initial_prices"
	StepPersistBatchID             = "persist_batch"
	StepDailyCheckpointLoopID      = "daily_checkpoint_loop"
	weeklyPickCronSchedule         = "0 9 * * 1"
	alphaVantageRateLimitMinuteKey = "alpha_vantage_minute"
	alphaVantageRateLimitDayKey    = "alpha_vantage_day"
	alphaVantageRateLimitUnits     = 4
	alphaVantageRateLimitMaxMinute = 5
	alphaVantageRateLimitMaxDay    = 500
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
	ID         string
	Cron       string
	Standalone bool
	Steps      []stepSpec
}

type stepSpec struct {
	ID         string
	Durable    bool
	RateLimits []rateLimitSpec
}

type rateLimitSpec struct {
	Key   string
	Units int
}

func workflowSpecs() []workflowSpec {
	return []workflowSpec{
		weeklyWorkflowSpec(),
		dailyCheckpointWorkflowSpec(),
	}
}

func weeklyWorkflowSpec() workflowSpec {
	return workflowSpec{
		ID:   WeeklyPickWorkflowID,
		Cron: weeklyPickCronSchedule,
		Steps: []stepSpec{
			{ID: StepGeneratePicksID},
			{ID: StepSnapshotPricesID, RateLimits: alphaVantageRateLimitSpecs()},
			{ID: StepPersistBatchID},
			{ID: StepDailyCheckpointLoopID, Durable: true},
		},
	}
}

func dailyCheckpointWorkflowSpec() workflowSpec {
	return workflowSpec{
		ID:         DailyCheckpointWorkflowID,
		Standalone: true,
		Steps: []stepSpec{
			{ID: DailyCheckpointWorkflowID, RateLimits: alphaVantageRateLimitSpecs()},
		},
	}
}

func BuildWorkflows(client *hatchet.Client, logger *slog.Logger, steps *Steps) ([]hatchet.WorkflowBase, error) {
	if client == nil {
		return nil, fmt.Errorf("hatchet client is required")
	}
	if steps == nil {
		return nil, fmt.Errorf("steps are required")
	}

	handlers := stepHandlers(steps, logger)
	workflows := make([]hatchet.WorkflowBase, 0, len(workflowSpecs()))

	for _, spec := range workflowSpecs() {
		if spec.Standalone {
			if len(spec.Steps) != 1 {
				return nil, fmt.Errorf("standalone workflow %q must define exactly one step", spec.ID)
			}
			step := spec.Steps[0]
			if step.ID != spec.ID {
				return nil, fmt.Errorf("standalone workflow %q step id must match workflow id", spec.ID)
			}
			handler := handlers[step.ID]
			if handler == nil {
				return nil, fmt.Errorf("missing handler for step %q", step.ID)
			}
			opts := taskOptionsFromStep(step, nil)
			standaloneOpts := make([]hatchet.StandaloneTaskOption, 0, len(opts))
			for _, opt := range opts {
				standaloneOpts = append(standaloneOpts, opt)
			}
			workflows = append(workflows, client.NewStandaloneTask(step.ID, handler, standaloneOpts...))
			continue
		}

		workflow := client.NewWorkflow(spec.ID, workflowOptionsFromSpec(spec)...)
		var previous *hatchet.Task
		for _, step := range spec.Steps {
			handler := handlers[step.ID]
			if handler == nil {
				return nil, fmt.Errorf("missing handler for step %q", step.ID)
			}
			opts := taskOptionsFromStep(step, previous)
			var task *hatchet.Task
			if step.Durable {
				task = workflow.NewDurableTask(step.ID, handler, opts...)
			} else {
				task = workflow.NewTask(step.ID, handler, opts...)
			}
			previous = task
		}
		workflows = append(workflows, workflow)
	}

	return workflows, nil
}

func workflowOptionsFromSpec(spec workflowSpec) []hatchet.WorkflowOption {
	opts := []hatchet.WorkflowOption{}
	if spec.Cron != "" {
		opts = append(opts, hatchet.WithWorkflowCron(spec.Cron))
	}
	return opts
}

func taskOptionsFromStep(step stepSpec, parent *hatchet.Task) []hatchet.TaskOption {
	opts := []hatchet.TaskOption{}
	if parent != nil {
		opts = append(opts, hatchet.WithParents(parent))
	}
	if len(step.RateLimits) > 0 {
		opts = append(opts, hatchet.WithRateLimits(rateLimitSpecsToTypes(step.RateLimits)...))
	}
	return opts
}

func alphaVantageRateLimitSpecs() []rateLimitSpec {
	return []rateLimitSpec{
		{Key: alphaVantageRateLimitMinuteKey, Units: alphaVantageRateLimitUnits},
		{Key: alphaVantageRateLimitDayKey, Units: alphaVantageRateLimitUnits},
	}
}

func rateLimitSpecsToTypes(specs []rateLimitSpec) []*types.RateLimit {
	limits := make([]*types.RateLimit, 0, len(specs))
	for _, spec := range specs {
		units := spec.Units
		limits = append(limits, &types.RateLimit{
			Key:   spec.Key,
			Units: &units,
		})
	}
	return limits
}

func stepHandlers(steps *Steps, logger *slog.Logger) map[string]any {
	if logger == nil {
		logger = slog.Default()
	}
	return map[string]any{
		StepGeneratePicksID:       withWorkflowLogging(logger, steps.GeneratePicks),
		StepSnapshotPricesID:      withWorkflowLogging(logger, steps.SnapshotInitialPrices),
		StepPersistBatchID:        withWorkflowLogging(logger, steps.PersistBatch),
		StepDailyCheckpointLoopID: withDurableWorkflowLogging(logger, steps.DailyCheckpointLoop),
		DailyCheckpointWorkflowID: withWorkflowLogging(logger, steps.DailyCheckpoint),
	}
}
