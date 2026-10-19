// Package ideas computes an idea's state from its events, as package tasks
// does for a task. An idea is a feature a Visionaries session suggested for
// a repo; you accept it, and it becomes a task in your Backlog, or dismiss it.
package ideas

import (
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// State is where an idea stands with you.
type State string

const (
	Open      State = "open"
	Accepted  State = "accepted"
	Dismissed State = "dismissed"
)

// Sizes an idea can be: small, medium or large.
var Sizes = []string{"S", "M", "L"}

type Idea struct {
	ID        string
	Title     string
	Problem   string
	Proposal  string // the feature
	WhyNow    string
	Size      string // S, M or L
	Risks     string
	BackedBy  []string   // the agents for it
	DoubtedBy []string   // the agents against it
	Task      string     // the Visionaries task it came from
	Repo      string     // the repo it is about
	Epic      string     // an epic it follows up, "" for none
	Kind      tasks.Kind // what it suggests making of it: a story or an epic
	Repeats   string     // an idea the Visionaries said it raises again, "" for none
	CreatedAt time.Time

	State        State
	Note         string // what you said when you accepted or dismissed it
	Became       string // for an accepted idea: the task made from it
	AutoAccepted bool   // Agentry accepted it, as the Visionaries agreed, not you
}

// FromEvents builds an idea by applying its accepted events in order.
// It returns nil if the events don't include idea.suggested.
func FromEvents(stream []events.Event) *Idea {
	var idea *Idea
	for _, e := range stream {
		if e.Type == IdeaSuggested {
			idea = &Idea{ID: e.Stream, CreatedAt: e.At}
		}
		if idea != nil {
			idea.apply(e)
		}
	}
	return idea
}

// Next is the idea after e, built on a copy: idea itself is left as it was,
// so whoever still holds it keeps reading what it said. FromEvents applies in
// place instead, and stays the reference Next must agree with. apply only
// replaces fields, never changes a slice, so a shallow copy is enough.
func Next(idea *Idea, e events.Event) *Idea {
	switch {
	case e.Type == IdeaSuggested:
		idea = &Idea{ID: e.Stream, CreatedAt: e.At}
	case idea == nil:
		return nil
	default:
		copied := *idea
		idea = &copied
	}
	idea.apply(e)
	return idea
}

func (idea *Idea) apply(e events.Event) {
	switch e.Type {
	case IdeaSuggested:
		idea.Title, idea.Problem, idea.Proposal = e.Text("title"), e.Text("problem"), e.Text("proposal")
		idea.WhyNow, idea.Size, idea.Risks = e.Text("whyNow"), e.Text("size"), e.Text("risks")
		idea.BackedBy, idea.DoubtedBy = e.List("backedBy"), e.List("doubtedBy")
		idea.Task, idea.Repo, idea.Epic = e.Text("task"), e.Text("repo"), e.Text("epic")
		idea.Kind, idea.Repeats = tasks.Kind(e.Text("kind")), e.Text("repeats")
		idea.State = Open
	case IdeaAccepted:
		idea.State, idea.Note, idea.Became = Accepted, e.Text("note"), e.Text("task")
		idea.AutoAccepted = e.Actor.Kind == events.System
	case IdeaDismissed:
		idea.State, idea.Note = Dismissed, e.Text("note")
	}
}
