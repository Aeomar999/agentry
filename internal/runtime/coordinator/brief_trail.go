package coordinator

import (
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// longestStep caps each trail entry in the coordinator's brief.
const longestStep = 4000

// trailText numbers the steps, the first being step first+1 of the trail.
func trailText(steps []tasks.Step, first int) string {
	var trail strings.Builder
	for i, step := range steps {
		fmt.Fprintf(&trail, "%d. %s → %s: %s\n\n", first+i+1, step.From, step.To, tasks.Shorten(step.Text, longestStep))
	}
	return trail.String()
}

// since is the trail after its first seen steps.
func since(trail []tasks.Step, seen int) []tasks.Step {
	return trail[min(seen, len(trail)):]
}
