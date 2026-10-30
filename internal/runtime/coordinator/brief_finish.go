package coordinator

import (
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
)

// finish is how the agent ends its turn: with a report, and a reviewer's
// verdict last.
func finish(b *strings.Builder, agent agents.Agent) {
	text := "End with a short report for the coordinator: what you did, what you found, and anything left or blocking you. " +
		"The coordinator reads only this report, so make it complete. Don't stop to ask questions: put them in the report."
	if agent.Column == agents.Reviews {
		text += "\n\n" + VerdictFormat
	}
	section(b, "When you finish", text)
}
