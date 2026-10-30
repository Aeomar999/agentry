package waitlist

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/runtime/coordinator"
)

func TestTheApprovingAnswersAreTheCoordinators(t *testing.T) {
	if CloseEpic != coordinator.CloseEpic || CopyFile != coordinator.CopyFile {
		t.Errorf("the approving answers differ: %q/%q here, %q/%q in the coordinator", CloseEpic, CopyFile, coordinator.CloseEpic, coordinator.CopyFile)
	}
}

// What waits names an epic as epic-N, and a story or a task on its own as task-N.
func TestALineNamesAnEpicAsEpicN(t *testing.T) {
	for _, c := range []struct {
		q    Question
		want string
	}{
		{Question{Kind: "question", Task: "task-21", TaskTitle: "Critique", TaskKind: "epic", Agent: "tech-lead", Text: "Close it?", Options: []string{CloseEpic}},
			"epic-21: the epic Critique is ready to close (close_epic)"},
		{Question{Kind: "question", Task: "task-21", TaskKind: "epic", Agent: "tech-lead", Text: "Which first?"}, "epic-21: tech-lead asks: Which first?"},
		{Question{Kind: "question", Task: "task-22", Agent: "tech-lead", Text: "Which port?"}, "task-22: tech-lead asks: Which port?"},
	} {
		if got := Line(c.q); got != c.want {
			t.Errorf("Line(%+v) = %q, want %q", c.q, got, c.want)
		}
	}
}
