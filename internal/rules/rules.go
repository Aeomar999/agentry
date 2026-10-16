// Package rules is check 3 of every proposal: given what happened so far, may
// this happen now? The store runs it inside the proposal's transaction.
package rules

import (
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Check is the store's OrderCheck.
func Check(p events.Proposal, history store.History) *events.Rejection {
	agentEvents, err := history.Stream(events.AgentsStream)
	if err != nil {
		return refuse("could not read the agents: " + err.Error())
	}
	if rule := rosterRules[p.Type]; rule != nil {
		if reason := rule(agents.FromEvents(agentEvents), p); reason != "" {
			return refuse(reason)
		}
	}
	if !events.IsTaskStream(p.Stream) {
		return nil
	}

	stream, err := history.Stream(p.Stream)
	if err != nil {
		return refuse("could not read the task: " + err.Error())
	}
	task := tasks.FromEvents(stream)
	switch {
	case p.Type == events.TaskCreated && task != nil:
		return refuse("task " + p.Stream + " already exists")
	case p.Type == events.TaskCreated:
		if reason := CheckLabels(p.List("labels")); reason != "" {
			return refuse(reason)
		}
		return nil
	case task == nil:
		return refuse("task " + p.Stream + " doesn't exist")
	case task.IsDone && !slices.Contains(afterDone, p.Type):
		return refuse("task " + p.Stream + " is done")
	}
	if reason := taskRules[p.Type](task, p); reason != "" {
		return refuse(reason)
	}
	return nil
}

// afterDone are what may happen to a finished task: labels and order sort
// finished work too, and you can reopen it.
var afterDone = []events.Type{events.TaskLabeled, events.TaskRanked, events.TaskReopened}

func refuse(reason string) *events.Rejection {
	return &events.Rejection{Check: "order", Reason: reason}
}
