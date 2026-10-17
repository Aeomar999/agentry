// Package rules is check 3 of every proposal: given what happened so far, may
// this happen now? The store runs it inside the proposal's transaction.
package rules

import (
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// Check is the store's OrderCheck.
func Check(p events.Proposal, history store.History) *events.Rejection {
	agentEvents, err := history.Stream(events.AgentsStream)
	if err != nil {
		return refuse("could not read the agents: " + err.Error())
	}
	roster := agents.FromEvents(agentEvents)
	if rule := rosterRules[p.Type]; rule != nil {
		if reason := rule(roster, p); reason != "" {
			return refuse(reason)
		}
	}
	if rule := workerRules[p.Type]; rule != nil {
		workerEvents, err := history.Stream(events.WorkersStream)
		if err != nil {
			return refuse("could not read the workers: " + err.Error())
		}
		if reason := rule(roster, workers.FromEvents(workerEvents), p); reason != "" {
			return refuse(reason)
		}
	}
	if rule := connectionRules[p.Type]; rule != nil {
		connectionEvents, err := history.Stream(events.ConnectionsStream)
		if err != nil {
			return refuse("could not read the connections: " + err.Error())
		}
		if reason := rule(connections.FromEvents(connectionEvents), p); reason != "" {
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
		if p.Actor.Kind == events.Agent && p.Text("parent") == "" && p.Text("epic") == "" {
			return refuse("an agent makes a task only for another task: a request to another team, or a story of an epic")
		}
		if kind := tasks.Kind(p.Text("kind")); kind != tasks.Story && kind != tasks.Epic {
			return refuse(`a task is a story ("") or an epic ("epic"), not ` + string(kind))
		}
		if p.Text("epic") != "" && p.Text("kind") == string(tasks.Epic) {
			return refuse("an epic can't be a story of another epic")
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

// afterDone are what may happen to a finished task: labels, order and epics
// sort finished work too, you can reopen it, and you can move it to the repo
// its work is really in, where it gets its worktree.
var afterDone = []events.Type{events.TaskLabeled, events.TaskRanked, events.TaskReopened, events.RepoChanged, events.WorktreeReady,
	events.AgentRenamedInTask,          // a finished task reopened later goes to its coordinator by its new name
	events.EpicJoined, events.EpicLeft} // finished work is filed under epics too

func refuse(reason string) *events.Rejection {
	return &events.Rejection{Check: "order", Reason: reason}
}
