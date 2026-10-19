package store

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

var you = events.Actor{Kind: events.Human, ID: "you"}

func newTask(id string) events.Proposal {
	return events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Remove the TODOs", "repo": "/src/app", "coordinator": "tech-lead"}}
}

// onlyOnce is a stand-in order check: a task can be created once.
func onlyOnce(p events.Proposal, history History) *events.Rejection {
	existing, _ := history.Stream(p.Stream)
	if p.Type == events.TaskCreated && len(existing) > 0 {
		return &events.Rejection{Check: "order", Reason: "task already exists"}
	}
	return nil
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "agentry.db"), onlyOnce)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProposeAppendsOrRecordsTheRejection(t *testing.T) {
	s := openTestStore(t)

	accepted, rejection, err := s.Propose(newTask("task-1"))
	if err != nil || rejection != nil || accepted.Position != 1 {
		t.Fatalf("first: %+v %v %v", accepted, rejection, err)
	}
	refused, rejection, err := s.Propose(newTask("task-1"))
	if err != nil || rejection == nil || rejection.Check != "order" || refused.Type != events.ProposalRejected {
		t.Fatalf("second: %+v %v %v", refused, rejection, err)
	}
	if _, rejection, _ := s.Propose(events.Proposal{Stream: "task-1", Actor: you, Type: "task.exploded"}); rejection == nil || rejection.Check != "shape" {
		t.Errorf("unknown type: %v", rejection)
	}

	stream, _ := s.Stream("task-1")
	if len(stream) != 1 || stream[0].Text("title") != "Remove the TODOs" {
		t.Errorf("the stream should hold only the accepted event, got %+v", stream)
	}
	all, _ := s.After(0, 100)
	if len(all) != 3 || all[1].Text("reason") != "task already exists" || all[1].Text("by") != "human:you" {
		t.Errorf("the log should hold both rejections, got %+v", all)
	}
}

func TestEventsCannotBeChangedOrDeleted(t *testing.T) {
	s := openTestStore(t)
	s.Propose(newTask("task-1"))
	for _, statement := range []string{`UPDATE events SET type = 'x'`, `DELETE FROM events`} {
		if _, err := s.db.Exec(statement); err == nil {
			t.Errorf("%s succeeded; events must be append-only", statement)
		}
	}
}

// Proof 05: twenty proposals racing for the same task, exactly one accepted.
func TestRacingProposalsAcceptExactlyOne(t *testing.T) {
	s := openTestStore(t)
	var wait sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, rejection, err := s.Propose(newTask("task-1"))
			if err != nil {
				t.Error(err)
			}
			if rejection == nil {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}
	wait.Wait()
	if accepted != 1 {
		t.Errorf("%d of 20 accepted, want exactly 1", accepted)
	}
}

func TestReopenKeepsTheLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	first, _ := Open(path, onlyOnce)
	first.Propose(newTask("task-1"))
	first.Propose(newTask("task-2"))
	first.Close()

	second, err := Open(path, onlyOnce)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	ids, _ := second.TaskIDs()
	if len(ids) != 2 || ids[0] != "task-1" || ids[1] != "task-2" {
		t.Errorf("task ids after reopening: %v", ids)
	}
}

func TestCreateNamesTheNextStreamOfItsKind(t *testing.T) {
	s := openTestStore(t)
	idea := events.Proposal{Actor: events.Actor{Kind: events.System, ID: "agentry"}, Type: events.IdeaSuggested,
		Data: map[string]any{"title": "x", "problem": "p", "proposal": "f", "size": "S", "repo": "/src/app", "task": "task-1"}}
	var names []string
	for _, p := range []events.Proposal{newTask(""), idea, newTask(""), idea} {
		prefix := "task-"
		if p.Type == events.IdeaSuggested {
			prefix = "idea-"
		}
		created, rejection, err := s.Create(prefix, p)
		if rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
		names = append(names, created.Stream)
	}
	if want := []string{"task-1", "idea-1", "task-2", "idea-2"}; !slices.Equal(names, want) {
		t.Errorf("made %v, want %v", names, want)
	}
}
