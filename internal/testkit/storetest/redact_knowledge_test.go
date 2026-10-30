package storetest

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
)

// Redacting a secret in an agent's knowledge, as the state, the rules and
// the files on disk see it (docs/design/log-growth.md section 5,
// "Removing a secret", item 4). Which rows are written again, and how, is
// in store's redact_knowledge_test.go.

// learnt is one version of an agent's knowledge as the log rebuilds it.
type learnt struct {
	position int64
	name     string
	text     string
}

// learnIn proposes text as an agent's next version: a patch on the
// knowledge the state holds, unless full. It returns its position.
func learnIn(t testing.TB, s *store.Store, state *current.State, name, text string, full bool) int64 {
	t.Helper()
	agent, ok := state.Roster().Get(name)
	if !ok {
		t.Fatalf("no agent called %s", name)
	}
	p := agent.Learned(you, text)
	if !full {
		p = agent.Patching(you, agents.Diff(agent.Knowledge, text))
	}
	e, rejection, err := s.Propose(p)
	if rejection != nil || err != nil {
		t.Fatalf("%s learning %q: %v, %v", name, text, rejection, err)
	}
	return e.Position
}

// versionsIn is every knowledge version the agents stream rebuilds, in
// order, with the agent's name then.
func versionsIn(t testing.TB, s *store.Store) []learnt {
	t.Helper()
	stream, err := s.Stream(events.AgentsStream)
	if err != nil {
		t.Fatal(err)
	}
	var roster agents.Roster
	var versions []learnt
	for _, e := range stream {
		roster.Apply(e)
		if e.Type == events.AgentLearned {
			agent, _ := roster.Get(e.Text("name"))
			versions = append(versions, learnt{e.Position, agent.Name, agent.Knowledge})
		}
	}
	return versions
}

// withKnowledge seeds the roster and gives the engineer six versions,
// full copies and patches, with secret written into the third, a patch,
// and kept by every one after; the reviewer learns two versions among
// them, without it. It returns the engineer's positions.
func withKnowledge(t testing.TB, s *store.Store, state *current.State, secret string) []int64 {
	t.Helper()
	for _, p := range agents.SeedProposals(nil) {
		mustPropose(t, s, p)
	}
	held := "The token is " + secret + ".\n"
	engineer := []int64{
		learnIn(t, s, state, "engineer", "Notes.\n", true),
		learnIn(t, s, state, "engineer", "Notes.\nRun go vet.\n", false),
		learnIn(t, s, state, "engineer", "Notes.\nRun go vet.\n"+held, false),
	}
	learnIn(t, s, state, "reviewer", "Review notes.\n", true)
	learnIn(t, s, state, "reviewer", "Review notes.\nCheck the tests.\n", false)
	return append(engineer,
		learnIn(t, s, state, "engineer", "Notes.\nRun go vet.\n"+held+"Keep commits small.\n", false),
		learnIn(t, s, state, "engineer", "Notes.\nRun go vet.\n"+held+"Keep commits small.\nAsk first.\n", true),
		learnIn(t, s, state, "engineer", "Engineer notes.\nRun go vet.\n"+held+"Keep commits small.\nAsk first.\n", false),
	)
}

// checkRedactedFrom fails the test unless after is before with each
// version from the one at position first on showing [redacted] where the
// secret was, and holding it no more, and the versions before as they were.
func checkRedactedFrom(t *testing.T, before, after []learnt, first int64, secret string) {
	t.Helper()
	if len(after) != len(before) {
		t.Fatalf("%d versions after the redaction, want the %d before", len(after), len(before))
	}
	for i, was := range before {
		want := was
		if was.position >= first {
			want.text = strings.ReplaceAll(was.text, secret, store.Redacted)
		}
		if after[i] != want {
			t.Errorf("the version at %d:\n got %+v\nwant %+v", was.position, after[i], want)
		}
	}
}

// A secret with what JSON and HTML escape in it, so the search of the
// files finds it however it is written.
const learntSecret = `tok<"en>&9f3a`

// Test 1: once a secret in an agent's knowledge is redacted, it is in no
// byte of the log, its WAL or a snapshot written after, raw or escaped,
// though only one row held it.
func TestARedactedKnowledgeSecretIsInNoByteOnDisk(t *testing.T) {
	dir := t.TempDir()
	s, state := Open(t, filepath.Join(dir, "agentry.db"))
	withKnowledge(t, s, state, learntSecret)
	if err := s.GiveBackSpace(); err != nil {
		t.Fatal(err)
	}
	if holding := HoldingAny(t, dir, learntSecret); !slices.Contains(holding, "agentry.db") {
		t.Fatalf("before redacting, the secret is in %v: want the log", holding)
	}

	redact(t, s, events.AgentsStream, learntSecret)
	if err := current.WriteSnapshot(filepath.Join(dir, "state.snapshot"), state, s, "build-1"); err != nil {
		t.Fatal(err)
	}
	if holding := HoldingAny(t, dir, learntSecret); len(holding) != 0 {
		t.Errorf("after redacting, the secret is still in %v", holding)
	}
}

