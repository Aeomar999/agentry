package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
)

// Target 9 of docs/design/log-growth.md, on a copy of a real agentry.db
// named by AGENTRY_BENCH_DB, which it copies again so the run never writes
// to it. Every agent.learned of the copy, all full copies, is written again
// as the hook would write it today: a patch, or a full copy by learning's
// rule. Yours stay full copies, as the page sends them. Each version must
// rebuild byte for byte; the test prints the bytes stored, as a share of the
// full copies' (target <= 8%), and the most text any version's rebuild reads,
// as a multiple of its own length (target <= 2.5).
//
//	D=$(mktemp -d); sqlite3 "file:$HOME/.agentry/agentry.db?mode=ro" "VACUUM INTO '$D/agentry.db'"
//	AGENTRY_BENCH_DB=$D/agentry.db go test ./internal/hooks -run TestKnowledgeAsPatchesOnARealLog -v -count=1
func TestKnowledgeAsPatchesOnARealLog(t *testing.T) {
	source := os.Getenv("AGENTRY_BENCH_DB")
	if source == "" {
		t.Skip("AGENTRY_BENCH_DB is not set")
	}
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "agentry.db")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	catalogue := storetest.WithFeatures()
	s, err := store.Open(path, catalogue, current.New(catalogue), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	stream, err := s.Stream(events.AgentsStream)
	if err != nil {
		t.Fatal(err)
	}

	var before, after agents.Roster // the log as it is, and as patches
	lastCopy := map[string]int{}    // each agent's last full copy's length
	var copiesBytes, storedBytes, versions, patches, mostRead int
	mostRatio, mostRatioOf := 0.0, ""
	for _, e := range stream {
		rewritten := e
		if e.Type == events.AgentLearned {
			name := e.Text("name")
			had, _ := after.Get(name)
			before.Apply(e)
			version, _ := before.Get(name)
			if e.Actor.Kind == events.Agent {
				rewritten = asLogged(t, learning(had, version.Knowledge), e)
			}
			copiesBytes += size(t, e.Data)
			storedBytes += size(t, rewritten.Data)
			versions++
			if agents.IsPatch(rewritten.Proposal) {
				patches++
			} else {
				lastCopy[name] = len(version.Knowledge)
			}
		} else {
			before.Apply(e)
		}
		if e.Type == events.AgentRenamed {
			lastCopy[e.Text("to")] = lastCopy[e.Text("name")]
		}
		after.Apply(rewritten)

		if e.Type != events.AgentLearned {
			continue
		}
		name := e.Text("name")
		want, _ := before.Get(name)
		got, _ := after.Get(name)
		if got.Knowledge != want.Knowledge {
			t.Fatalf("position %d: %s's knowledge rebuilds as %d bytes, want %d", e.Position, name, len(got.Knowledge), len(want.Knowledge))
		}
		read := lastCopy[name] + got.Patched()
		mostRead = max(mostRead, read)
		if ratio := float64(read) / float64(max(len(got.Knowledge), 1)); ratio > mostRatio {
			mostRatio, mostRatioOf = ratio, name
		}
	}
	if versions == 0 {
		t.Fatal("the copy has no agent.learned")
	}
	share := 100 * float64(storedBytes) / float64(copiesBytes)
	t.Logf("%d versions: %d patches, %d full copies", versions, patches, versions-patches)
	t.Logf("stored: %d bytes of %d as full copies, %.2f%% (target <= 8%%)", storedBytes, copiesBytes, share)
	t.Logf("most read to rebuild a version: %.2f times its length (%s; target <= 2.5); most bytes read: %d", mostRatio, mostRatioOf, mostRead)
	if share > 8 || mostRatio > 2.5 {
		t.Errorf("Target 9 missed: %.2f%% stored, %.2f times read", share, mostRatio)
	}
}

// asLogged is p written in place of e, as the log would hand it back.
func asLogged(t *testing.T, p events.Proposal, e events.Event) events.Event {
	written, err := json.Marshal(p.Data)
	if err != nil {
		t.Fatal(err)
	}
	e.Proposal, e.Data = p, nil
	if err := json.Unmarshal(written, &e.Data); err != nil {
		t.Fatal(err)
	}
	return e
}

func size(t *testing.T, data map[string]any) int {
	written, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return len(written)
}
