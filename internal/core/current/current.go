// Package current keeps the state of every stream in memory, as of the
// last event the log committed. The log stays the only truth: the state is
// built from it at every start and changed only by the store, as each event
// commits, so it can't disagree with what happened. A start may read the
// state from a snapshot instead and apply only the events after it, but a
// snapshot is only a cache, thrown away whenever it might disagree with
// the log (snapshot.go).
//
// The catalogue the state is made with says what each stream is. A task's
// stream makes a task, and an extension's own stream whatever that extension's
// Streams build from it: an idea, never a task. The agents, workers and
// connections streams each make a roster, the settings stream your
// settings, and the templates stream the team templates you saved. The
// policy stream keeps no state here: its events only move the position.
// A foreign stream, which an extension this build doesn't have wrote, is
// skipped and only counted, as is an event of a type the catalogue doesn't
// know in any other stream.
package current

import (
	"errors"
	"maps"
	"slices"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/connections"
	"gitlab.com/dev.chempa/agentry/internal/domain/settings"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// State is every stream's state. A *tasks.Task or a roster it hands out is
// never changed afterwards: applying an event replaces it with a new one,
// so readers may keep what they got as long as they like, without a lock.
// They must not change it themselves.
type State struct {
	mu          sync.RWMutex
	catalogue   *events.Catalogue
	position    int64                          // of the last event applied, rejections included
	streams     map[string]int64               // each stream's last accepted event applied, by stream
	tasks       map[string]*tasks.Task         // by stream
	created     []created                      // every task.created applied, in the order of their positions, as the log lists them
	tasksMade   int                            // every task.created applied, a foreign stream's too, as Store.Create counts them
	owners      map[string]*extensions.Streams // by prefix: the extensions whose streams the state keeps
	owned       map[string]any                 // an extension's stream's state, as its Streams built it, by stream
	ownedOrder  map[string][]created           // by prefix: each owned stream's first event applied, in the order of their positions
	broken      map[string]error               // streams whose apply panicked: reads fail, as a replay would
	archived    map[string]tasks.Summary       // tasks the state holds as their summary alone, by stream
	erased      map[string]tasks.Erasure       // tasks erased for good, whose ids stay taken, by stream
	roster      agents.Roster
	workers     workers.Roster
	connections connections.Roster
	settings    settings.Settings
	templates   agents.UserTemplates
	removed     agents.RemovedTemplates // built from the templates stream too
	skipped     tally                   // foreign streams and unknown types

	snapshot *snapshotAt   // read from a snapshot, still to be checked against the log by Resume
	tail     *tail         // set while Resume applies the events after a snapshot
	resumed  int64         // the snapshot position Open went on from, 0 after a whole replay
	moved    chan struct{} // sent on, if nothing waits in it, when the position moves: a SnapshotWriter's wake
}

var _ extensions.Reader = (*State)(nil)

// created is a task.created event's place in the log, or the first event of
// an extension's stream. The task and owned stream orders are by these
// positions, not by when the state took each event, so a state that starts
// part way and takes the rest keeps the log's order.
type created struct {
	position int64
	id       string
}

// ErrArchived is Task's error for an archived task: a done task the state
// holds no full state for, which takes no more events. Its readers skip it
// as they skip a done task, without reading its stream.
var ErrArchived = errors.New("the task is archived")

// ErrErased is Task's error for an erased task: one the state holds only
// as its id and when it went, which takes no events, ever again. Its
// readers skip it as they skip an archived task.
var ErrErased = errors.New("the task is erased")

// New is an empty state that reads streams as catalogue says, which must be
// the catalogue the store is opened with, and keeps the streams of each
// extension in owned. A stream the catalogue doesn't give to an extension is
// never one of owned's, whatever its name: without the registration it is
// foreign. An owned stream with no Streams here keeps no state.
func New(catalogue *events.Catalogue, owned ...*extensions.Streams) *State {
	owners := map[string]*extensions.Streams{}
	for _, streams := range owned {
		if owners[streams.Prefix] != nil {
			panic("current: two extensions own the streams " + streams.Prefix)
		}
		owners[streams.Prefix] = streams
	}
	s := &State{catalogue: catalogue, owners: owners, moved: make(chan struct{}, 1)}
	s.empty()
	return s
}

// empty sets everything the log fills back to nothing, as New makes it:
// for New, and for Resume to throw a snapshot away. A field the log fills
// must be set here, even to its zero value, or a thrown away snapshot
// leaves it behind.
func (s *State) empty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.position, s.streams, s.tasks = 0, map[string]int64{}, map[string]*tasks.Task{}
	s.created, s.tasksMade = nil, 0
	s.owned, s.ownedOrder = map[string]any{}, map[string][]created{}
	s.broken, s.archived, s.erased = map[string]error{}, map[string]tasks.Summary{}, map[string]tasks.Erasure{}
	s.roster, s.workers, s.connections, s.settings = nil, workers.Roster{}, connections.Roster{}, settings.Default()
	s.templates, s.removed = nil, nil
	s.skipped = newTally()
	s.snapshot, s.tail, s.resumed = nil, nil, 0
}

// copy is the state as it is now, in maps of its own, so it can be read
// while the store goes on applying events to s. The tasks, owned streams'
// states and rosters in it are shared: no one changes them.
func (s *State) copy() *State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ownedOrder := map[string][]created{}
	for prefix, order := range s.ownedOrder {
		ownedOrder[prefix] = slices.Clone(order)
	}
	return &State{
		position:    s.position,
		streams:     maps.Clone(s.streams),
		tasks:       maps.Clone(s.tasks),
		created:     slices.Clone(s.created),
		tasksMade:   s.tasksMade,
		owners:      s.owners,
		owned:       maps.Clone(s.owned),
		ownedOrder:  ownedOrder,
		broken:      maps.Clone(s.broken),
		archived:    maps.Clone(s.archived),
		erased:      maps.Clone(s.erased),
		roster:      s.roster,
		workers:     s.workers,
		connections: s.connections,
		settings:    s.settings,
		templates:   s.templates,
		removed:     s.removed,
		skipped:     s.skipped.clone(),
	}
}