// Test 2: every version from the one the secret was written in rebuilds
// with [redacted] in its place, the versions before as they were, and the
// state is a replay's, live and after a restart. The newest version is a
// full copy now, so the agent has patched nothing since, and the next
// patch, made against the redacted text as the hook makes it, lands on it.
func TestAKnowledgeRedactionReplaysAsItWasLive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, live := Open(t, path)
	engineer := withKnowledge(t, s, live, pasted)
	before, newest := versionsIn(t, s), knowledgeOf(t, live, "engineer").Knowledge

	redact(t, s, events.AgentsStream, pasted)
	CheckEqual(t, s, live)
	checkRedactedFrom(t, before, versionsIn(t, s), engineer[2], pasted)
	agent := knowledgeOf(t, live, "engineer")
	if want := strings.ReplaceAll(newest, pasted, store.Redacted); agent.Knowledge != want || agent.Patched() != 0 {
		t.Errorf("the engineer knows %q, patched %d since a full copy; want %q and none", agent.Knowledge, agent.Patched(), want)
	}

	next := agent.Knowledge + "Run the tests.\n"
	learnIn(t, s, live, "engineer", next, false)
	CheckEqual(t, s, live)
	if agent := knowledgeOf(t, live, "engineer"); agent.Knowledge != next || agent.Patched() != len("Run the tests.\n") {
		t.Errorf("after a patch, the engineer knows %q, patched %d; want %q", agent.Knowledge, agent.Patched(), next)
	}

	_, restarted := Open(t, path)
	if again, want := knowledgeOf(t, restarted, "engineer"), knowledgeOf(t, live, "engineer"); !reflect.DeepEqual(again, want) {
		t.Errorf("the engineer after a restart: %+v, want %+v", again, want)
	}
}

// knowledgeOf is the agent called name in the state.
func knowledgeOf(t *testing.T, state *current.State, name string) agents.Agent {
	t.Helper()
	agent, ok := state.Roster().Get(name)
	if !ok {
		t.Fatalf("no agent called %s", name)
	}
	return agent
}

// Test 3: an agent is followed across a rename: its versions before and
// after it are redacted, from the first that holds the secret.
func TestKnowledgeIsRedactedAcrossARename(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		mustPropose(t, s, p)
	}
	learnIn(t, s, state, "engineer", "Notes.\n", true)
	first := learnIn(t, s, state, "engineer", "Notes.\nUse "+pasted+".\n", false)
	mustPropose(t, s, agents.Agent{Name: "engineer"}.Renaming(you, "builder"))
	learnIn(t, s, state, "builder", "Notes.\nUse "+pasted+".\nAsk first.\n", false)
	learnIn(t, s, state, "builder", "Builder notes.\nUse "+pasted+".\nAsk first.\n", false)
	before := versionsIn(t, s)

	redaction := redact(t, s, events.AgentsStream, pasted)
	if positions := redaction.Numbers("positions"); len(positions) != 3 || positions[0] != first {
		t.Errorf("the redaction names %v, want the three versions from %d", positions, first)
	}
	checkRedactedFrom(t, before, versionsIn(t, s), first, pasted)
	if knowledge := knowledgeOf(t, state, "builder").Knowledge; knowledge != "Builder notes.\nUse [redacted].\nAsk first.\n" {
		t.Errorf("the builder knows %q", knowledge)
	}
	CheckEqual(t, s, state)
}

// Test 4: another agent's knowledge, which never held the secret, keeps
// its rows and its state, its patches too.
func TestAnotherAgentsKnowledgeIsUntouched(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	withKnowledge(t, s, state, pasted)
	reviewersRows := func() []events.Event {
		stream, err := s.Stream(events.AgentsStream)
		if err != nil {
			t.Fatal(err)
		}
		return slices.DeleteFunc(stream, func(e events.Event) bool { return e.Type != events.AgentLearned || e.Text("name") != "reviewer" })
	}
	rows, reviewer := reviewersRows(), knowledgeOf(t, state, "reviewer")

	redact(t, s, events.AgentsStream, pasted)
	if after := reviewersRows(); !reflect.DeepEqual(after, rows) {
		t.Errorf("the reviewer's versions changed:\n%v\n%v", rows, after)
	}
	if after := knowledgeOf(t, state, "reviewer"); !reflect.DeepEqual(after, reviewer) || after.Patched() == 0 {
		t.Errorf("the reviewer is %+v after, want %+v, with its patch", after, reviewer)
	}
}

