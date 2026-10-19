// Package rules is check 3 of every proposal: given what happened so far, may
// this happen now? The store runs it inside Propose, under its lock, so the
// state it reads can't change while it does.
package rules

import (
	"cmp"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// State is what the rules read: the state the proposal would be appended to.
// *current.State is one. Broken is why a roster's stream could not be built,
// or nil: the rules refuse what would be checked against a broken roster.
type State interface {
	Task(id string) (*tasks.Task, error)
	Idea(id string) (*ideas.Idea, error)
	Ideas() []*ideas.Idea
	Roster() agents.Roster
	Settings() settings.Settings
	Workers() workers.Roster
	Connections() connections.Roster
	Broken(stream string) error
}

// Check is the store's OrderCheck, made with the state it reads.
func Check(p events.Proposal, state State) *events.Rejection {
	if err := state.Broken(events.AgentsStream); err != nil {
		return refuse("could not read the agents: " + err.Error())
	}
	roster := state.Roster()
	if rule := rosterRules[p.Type]; rule != nil {
		if reason := rule(roster, p); reason != "" {
			return refuse(reason)
		}
	}
	if rule := workerRules[p.Type]; rule != nil {
		if err := state.Broken(events.WorkersStream); err != nil {
			return refuse("could not read the workers: " + err.Error())
		}
		if reason := rule(roster, state.Workers(), p); reason != "" {
			return refuse(reason)
		}
	}
	if rule := connectionRules[p.Type]; rule != nil {
		if err := state.Broken(events.ConnectionsStream); err != nil {
			return refuse("could not read the connections: " + err.Error())
		}
		if reason := rule(state.Connections(), p); reason != "" {
			return refuse(reason)
		}
	}
	if events.IsIdeaStream(p.Stream) {
		return checkIdea(p, state)
	}
	if !events.IsTaskStream(p.Stream) {
		return nil
	}

	task, err := state.Task(p.Stream)
	if err != nil {
		return refuse("could not read the task: " + err.Error())
	}
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
		if p.Actor.Kind == events.System && p.Text("about") == "" {
			if reason := forAnOpenIdea(p, state); reason != "" {
				return refuse(reason)
			}
		}
		if reason := cmp.Or(aBudget(nil, p), aTurnLimit(nil, p)); reason != "" {
			return refuse(reason)
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
	case task.IsDone && task.WrapUp != nil && slices.Contains(duringWrapUp, p.Type):
	case task.IsDone && !slices.Contains(afterDone, p.Type):
		if reason := notUndoingYourMove(task, p); reason != "" {
			return refuse(reason)
		}
		return refuse("task " + p.Stream + " is done")
	}
	if reason := taskRules[p.Type](task, p); reason != "" {
		return refuse(reason)
	}
	if p.Type == events.TaskHeld && p.Actor.Kind == events.System {
		if reason := startingAnAcceptedIdea(task, p, state); reason != "" {
			return refuse(reason)
		}
	}
	if rule := taskRosterRules[p.Type]; rule != nil {
		if reason := rule(task, roster, p); reason != "" {
			return refuse(reason)
		}
	}
	return nil
}

// afterDone are what may happen to a finished task: labels, order and epics
// sort finished work too, you can reopen it, and you can move it to the repo
// its work is really in, where it gets its worktree.
var afterDone = []events.Type{events.TaskLabeled, events.TaskRanked, events.TaskReopened, events.RepoChanged, events.WorktreeReady,
	events.AgentRenamedInTask,          // a finished task reopened later goes to its coordinator by its new name
	events.EpicJoined, events.EpicLeft, // finished work is filed under epics too
	events.FollowUpsSkipped} // an epic you closed: why no Visionaries session started after it

// duringWrapUp are what may happen to a task you moved to Done while its
// coordinator wraps up: its one turn, the shared files around it, and
// Agentry asking you for what the turn needs to start (a machine online).
var duringWrapUp = []events.Type{events.TaskNeedsYou, events.TaskAnswered, events.TurnStarted, events.PromptAcknowledged, events.SessionStarted,
	events.ActionRequested, events.ActionAsked, events.ActionApproved, events.ActionDenied, events.ActionResult,
	events.PlanUpdated, events.AgentWaiting, events.TurnEnded, events.IssueRaised, events.TurnStopped, events.TaskWrappedUp,
	events.WorktreeMoved, events.FileAdded, events.FileRemoved, events.FilesCollected, events.FilesDelivered}

func refuse(reason string) *events.Rejection {
	return &events.Rejection{Check: "order", Reason: reason}
}
