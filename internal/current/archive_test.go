package current

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// archiveTask archives a task through the store, with the summary the list
// shows of it, and returns its marker.
func archiveTask(t *testing.T, s *store.Store, state *State, id string, blobs store.Blobs) events.Event {
	t.Helper()
	marker, rejection, err := s.Archive(id, summarise(state, id), blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return marker
}

// summarise is the summariser Archive calls for a task: its summary as the
// state holds it then. (storetest.Summarise, which imports this package.)
func summarise(state *State, id string) func() any {
	return func() any {
		task, err := state.Task(id)
		if task == nil || err != nil {
			return nil // archived, broken or missing: the rules refuse the marker
		}
		return tasks.SummaryOf(task, state.Roster())
	}
}

// blobAndLive is the log with an archived stream's blob put back: every
// event, in order, as before the archive, and the marker.
func blobAndLive(t *testing.T, live []events.Event, blobs sharedfiles.Disk, marker events.Event) []events.Event {
	t.Helper()
	blob, err := blobs.Open(marker.Text("archive"))
	if err != nil {
		t.Fatal(err)
	}
	defer blob.Close()
	archived, err := store.ReadArchive(blob)
	if err != nil {
		t.Fatal(err)
	}
	whole := append(archived, live...)
	slices.SortFunc(whole, func(a, b events.Event) int { return int(a.Position - b.Position) })
	return whole
}

// Equivalence after archiving (log-growth.md section 2): the state the
// store keeps, the state from before with the marker applied, and a replay
// of the blob's rows with the live ones all equal a replay of the live log,
// in which the task is its summary.
func TestAnArchivedTaskReplaysFromItsBlobAndTheLiveRows(t *testing.T) {
	path, _ := seededLog(t, someProposals()...)
	state := New(withIdeas, ideas.Streams())
	s, err := store.Open(path, withIdeas, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	all, err := everyEvent(s)
	if err != nil {
		t.Fatal(err)
	}
	before := New(withIdeas, ideas.Streams())
	before.loadAll(all)

	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	marker := archiveTask(t, s, state, "task-1", blobs)
	before.Apply(marker)
	live, err := everyEvent(s)
	if err != nil {
		t.Fatal(err)
	}
	whole := blobAndLive(t, live, blobs, marker)
	if !reflect.DeepEqual(whole[:len(whole)-1], all) {
		t.Fatal("the blob's rows and the live ones aren't the log from before the marker")
	}
	replayed := New(withIdeas, ideas.Streams())
	replayed.loadAll(whole)

	for name, held := range map[string]*State{"the store's": state, "the one from before": before, "a replay with the blob": replayed} {
		if err := Equal(s, held); err != nil {
			t.Errorf("%s state: %v", name, err)
		}
	}
	if task, err := state.Task("task-1"); task != nil || !errors.Is(err, ErrArchived) || len(live) >= len(all) {
		t.Errorf("task-1 should be archived and its rows gone: %v, %v, %d events live of %d", task, err, len(live), len(all))
	}
}

// A snapshot taken at one of the rows archived since names a row the log
// no longer has: Open replays the whole log, which equals the state.
func TestASnapshotAtAnArchivedRowFallsBack(t *testing.T) {
	logPath, log := seededLog(t, append(someProposals(), proposal("task-1", you, events.TaskRenamed, map[string]any{"title": "Last"}))...)
	anchor := log[len(log)-1]
	snapshot := snapshotted(t, logPath, anchor.Position, nil)

	state := New(withIdeas, ideas.Streams())
	s, err := store.Open(logPath, withIdeas, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	archiveTask(t, s, state, anchor.Stream, sharedfiles.Disk{Dir: t.TempDir()})
	s.Close()

	restored, logged := opened(t, logPath, snapshot, "build-1")
	if restored.Resumed() != 0 || !strings.Contains(logged, "the log has no event at its position") {
		t.Errorf("Open went on from %d and logged %q; want a whole replay, as the anchor row is archived", restored.Resumed(), logged)
	}
}

// The marker keeps the summary of every row it covers: a rename between
// taking the summariser and archiving is in the marker's summary and the
// list's, as Archive summarises under the store's lock, from the rows it
// read.
func TestTheMarkerSummarisesTheRowsItCovers(t *testing.T) {
	path, _ := seededLog(t, someProposals()...)
	state := New(withIdeas, ideas.Streams())
	s, err := store.Open(path, withIdeas, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	summariser := summarise(state, "task-1")
	if _, rejection, err := s.Propose(proposal("task-1", you, events.TaskRenamed, map[string]any{"title": "Renamed in the gap"})); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	marker, rejection, err := s.Archive("task-1", summariser, sharedfiles.Disk{Dir: t.TempDir()})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if title := marker.Data["summary"].(map[string]any)["title"]; title != "Renamed in the gap" {
		t.Errorf("the marker's summary has the title %q, want the rename's", title)
	}
	listed := slices.IndexFunc(state.Summaries(), func(s tasks.Summary) bool { return s.ID == "task-1" && s.Title == "Renamed in the gap" })
	if listed < 0 {
		t.Error("the list should show task-1 with the rename's title")
	}
}

// The summariser gives nil for a task the state can't hand out, archived
// or broken, for the rules to refuse: Archive calls it before they run.
func TestTheSummariserGivesNilForATaskTheStateCantHandOut(t *testing.T) {
	path, _ := seededLog(t, someProposals()...)
	state := New(withIdeas, ideas.Streams())
	s, err := store.Open(path, withIdeas, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	archiveTask(t, s, state, "task-1", sharedfiles.Disk{Dir: t.TempDir()})
	for _, id := range []string{"task-1", "task-3"} { // archived, broken
		if summary := summarise(state, id)(); summary != nil {
			t.Errorf("the summary of %s is %v, want nil", id, summary)
		}
	}
}