// Test 5: with the rules on, a refused redaction of the agents stream
// changes nothing: no row holds the secret, the reason holds it, or a
// version holds it in a key.
func TestARefusedKnowledgeRedactionChangesNothing(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	withKnowledge(t, s, state, pasted)
	keyed := agents.Agent{Name: "tester"}.Learned(you, "Use "+short+".")
	keyed.Data["extra"] = map[string]any{short: "x"}
	mustPropose(t, s, keyed)
	for _, c := range []struct {
		name, secret, why, want string
	}{
		{"no row holds it", "not-there", "", "no row of agents holds the secret"},
		{"the reason holds it", pasted, "Remove " + pasted + ".", "the reason holds the secret, which the log would then keep"},
		{"a key", short, "", "holds the secret in a key, which redacting doesn't rewrite"},
	} {
		before, err := s.Stream(events.AgentsStream)
		if err != nil {
			t.Fatal(err)
		}
		last := s.LastPosition()
		if _, rejection, err := s.Redact(events.AgentsStream, c.secret, you, c.why); err != nil || rejection == nil || !strings.HasSuffix(rejection.Reason, c.want) {
			t.Errorf("%s: %v, %v, want %q", c.name, rejection, err, c.want)
		}
		after, err := s.Stream(events.AgentsStream)
		if err != nil {
			t.Fatal(err)
		}
		if s.LastPosition() != last || !reflect.DeepEqual(after, before) {
			t.Errorf("%s: a refused redaction changed the log", c.name)
		}
	}
	CheckEqual(t, s, state)
}

// Test 6: a patch whose text no longer holds the secret but whose base did
// is written again as a full copy, though it keeps part of the secret in
// its head and tail: "use sek-12 now" on "use sek-1234 now" keeps "use
// sek-12" and " now", which would rebuild as "use [redac now" on the
// redacted base, and the state would fold the same wrong text.
func TestAPatchKeepingPartOfTheSecretIsWrittenAsAFullCopy(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		mustPropose(t, s, p)
	}
	const secret = "sek-1234"
	learnIn(t, s, state, "engineer", "use sek-", true)
	learnIn(t, s, state, "engineer", "use sek-12", false)
	held := learnIn(t, s, state, "engineer", "use sek-1234 now", false)
	kept := learnIn(t, s, state, "engineer", "use sek-12 now", false)
	if patch := patchAt(t, s, kept); patch.Head != len("use sek-12") || patch.Tail != len(" now") {
		t.Fatalf("the patch at %d keeps head %d and tail %d, want 10 and 4", kept, patch.Head, patch.Tail)
	}
	before := versionsIn(t, s)

	redaction := redact(t, s, events.AgentsStream, secret)
	if positions := redaction.Numbers("positions"); !slices.Equal(positions, []int64{held, kept}) {
		t.Errorf("the redaction names %v, want %d and %d", positions, held, kept)
	}
	knowledge := []any{"$.knowledge"}
	if fields := redaction.Data["fields"]; !reflect.DeepEqual(fields, []any{knowledge, knowledge}) {
		t.Errorf("the redaction names the fields %v, want $.knowledge twice", fields)
	}
	checkRedactedFrom(t, before, versionsIn(t, s), held, secret)
	if newest := eventAt(t, s, kept); agents.IsPatch(newest.Proposal) || newest.Text("knowledge") != "use sek-12 now" {
		t.Errorf("the newest version is %+v, want a full copy of its text", newest)
	}
	CheckEqual(t, s, state)
	if count, _, err := s.Preview(events.AgentsStream, secret); count != 0 || err != nil {
		t.Errorf("%d rows still hold the secret, %v", count, err)
	}
}

// patchAt is the patch the agent.learned at position carries.
func patchAt(t *testing.T, s *store.Store, position int64) agents.Patch {
	t.Helper()
	patch, err := agents.PatchOf(eventAt(t, s, position).Proposal)
	if err != nil {
		t.Fatal(err)
	}
	return patch
}

// eventAt is the agents stream's event at position.
func eventAt(t *testing.T, s *store.Store, position int64) events.Event {
	t.Helper()
	stream, err := s.Stream(events.AgentsStream)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range stream {
		if e.Position == position {
			return e
		}
	}
	t.Fatalf("no event at %d", position)
	return events.Event{}
}
