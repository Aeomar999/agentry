package current

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Notes are a test extension that owns the note- streams: a note is the text
// of every note.written, and a note.spoiled panics, in FromEvents and Next
// alike, as an extension's bug would.
const (
	noteWritten events.Type = "note.written"
	noteSpoiled events.Type = "note.spoiled"
)

type note struct{ texts []string }

func (n *note) apply(e events.Event) {
	if e.Type == noteSpoiled {
		panic("a spoiled note")
	}
	n.texts = append(n.texts, e.Text("text"))
}

var notes = &extensions.Streams{
	Prefix: "note-",
	FromEvents: func(accepted []events.Event) any {
		n := &note{}
		for _, e := range accepted {
			n.apply(e)
		}
		return n
	},
	Next: func(prev any, e events.Event) any {
		n := &note{}
		if held, ok := prev.(*note); ok {
			n.texts = slices.Clone(held.texts)
		}
		n.apply(e)
		return n
	},
}

var withNotes = func() *events.Catalogue {
	writers := []events.ActorKind{events.Human}
	catalogue, err := events.NewCatalogue(extensions.Registration(ideas.Extension()), events.Registration{Prefix: notes.Prefix, Types: map[events.Type]events.Spec{
		noteWritten: {Fields: []string{"text"}, Writers: writers},
		noteSpoiled: {Writers: writers},
	}})
	if err != nil {
		panic(err)
	}
	return catalogue
}()

// notesLog is withIdeasLog with two notes, note-2 first, between its events.
func notesLog(more ...events.Proposal) []events.Event {
	var proposals []events.Proposal
	for i, e := range withIdeasLog() {
		proposals = append(proposals, e.Proposal)
		switch i {
		case 2:
			proposals = append(proposals, proposal("note-2", you, noteWritten, map[string]any{"text": "first"}))
		case 5:
			proposals = append(proposals, proposal("note-1", you, noteWritten, map[string]any{"text": "second"}))
		case 8:
			proposals = append(proposals, proposal("note-2", you, noteWritten, map[string]any{"text": "again"}))
		}
	}
	return logOf(append(proposals, more...)...)
}

func newWithNotes() *State { return New(withNotes, ideas.Streams(), notes) }

func TestAnExtensionsStreamsAreKeptInLogOrder(t *testing.T) {
	log := notesLog()
	for name, s := range map[string]*State{"applied": applyInto(newWithNotes(), log), "loaded": loadInto(newWithNotes(), log)} {
		if order := s.OwnedIDs(notes.Prefix); !slices.Equal(order, []string{"note-2", "note-1"}) {
			t.Errorf("%s: the notes are %v, want note-2 then note-1 as the log has them", name, order)
		}
		if held, err := s.Owned("note-2"); err != nil || !reflect.DeepEqual(held, &note{texts: []string{"first", "again"}}) {
			t.Errorf("%s: note-2 is %+v, %v", name, held, err)
		}
		if order := s.OwnedIDs(ideas.Prefix); !slices.Equal(order, []string{"idea-1", "idea-2"}) {
			t.Errorf("%s: the ideas are %v", name, order)
		}
		if held, err := s.Owned("note-9"); held != nil || err != nil {
			t.Errorf("%s: a note never written should hold nothing, got %+v, %v", name, held, err)
		}
	}
}

// An extension whose Next panics breaks only the stream it was applying: its
// other streams, the ideas, the tasks and the rosters read as they would
// without the event.
func TestAnExtensionsPanicInNextBreaksOnlyItsStream(t *testing.T) {
	log := notesLog()
	spoiled := events.Event{Position: 100, Proposal: proposal("note-1", you, noteSpoiled, nil)}
	more := events.Event{Position: 101, Proposal: proposal("note-1", you, noteWritten, map[string]any{"text": "never applied"})}
	for name, s := range map[string]*State{"applied": applyInto(newWithNotes(), log), "loaded": loadInto(newWithNotes(), log)} {
		s.Apply(spoiled)
		s.Apply(more)
		brokenOnly(t, name, s, applyInto(newWithNotes(), log), "note-1")
	}
}

// An extension whose FromEvents panics at Load breaks only that stream, as
// its Next would.
func TestAnExtensionsPanicInFromEventsBreaksOnlyItsStream(t *testing.T) {
	log := notesLog(proposal("note-1", you, noteSpoiled, nil))
	without := notesLog()
	s := loadInto(newWithNotes(), log)
	brokenOnly(t, "loaded", s, loadInto(newWithNotes(), without), "note-1")
	if err := s.Broken("note-1"); !strings.Contains(err.Error(), "building note-1: a spoiled note") {
		t.Errorf("note-1 broke with %v, want what building it hit", err)
	}
}

