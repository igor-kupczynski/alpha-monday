package worker

import (
	"testing"

	hatchetworker "github.com/hatchet-dev/hatchet/pkg/worker"
)

func TestWorkflowRegistrationIDs(t *testing.T) {
	specs := workflowSpecs()

	weeklyFound := false
	dailyStepFound := false

	for _, spec := range specs {
		if spec.ID == WeeklyPickWorkflowID {
			weeklyFound = true
			for _, step := range spec.Steps {
				if step.ID == DailyCheckpointStepID {
					dailyStepFound = true
				}
			}
		}
	}

	if !weeklyFound {
		t.Fatalf("expected workflow %q to be registered", WeeklyPickWorkflowID)
	}

	if !dailyStepFound {
		t.Fatalf("expected step %q to be registered", DailyCheckpointStepID)
	}
}

func TestWorkflowRateLimitsConfigured(t *testing.T) {
	var spec workflowSpec
	for _, candidate := range workflowSpecs() {
		if candidate.ID == WeeklyPickWorkflowID {
			spec = candidate
			break
		}
	}
	if spec.ID == "" {
		t.Fatalf("expected workflow %q to be registered", WeeklyPickWorkflowID)
	}

	workflow := buildWorkflow(spec, nil, nil)
	var snapshotStep *hatchetworker.WorkflowStep
	var dailyStep *hatchetworker.WorkflowStep
	for _, step := range workflow.Steps {
		if step.Name == StepSnapshotPricesID {
			snapshotStep = step
		}
		if step.Name == DailyCheckpointStepID {
			dailyStep = step
		}
	}

	if snapshotStep == nil {
		t.Fatalf("expected step %q to be present", StepSnapshotPricesID)
	}
	if dailyStep == nil {
		t.Fatalf("expected step %q to be present", DailyCheckpointStepID)
	}

	assertRateLimit(t, snapshotStep, alphaVantageRateLimitMinuteKey, alphaVantageRateLimitUnits)
	assertRateLimit(t, snapshotStep, alphaVantageRateLimitDayKey, alphaVantageRateLimitUnits)
	assertRateLimit(t, dailyStep, alphaVantageRateLimitMinuteKey, alphaVantageRateLimitUnits)
	assertRateLimit(t, dailyStep, alphaVantageRateLimitDayKey, alphaVantageRateLimitUnits)
}

func assertRateLimit(t *testing.T, step *hatchetworker.WorkflowStep, key string, units int) {
	t.Helper()
	if len(step.RateLimit) == 0 {
		t.Fatalf("expected rate limit on step %q", step.Name)
	}
	var limit *hatchetworker.RateLimit
	for i := range step.RateLimit {
		if step.RateLimit[i].Key == key {
			limit = &step.RateLimit[i]
			break
		}
	}
	if limit == nil {
		t.Fatalf("expected rate limit key %q on step %q", key, step.Name)
	}
	if limit.Units == nil || *limit.Units != units {
		if limit.Units == nil {
			t.Fatalf("expected rate limit units %d, got nil", units)
		}
		t.Fatalf("expected rate limit units %d, got %d", units, *limit.Units)
	}
}
