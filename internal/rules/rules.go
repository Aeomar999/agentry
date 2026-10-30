// Package rules is check 3 of every proposal: given what happened so far, may
// this happen now? The store runs it inside Propose, under its lock, so the
// state it reads can't change while it does.
package rules

import (
	"cmp"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// State is what the rules read: the state the proposal would be appended to,
// as an extension reads it. *current.State is one. The rules refuse what would
// be checked against a broken roster.
type State = extensions.Reader

// Check is the store's OrderCheck, made with the state it reads, the
// catalogue the store has, which says which streams are tasks, and the
// extensions installed. Nothing goes into a foreign stream: its extension isn't
// in this build. An extension's rules run only once the core's pass, so they
// can only refuse: its Streams.Check for a proposal on one of its streams,
// and every extension's Guards for the proposal's type.
func Check(p events.Proposal, state State, catalogue *events.Catalogue, installed ...extensions.Extension) *events.Rejection {
	if rejection := checkCore(p, state, catalogue, installed); rejection != nil {
		return rejection
	}
	for _, f := range installed {
		if guard := f.Guards[p.Type]; guard != nil {
			if rejection := byExtension(f, guard, p, state); rejection != nil {
				return rejection
			}
		}
	}
	return nil
}

// checkCore is Check without the Guards: the core's rules, and the owning
// extension's Streams.Check once they pass.
func checkCore(p events.Proposal, state State, catalogue *events.Catalogue, installed []extensions.Extension) *events.Rejection {
	if catalogue.KindOf(p.Stream) == events.ForeignStream {
		return refuse(events.ForeignReason(p.Stream))
	}
	if p.Type == events.EventRedacted {
		if reason := aRedaction(p); reason != "" {
			return refuse(reason)
		}
	}
	if err := state.Broken(events.AgentsStream); err != nil {
		return refuse("could not read the agents: " + err.Error())
	}
	roster := state.Roster()
	if rule := rosterRules[p.Type]; rule != nil {
		if reason := rule(roster, p); reason != "" {
			return refuse(reason)
		}
	}
	if p.Type == events.TeamEdited { // the one roster rule that reads the tasks too
		if reason := teamEdited(state, p); reason != "" {
			return refuse(reason)
		}
	}
	if p.Type == events.TeamLinked { // the one roster rule that reads the templates too
		if err := state.Broken(events.TemplatesStream); err != nil {
			return refuse("could not read the templates: " + err.Error())
		}
		if reason := teamLinked(roster, state.Templates(), state.RemovedTemplates(), p); reason != "" {
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
	if rule := templateRules[p.Type]; rule != nil {
		if err := state.Broken(events.TemplatesStream); err != nil {
			return refuse("could not read the templates: " + err.Error())
		}
		if reason := rule(state.Templates(), state.RemovedTemplates(), p); reason != "" {
			return refuse(reason)
		}
	}
	if catalogue.KindOf(p.Stream) == events.OwnedStream {
		return checkOwned(p, state, installed)
	}
	if !catalogue.IsTaskStream(p.Stream) {
		return nil
	}

	task, err := state.Task(p.Stream)
	switch {
	case errors.Is(err, current.ErrErased):
		return refuse(ErasedRefusal(p.Stream))
	case p.Type == events.TaskErased && task == nil && err == nil:
		return refuse("task " + p.Stream + " doesn't exist")
	case p.Type == events.TaskErased:
		if reason := anErasure(p); reason != "" {
			return refuse(reason)
		}
		return nil
	case errors.Is(err, current.ErrArchived) && p.Type == events.TaskUnarchived:
		if reason := anUnarchiving(p); reason != "" {
			return refuse(reason)
		}
		return nil
	case errors.Is(err, current.ErrArchived) && p.Type == events.EventRedacted:
		return refuse(store.ArchivedRedactionRefusal(p.Stream))
	case errors.Is(err, current.ErrArchived):
		return refuse(ArchivedRefusal(p.Stream))
	case p.Type == events.EventRedacted && task == nil && err == nil:
		return refuse("task " + p.Stream + " doesn't exist")
	case p.Type == events.EventRedacted:
		return nil // whole, done or broken: its secret goes all the same
	case err != nil:
		return refuse("could not read the task: " + err.Error())
	}
	switch {
	case p.Type == events.TaskCreated && task != nil:
		return refuse("task " + p.Stream + " already exists")
	case p.Type == events.TaskCreated:
		if next := state.NextTask(); p.Stream != next {
			return refuse("a new task goes into " + next + ", the next task stream, not " + p.Stream)
		}
		if reason := CheckLabels(p.List("labels")); reason != "" {
			return refuse(reason)
		}
		if from, idea := p.Text("from"), p.Text("idea"); from != "" && idea != "" && from != idea {
			return refuse("a task is made from one stream: its \"from\" and \"idea\" differ")
		}
		if p.Actor.Kind == events.Agent && p.Text("parent") == "" && p.Text("epic") == "" {
			return refuse("an agent makes a task only for another task: a request to another team, or a story of an epic")
		}
		if p.Actor.Kind == events.System && p.Text("about") == "" && !guarded(installed, tasks.MadeFromOf(p), p.Type) {
			return refuse("Agentry makes a task only about an epic you closed, or from an idea the Visionaries agreed on")
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
	if p.Type == events.TaskHeld && p.Actor.Kind == events.System && !guarded(installed, task.MadeFrom, p.Type) {
		return refuse("Agentry only starts, once, a task it made from an idea it accepted")
	}
	if rule := taskRosterRules[p.Type]; rule != nil {
		if reason := rule(task, roster, p); reason != "" {
			return refuse(reason)
		}
	}
	return nil
}

// checkOwned runs the Streams.Check of the extension that owns p's stream. The
// catalogue registers only installed extensions' streams, so one with no
// owner here is refused rather than let through unchecked.
func checkOwned(p events.Proposal, state State, installed []extensions.Extension) *events.Rejection {
	owner, found := ownerOf(installed, p.Stream)
	switch {
	case !found:
		return refuse("no installed feature checks stream " + p.Stream)
	case owner.Streams.Check == nil:
		return nil
	}
	return byExtension(owner, owner.Streams.Check, p, state)
}

// guarded says whether stream belongs to an installed extension with a Guard
// for kind: the core lets Agentry make or start a task from an extension's
// stream only when that extension checks it.
func guarded(installed []extensions.Extension, stream string, kind events.Type) bool {
	owner, found := ownerOf(installed, stream)
	return found && owner.Guards[kind] != nil
}

// ownerOf is the installed extension whose streams stream is one of.
func ownerOf(installed []extensions.Extension, stream string) (extensions.Extension, bool) {
	for _, f := range installed {
		if f.Streams != nil && f.Streams.Prefix != "" && strings.HasPrefix(stream, f.Streams.Prefix) {
			return f, true
		}
	}
	return extensions.Extension{}, false
}

// byExtension runs one of f's rules. One that panics refuses the proposal,
// and is logged, so a broken extension stops only what it checks: the store
// unlocks and goes on as after any refusal.
func byExtension(f extensions.Extension, rule extensions.Rule, p events.Proposal, state State) (rejection *events.Rejection) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Warn("rules: a feature's rule panicked", "feature", f.Name, "type", p.Type, "stream", p.Stream, "panic", recovered)
			rejection = refuse("the " + f.Name + " feature's rules failed on this " + string(p.Type) + ", so it is refused")
		}
	}()
	return rule(p, state)
}

// afterDone are what may happen to a finished task: labels, order and epics
// sort finished work too, you can reopen it, and you can move it to the repo
// its work is really in, where it gets its worktree. A finished epic lets go
// of a story: you move one out of it or erase one, and one erased while the
// epic was archived leaves it as it is reopened.
var afterDone = []events.Type{events.TaskLabeled, events.TaskRanked, events.TaskReopened, events.RepoChanged, events.WorktreeReady,
	events.AgentRenamedInTask,          // a finished task reopened later goes to its coordinator by its new name
	events.EpicJoined, events.EpicLeft, // finished work is filed under epics too
	events.StoryRemoved,                        // a story moved out of a finished epic, or erased
	events.FollowUpsSkipped,                    // an epic you closed: why no Visionaries session started after it
	events.TaskArchived, events.TaskUnarchived} // its rows moved out of the live log, and back

// duringWrapUp are what may happen to a task you moved to Done while its
// coordinator wraps up: its one turn, the shared files around it, and
// Agentry asking you for what the turn needs to start (a machine online).
var duringWrapUp = []events.Type{events.TaskNeedsYou, events.TaskAnswered, events.TurnStarted, events.PromptAcknowledged, events.SessionStarted,
	events.ActionRequested, events.ActionAsked, events.ActionApproved, events.ActionDenied, events.ActionResult,
	events.PlanUpdated, events.AgentWaiting, events.TurnEnded, events.TurnLimited, events.TurnResumed, events.IssueRaised, events.TurnStopped, events.TaskWrappedUp,
	events.WorktreeMoved, events.FileAdded, events.FileRemoved, events.FilesCollected, events.FilesDelivered}

// ArchivedRefusal is why an archived task takes no event, for the rules
// and for a route that refuses one before proposing anything.
func ArchivedRefusal(stream string) string {
	return stream + " is archived: an archived task takes no events"
}

func refuse(reason string) *events.Rejection {
	return &events.Rejection{Check: "order", Reason: reason}
}
