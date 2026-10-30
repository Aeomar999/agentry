package store

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// Redacting a secret in an agent's knowledge, as the rows see it. The
// state, the rules and the files on disk are in storetest's
// redact_knowledge_test.go.

// A learning is one version of an agent's knowledge: a full copy of text,
// or a patch on the version before.
type learning struct {
	text  string
	patch bool
}

// learn proposes an agent's versions, each a full copy or a patch on the
// one before, and returns their positions.
func learn(t *testing.T, s *Store, name string, versions ...learning) []int64 {
	t.Helper()
	agent, before := agents.Agent{Name: name}, ""
	var positions []int64
	for _, v := range versions {
		p := agent.Learned(you, v.text)
		if v.patch {
			p = agent.Patching(you, agents.Diff(before, v.text))
		}
		e, rejection, err := s.Propose(p)
		if rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
		positions, before = append(positions, e.Position), v.text
	}
	return positions
}

// agentCreated is the agent.created of an agent called name.
func agentCreated(name string) events.Proposal {
	return events.Proposal{Stream: events.AgentsStream, Actor: you, Type: events.AgentCreated, Data: map[string]any{"name": name, "prompt": "Work.", "engine": "claude"}}
}

// engineersVersions are six versions, full copies and patches, with the
// secret written into the third, a patch, and kept by every one after: a
// patch whose own middle doesn't hold it, a full copy, and a patch at the
// start of the text.
var engineersVersions = []learning{
	{"Notes.\n", false},
	{"Notes.\nRun go vet.\n", true},
	{"Notes.\nRun go vet.\nThe token is " + secret + ".\n", true},
	{"Notes.\nRun go vet.\nThe token is " + secret + ".\nKeep commits small.\n", true},
	{"Notes.\nRun go vet.\nThe token is " + secret + ".\nKeep commits small.\nAsk first.\n", false},
	{"Engineer notes.\nRun go vet.\nThe token is " + secret + ".\nKeep commits small.\nAsk first.\n", true},
}

// From the version the secret is written in, every version of the agent
// is written again as a full copy of its text with the secret redacted,
// the full copy in place; the versions before it, and another agent's,
// are the rows they were, byte for byte. The redaction names each version
// rewritten, by its knowledge field.
func TestKnowledgeFromTheSecretOnIsWrittenAgainInFull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	acceptAll(t, s, agentCreated("engineer"), agentCreated("reviewer"))
	engineer := learn(t, s, "engineer", engineersVersions[:3]...)
	reviewer := learn(t, s, "reviewer", learning{"Review notes.\n", false}, learning{"Review notes.\nCheck tests.\n", true})
	engineer = append(engineer, learn(t, s, "engineer", engineersVersions[3:]...)...)
	before := rowsAt(t, path)

	redaction, rejection, err := s.Redact(events.AgentsStream, secret, you, "")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	rewritten := map[int64]string{}
	for i, position := range engineer[2:] {
		text := strings.ReplaceAll(engineersVersions[2+i].text, secret, Redacted)
		data, _ := json.Marshal(map[string]any{"knowledge": text, "name": "engineer"})
		rewritten[position] = string(data)
	}
	after := rowsAt(t, path)
	for i, was := range before {
		want := was
		if data, ok := rewritten[was.Position]; ok {
			want.Data = data
		}
		if after[i] != want {
			t.Errorf("the row at %d:\n got %+v\nwant %+v", was.Position, after[i], want)
		}
	}
	if positions := redaction.Numbers("positions"); !reflect.DeepEqual(positions, engineer[2:]) {
		t.Errorf("the redaction names %v, want the engineer's versions from the third %v (the reviewer's are %v)", positions, engineer[2:], reviewer)
	}
	knowledge := []any{"$.knowledge"}
	if fields := redaction.Data["fields"]; !reflect.DeepEqual(fields, []any{knowledge, knowledge, knowledge, knowledge}) {
		t.Errorf("the redaction's fields %v, want the knowledge of each", fields)
	}
	if removed := redaction.Number("removed"); removed != 4*len(secret) {
		t.Errorf("the redaction removed %d bytes, want %d", removed, 4*len(secret))
	}
	if count, _, err := s.Preview(events.AgentsStream, secret); count != 0 || err != nil {
		t.Errorf("the rows holding the secret after: %d, %v, want none", count, err)
	}
}

// A secret no row holds, which a patch's middle makes with the end of its
// base, is found in the text the patch makes, and that patch is written
// again in full.
func TestASecretAcrossAPatchAndItsBaseIsFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	acceptAll(t, s, agentCreated("engineer"))
	half := len(secret) / 2
	positions := learn(t, s, "engineer", learning{"Use " + secret[:half], false}, learning{"Use " + secret + ".", true})
	for _, r := range rowsAt(t, path) {
		if strings.Contains(r.Data, secret) {
			t.Fatalf("the row at %d holds the secret whole: %s", r.Position, r.Data)
		}
	}
	if count, rejection, err := s.Preview(events.AgentsStream, secret); count != 1 || rejection != nil || err != nil {
		t.Fatalf("the rows holding the secret: %d, %v, want the patch", count, err)
	}
	if _, rejection, err := s.Redact(events.AgentsStream, secret, you, ""); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if got, want := rowAt(t, path, positions[1]).Data, `{"knowledge":"Use [redacted].","name":"engineer"}`; got != want {
		t.Errorf("the patch is now %s, want %s", got, want)
	}
	if got := rowAt(t, path, positions[0]).Data; !strings.Contains(got, secret[:half]) {
		t.Errorf("the version before the secret is now %s: it holds only part of it, and stays", got)
	}
}

// A knowledge version with the secret in a key is refused for that, as
// any row is, though writing it in full would drop the key; and nothing
// is written.
func TestAKnowledgeVersionWithTheSecretInAKeyIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	acceptAll(t, s, agentCreated("engineer"))
	learn(t, s, "engineer", learning{"Notes.", false})
	patch := agents.Agent{Name: "engineer"}.Patching(you, agents.Diff("Notes.", "Notes. Use "+secret+"."))
	patch.Data["extra"] = map[string]any{secret: "x"}
	acceptAll(t, s, patch)
	before := rowsAt(t, path)
	_, rejection, err := s.Redact(events.AgentsStream, secret, you, "")
	if want := "holds the secret in a key, which redacting doesn't rewrite"; err != nil || rejection == nil || !strings.HasSuffix(rejection.Reason, want) {
		t.Errorf("redacting: %v, %v, want %q", rejection, err, want)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
		t.Errorf("a refused redaction changed the log:\n%v\n%v", before, after)
	}
}

// A knowledge version that would still hold the secret once its text is
// redacted, in a number beside it, is refused like any other row, and
// nothing is written.
func TestAKnowledgeVersionLeavingTheSecretInANumberIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	acceptAll(t, s, agentCreated("engineer"))
	learned := agents.Agent{Name: "engineer"}.Learned(you, "The PIN is "+pin+".")
	learned.Data["pin"] = 90210417
	acceptAll(t, s, learned)
	before := rowsAt(t, path)
	_, rejection, err := s.Redact(events.AgentsStream, pin, you, "")
	if want := "the row at 2 holds the secret outside its text, as in a number, which redacting can't rewrite: redact a longer piece of the text"; err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("%v, %v, want %q", rejection, err, want)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
		t.Errorf("a refused redaction changed the log:\n%v\n%v", before, after)
	}
}
