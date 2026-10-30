package hooks

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

const secret = "sk-live-7f3a90b2"

// startAnotherTurn opens task-2 with its coordinator's turn-1 running.
func startAnotherTurn(t *testing.T, s *store.Store) {
	t.Helper()
	you := events.Actor{Kind: events.Human, ID: "you"}
	for _, p := range []events.Proposal{
		{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"}},
		{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": t.TempDir(), "branch": "b2", "port": 7902}},
		{Stream: "task-2", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
}

// endTurn ends the task's turn as its hook does, with learned in the
// agent's knowledge file.
func endTurn(h *Handler, task, turn, agent, learned string) {
	h.Handle(context.Background(), hookcall.Call{Task: task, Turn: turn, Agent: agent, Engine: "claude", Event: "Stop",
		Payload: []byte(`{"hook_event_name":"Stop","last_assistant_message":"Done."}`), Ending: &hookcall.Ending{Tokens: 1, Knowledge: &learned}})
}

// A turn that starts after the redaction wrote its knowledge file from the
// redacted text, so what it learns is kept as it is, even the secret
// written anew. The secret is held until turn-2, running as it was
// redacted, ends, though its end carries no knowledge.
func TestATurnStartedAfterARedactionKeepsWhatItLearned(t *testing.T) {
	s, state := engineerAtWorkIn(t, t.TempDir())
	h := NewHandler(s, state, judgeByCommand)
	h.Redactions().Hold(secret)(true)
	startAnotherTurn(t, s)

	endTurn(h, "task-2", "turn-1", "tech-lead", "The key is "+secret+".")
	if lead, _ := state.Roster().Get("tech-lead"); lead.Knowledge != "The key is "+secret+"." {
		t.Errorf("the tech lead, whose turn started after the redaction, knows %q", lead.Knowledge)
	}
	if !h.Redactions().Holds(secret) {
		t.Fatal("the secret was let go before turn-2, running as it was redacted, ended")
	}
	stop := hookCall("Stop", `{"hook_event_name":"Stop","last_assistant_message":"Done."}`)
	h.Handle(context.Background(), stop)
	if h.Redactions().Holds(secret) {
		t.Error("the secret is still held after every turn running as it was redacted ended")
	}
}

// Both turns run as the secret is redacted. task-2's ends without its hook
// (as when the session dies): the secret is let go once the state shows
// it ended and the other turn's end comes in, and that turn's text is
// redacted again.
func TestASecretIsLetGoOnceEveryTurnRunningThenHasEnded(t *testing.T) {
	s, state := engineerAtWork(t)
	startAnotherTurn(t, s)
	r := NewHandler(s, state, judgeByCommand).Redactions()
	r.Hold(secret)(true)
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1",
		Data: map[string]any{"agent": "tech-lead", "report": "r"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if !r.Holds(secret) {
		t.Fatal("the secret was let go while turn-2 still runs")
	}
	if learned, clean := r.Ended(Turn{"task-1", "turn-2"}, "a "+secret+" b "+secret); !clean || learned != "a [redacted] b [redacted]" {
		t.Errorf("turn-2's text redacted again: %q, %v", learned, clean)
	}
	if r.Holds(secret) {
		t.Error("the secret is still held after both turns ended")
	}
}

// A turn that starts while the redaction commits wrote its knowledge file
// from the text as it was, so it is held too.
func TestATurnStartedAsTheRedactionCommitsIsHeldToo(t *testing.T) {
	s, state := engineerAtWork(t)
	r := NewHandler(s, state, judgeByCommand).Redactions()
	settle := r.Hold(secret)
	startAnotherTurn(t, s)
	settle(true)
	if learned, _ := r.Ended(Turn{"task-2", "turn-1"}, "The key is "+secret+"."); learned != "The key is [redacted]." {
		t.Errorf("task-2's turn, started as the redaction committed: %q", learned)
	}
}

// A redaction that doesn't commit, as when no row holds the secret since
// an earlier one redacted it, lets go of the turns its own hold added and
// no other: turn-2 is still held for the earlier redaction.
func TestARedactionThatDoesNotCommitLetsGoOnlyOfItsOwnHold(t *testing.T) {
	s, state := engineerAtWork(t)
	r := NewHandler(s, state, judgeByCommand).Redactions()
	r.Hold(secret)(true)
	startAnotherTurn(t, s)
	r.Hold(secret)(false)
	if !r.Holds(secret) {
		t.Fatal("the refused redaction let go of the earlier one's hold")
	}
	if learned, _ := r.Ended(Turn{"task-2", "turn-1"}, "The key is "+secret+"."); learned != "The key is "+secret+"." {
		t.Errorf("task-2's turn, held only by the refused redaction, had its text changed: %q", learned)
	}
	if learned, _ := r.Ended(Turn{"task-1", "turn-2"}, "The key is "+secret+"."); learned != "The key is [redacted]." {
		t.Errorf("turn-2's text: %q", learned)
	}

	r.Hold("never-redacted")(false)
	if r.Holds("never-redacted") {
		t.Error("a redaction that never committed is still held")
	}
}

// A secret that would show again next to the marker, as the store refuses
// to leave it, makes the turn's end keep nothing it learned.
func TestASecretThatWouldStillShowKeepsNothingTheTurnLearned(t *testing.T) {
	s, state := engineerAtWorkIn(t, t.TempDir())
	h := NewHandler(s, state, judgeByCommand)
	h.recordWhatWasLearned("engineer", "Use gofmt.")
	h.Redactions().Hold("d]x")(true)
	endTurn(h, "task-1", "turn-2", "engineer", "Use gofmt.\nd]xx")
	if engineer, _ := state.Roster().Get("engineer"); engineer.Knowledge != "Use gofmt." {
		t.Errorf("the engineer knows %q, want what it knew before the turn", engineer.Knowledge)
	}
}

// Replacing "yy" leaves "d]x" showing in "[redacted]x" when "d]x" was
// replaced first, which the map's order decides: whichever order the two
// are replaced in, the end isn't clean while either still shows.
func TestASecretALaterReplacementLeavesShowingKeepsNothing(t *testing.T) {
	s, state := engineerAtWork(t)
	r := NewHandler(s, state, judgeByCommand).Redactions()
	for range 50 {
		r.Hold("d]x")(true)
		r.Hold("yy")(true)
		learned, clean := r.Ended(Turn{"task-1", "turn-2"}, "yyx")
		if shows := strings.Contains(learned, "d]x") || strings.Contains(learned, "yy"); clean && shows {
			t.Fatalf("%q reported clean", learned)
		}
	}
}

// An older log can hold an archived task whose wrap-up turn never ended,
// archived before the rules made a running turn wait. The state shows no
// turn of it, so a redaction can't hold it, and its end, refused on the
// archived task, keeps nothing it learned.
func TestATurnOnAnArchivedTaskKeepsNothingItLearned(t *testing.T) {
	s, state := olderLog(t)
	you := events.Actor{Kind: events.Human, ID: "you"}
	for _, p := range append(agents.SeedProposals(nil),
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": t.TempDir(), "branch": "b", "port": 7901}},
		events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "engineer", "hash": "h"}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Moved.", "moved": true}}, // stops turn-1
		events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-2", Data: map[string]any{"agent": "engineer", "hash": "h"}},
	) {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	storetest.Archive(t, s, state, "task-1")
	h := NewHandler(s, state, judgeByCommand)
	h.recordWhatWasLearned("engineer", "The key is "+secret+".")
	settle := h.Redactions().Hold(secret)
	redaction, rejection, err := s.Redact(events.AgentsStream, secret, you, "Pasted.")
	settle(redaction.Position != 0)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	endTurn(h, "task-1", "turn-2", "engineer", "The key is "+secret+".\n\nDeploy on Fridays.")
	if engineer, _ := state.Roster().Get("engineer"); engineer.Knowledge != "The key is [redacted]." {
		t.Errorf("the engineer knows %q after a turn on an archived task ended, want the redacted knowledge alone", engineer.Knowledge)
	}
}

// olderLog is a log as the rules were before a task with a turn running
// waited for it to end to be archived: they let its marker through.
func olderLog(t *testing.T) (*store.Store, *current.State) {
	t.Helper()
	state := storetest.NewState(events.Core())
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), events.Core(), state, func(p events.Proposal) *events.Rejection {
		rejection := rules.Check(p, state, events.Core(), storetest.Installed()...)
		if p.Type == events.TaskArchived && rejection != nil && strings.Contains(rejection.Reason, "turn running") {
			return nil
		}
		return rejection
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		storetest.CheckEqual(t, s, state)
		s.Close()
	})
	return s, state
}

// Without a hooks handler, as in a test of the routes, nothing is held.
func TestNoRedactionsHoldNothing(t *testing.T) {
	var r *Redactions
	r.Hold(secret)(true)
	if r.Holds(secret) {
		t.Error("nil Redactions holds a secret")
	}
}
