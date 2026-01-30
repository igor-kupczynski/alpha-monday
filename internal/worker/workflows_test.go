package worker

import "testing"

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
