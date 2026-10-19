package waitlist

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/coordinator"
)

func TestTheApprovingAnswersAreTheCoordinators(t *testing.T) {
	if CloseEpic != coordinator.CloseEpic || CopyFile != coordinator.CopyFile {
		t.Errorf("the approving answers differ: %q/%q here, %q/%q in the coordinator", CloseEpic, CopyFile, coordinator.CloseEpic, coordinator.CopyFile)
	}
}