// brokenOnly fails the test unless stream is broken in s and every other
// stream reads as it does in want.
func brokenOnly(t *testing.T, name string, s, want *State, stream string) {
	t.Helper()
	if err := s.Broken(stream); err == nil || !strings.Contains(err.Error(), "a spoiled note") {
		t.Errorf("%s: Broken(%s) is %v, want the panic", name, stream, err)
	}
	if held, err := s.Owned(stream); held != nil || err == nil {
		t.Errorf("%s: %s should fail with its stream's error, got %+v, %v", name, stream, held, err)
	}
	for _, prefix := range []string{notes.Prefix, ideas.Prefix} {
		if order, wanted := s.OwnedIDs(prefix), want.OwnedIDs(prefix); !slices.Equal(order, wanted) {
			t.Errorf("%s: the %s streams are %v, want %v, the broken one listed too", name, prefix, order, wanted)
		}
		for _, id := range want.OwnedIDs(prefix) {
			if id == stream {
				continue
			}
			held, err := s.Owned(id)
			wanted, _ := want.Owned(id)
			if err != nil || !reflect.DeepEqual(held, wanted) {
				t.Errorf("%s: %s is %+v, %v, want %+v", name, id, held, err, wanted)
			}
		}
	}
	if !reflect.DeepEqual(ideas.All(s), ideas.All(want)) {
		t.Errorf("%s: the ideas changed", name)
	}
	if order := s.TaskIDs(); !slices.Equal(order, want.TaskIDs()) {
		t.Errorf("%s: the task order is %v, want %v", name, order, want.TaskIDs())
	}
	for _, id := range want.TaskIDs() {
		task, err := s.Task(id)
		wanted, _ := want.Task(id)
		if err != nil || !reflect.DeepEqual(task, wanted) {
			t.Errorf("%s: %s is %+v, %v", name, id, task, err)
		}
	}
	rosters := map[string][2]any{
		"roster":      {s.Roster(), want.Roster()},
		"workers":     {s.Workers(), want.Workers()},
		"connections": {s.Connections(), want.Connections()},
		"settings":    {s.Settings(), want.Settings()},
	}
	for what, both := range rosters {
		if !reflect.DeepEqual(both[0], both[1]) {
			t.Errorf("%s: the %s changed", name, what)
		}
	}
}

// Equal compares what each owned stream holds with its replay, so it sees
// an extension whose Next is subtly wrong though every stream's position and
// the order are right.
func TestEqualSeesAnExtensionsWrongNext(t *testing.T) {
	forgetful := *notes
	forgetful.Next = func(prev any, e events.Event) any {
		n := notes.Next(prev, e).(*note)
		if len(n.texts) > 1 {
			n.texts = n.texts[1:] // forgets the first text once there is another
		}
		return n
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), withNotes, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, e := range notesLog() {
		propose(t, s, e.Proposal)
	}
	state := New(withNotes, ideas.Streams(), &forgetful)
	state.Loaded(0)
	fromTheLog(t, s, state)
	if err := Equal(s, state); err == nil || !strings.HasPrefix(err.Error(), "note-2:") {
		t.Errorf("Equal with a Next that forgets note-2's first text: %v, want it to name note-2", err)
	}
}

// Every log these tests make, written to a store: a state that applied it
// event by event, and one loaded from it, equal a replay, every extension's
// streams and their order included. A spoiled note breaks in both and in
// the replay alike.
func TestEveryTestLogAppliedOrLoadedEqualsAReplay(t *testing.T) {
	logs := map[string][]events.Event{
		"every stream":  everyStream(),
		"with ideas":    withIdeasLog(),
		"with notes":    notesLog(),
		"spoiled note":  notesLog(proposal("note-1", you, noteSpoiled, nil), proposal("note-2", you, noteWritten, map[string]any{"text": "after"})),
		"refused shape": logOf(someProposals()...),
	}
	for name, log := range logs {
		path := filepath.Join(t.TempDir(), "agentry.db")
		s, err := store.Open(path, withNotes, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range log {
			propose(t, s, e.Proposal)
		}
		applied := newWithNotes()
		applied.Loaded(0)
		fromTheLog(t, s, applied)
		if err := Equal(s, applied); err != nil {
			t.Errorf("%s, applied: %v", name, err)
		}
		loaded := newWithNotes()
		again, err := store.Open(path, withNotes, loaded, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := Equal(again, loaded); err != nil {
			t.Errorf("%s, loaded: %v", name, err)
		}
		if order := loaded.OwnedIDs(notes.Prefix); strings.Contains(name, "note") && len(order) != 2 {
			t.Errorf("%s: the notes are %v", name, order)
		}
		again.Close()
		s.Close()
	}
}

// A prefix no extension here owns lists no streams, a stream no extension here
// owns holds nothing, though the catalogue knows it, and what OwnedIDs hands
// out is the caller's to change.
func TestWhatNoExtensionHereOwnsHoldsNothing(t *testing.T) {
	log := notesLog()
	for name, s := range map[string]*State{"applied": applyInto(New(withNotes, ideas.Streams()), log), "loaded": loadInto(New(withNotes, ideas.Streams()), log)} {
		if order := s.OwnedIDs(notes.Prefix); len(order) != 0 {
			t.Errorf("%s: with no notes extension the notes are %v, want none", name, order)
		}
		if order := s.OwnedIDs("nope-"); len(order) != 0 {
			t.Errorf("%s: an unknown prefix lists %v, want none", name, order)
		}
		for _, stream := range []string{"note-2", "task-1", "nope-1"} {
			if held, err := s.Owned(stream); held != nil || err != nil {
				t.Errorf("%s: %s should hold nothing, got %+v, %v", name, stream, held, err)
			}
		}
		order := s.OwnedIDs(ideas.Prefix)
		order[0] = "idea-9"
		if again := s.OwnedIDs(ideas.Prefix); !slices.Equal(again, []string{"idea-1", "idea-2"}) {
			t.Errorf("%s: changing what OwnedIDs gave changed the state: %v", name, again)
		}
	}
}
