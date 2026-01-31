package worker

import (
	"testing"

	"github.com/hatchet-dev/hatchet/pkg/client/types"
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

	assertRateLimit(t, snapshotStep, alphaVantageRateLimitKey, alphaVantageRateLimitUnits, types.Minute)
	assertRateLimit(t, dailyStep, alphaVantageRateLimitKey, alphaVantageRateLimitUnits, types.Minute)
}

func assertRateLimit(t *testing.T, step *hatchetworker.WorkflowStep, key string, units int, duration types.RateLimitDuration) {
	t.Helper()
	if len(step.RateLimit) == 0 {
		t.Fatalf("expected rate limit on step %q", step.Name)
	}
	limit := step.RateLimit[0]
	if limit.Key != key {
		t.Fatalf("expected rate limit key %q, got %q", key, limit.Key)
	}
	if limit.Units == nil || *limit.Units != units {
		if limit.Units == nil {
			t.Fatalf("expected rate limit units %d, got nil", units)
		}
		t.Fatalf("expected rate limit units %d, got %d", units, *limit.Units)
	}
	if limit.Duration == nil || *limit.Duration != duration {
		if limit.Duration == nil {
			t.Fatalf("expected rate limit duration %v, got nil", duration)
		}
		t.Fatalf("expected rate limit duration %v, got %v", duration, *limit.Duration)
	}
}
