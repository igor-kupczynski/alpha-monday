package worker

import (
	"testing"
)

func TestWorkflowRegistrationIDs(t *testing.T) {
	specs := workflowSpecs()

	weeklyFound := false
	dailyFound := false
	dailyLoopFound := false

	for _, spec := range specs {
		if spec.ID == WeeklyPickWorkflowID {
			weeklyFound = true
			for _, step := range spec.Steps {
				if step.ID == StepDailyCheckpointLoopID {
					dailyLoopFound = true
				}
			}
		}
		if spec.ID == DailyCheckpointWorkflowID {
			dailyFound = true
		}
	}

	if !weeklyFound {
		t.Fatalf("expected workflow %q to be registered", WeeklyPickWorkflowID)
	}

	if !dailyFound {
		t.Fatalf("expected workflow %q to be registered", DailyCheckpointWorkflowID)
	}

	if !dailyLoopFound {
		t.Fatalf("expected step %q to be registered", StepDailyCheckpointLoopID)
	}
}

func TestWorkflowRateLimitsConfigured(t *testing.T) {
	weekly := findWorkflowSpec(t, WeeklyPickWorkflowID)
	daily := findWorkflowSpec(t, DailyCheckpointWorkflowID)

	snapshotStep := findStepSpec(t, weekly, StepSnapshotPricesID)
	dailyTask := findStepSpec(t, daily, DailyCheckpointWorkflowID)

	assertRateLimit(t, snapshotStep, alphaVantageRateLimitMinuteKey, alphaVantageRateLimitUnits)
	assertRateLimit(t, snapshotStep, alphaVantageRateLimitDayKey, alphaVantageRateLimitUnits)
	assertRateLimit(t, dailyTask, alphaVantageRateLimitMinuteKey, alphaVantageRateLimitUnits)
	assertRateLimit(t, dailyTask, alphaVantageRateLimitDayKey, alphaVantageRateLimitUnits)
}

func TestWorkflowDurableLoopConfigured(t *testing.T) {
	weekly := findWorkflowSpec(t, WeeklyPickWorkflowID)
	dailyLoop := findStepSpec(t, weekly, StepDailyCheckpointLoopID)
	if !dailyLoop.Durable {
		t.Fatalf("expected step %q to be durable", StepDailyCheckpointLoopID)
	}
}

func findWorkflowSpec(t *testing.T, id string) workflowSpec {
	t.Helper()
	for _, spec := range workflowSpecs() {
		if spec.ID == id {
			return spec
		}
	}
	t.Fatalf("expected workflow %q to be registered", id)
	return workflowSpec{}
}

func findStepSpec(t *testing.T, spec workflowSpec, id string) stepSpec {
	t.Helper()
	for _, step := range spec.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("expected step %q to be present in workflow %q", id, spec.ID)
	return stepSpec{}
}

func assertRateLimit(t *testing.T, step stepSpec, key string, units int) {
	t.Helper()
	if len(step.RateLimits) == 0 {
		t.Fatalf("expected rate limit on step %q", step.ID)
	}
	var limit *rateLimitSpec
	for i := range step.RateLimits {
		if step.RateLimits[i].Key == key {
			limit = &step.RateLimits[i]
			break
		}
	}
	if limit == nil {
		t.Fatalf("expected rate limit key %q on step %q", key, step.ID)
	}
	if limit.Units != units {
		t.Fatalf("expected rate limit units %d, got %d", units, limit.Units)
	}
}
