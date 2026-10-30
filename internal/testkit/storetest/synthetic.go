package storetest

import (
	"database/sql"
	"encoding/json"
	"io"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
)

// A synthetic log is for benchmarks that need more history than a copy of a
// real log holds (docs/design/log-growth.md, section 6, "Targets"): the
// copy's own events, then its work again in rounds at later times, as if the
// same work had gone on, until the file reaches a size.

const (
	// WorkingDays is how long a done task stays whole in the state before
	// the archiver takes it: the default archive age.
	WorkingDays = 30
	// openTasks and taskEventsADay make the working set W: 20 open tasks,
	// and a busy day's events on task streams (9,700 × 0.955), so the last
	// WorkingDays hold W's done work.
	openTasks      = 20
	taskEventsADay = 9_264
)

// Synthetic says what Grow made.
type Synthetic struct {
	Rounds float64   // the copy's work, its own included; the last round may be part of one
	Tasks  int       // tasks in the log
	Open   int       // of them, the ones left open
	End    time.Time // the time of the log's last event
	Round  Mix       // what one whole round adds, its File left 0
}

// Grow copies the log at source to path and appends to the copy until its
// file holds about size bytes. The source is only read.
//
// Each round repeats the copy's task streams under the next task ids,
// numbered in the order they are created, as Store.Create numbers them;
// the rows of the agents and workers streams that change an agent or
// worker already there: what an agent learned, its changes, workers
// connecting and leaving; and the policy and settings rows, which set again
// what they set before. Creating, renaming or removing an agent, worker or
// connection again would make a roster the copy never had, and ideas are
// left as they are. Time moves on at taskEventsADay task events a day;
// working scales W, 2 for twice the open tasks at twice the rate. Only the
// newest open tasks stay open: each of the others gets a task.done at the
// end of its round. The rows it never repeats are why the copy's type
// shares differ from a round's, and why a large log's come near Round's.
func Grow(t testing.TB, source, path string, size int64, working int) Synthetic {
	t.Helper()
	copyFile(t, source, path)
	rows := readRows(t, source)
	if len(rows) == 0 {
		t.Fatal("the copy has no events to grow from")
	}
	units := unitsOf(t, rows)
	rounds := plan(t, source, size, rows, units)
	made := numberTasks(t, rows, rounds)
	wasOpen := openAtEnd(rows)
	open := keptOpen(rounds, wasOpen, openTasks*working)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first, last := rows[0].at, rows[len(rows)-1].at
	end := last
	for i, r := range rounds {
		if i > 0 {
			start := end
			span := time.Duration(float64(r.taskRows) / float64(taskEventsADay*working) * float64(24*time.Hour))
			appendRows(t, db, r.copied(rows, func(at time.Time) time.Time {
				return start.Add(time.Duration(float64(at.Sub(first)) / float64(last.Sub(first)) * float64(span)))
			}))
			end = start.Add(span)
		}
		appendRows(t, db, r.closing(wasOpen, open, end))
	}
	grown := Synthetic{Tasks: made, Open: len(open), End: end, Round: mixOfRound(rows, units)}
	for _, r := range rounds {
		grown.Rounds += r.part
	}
	return grown
}

// row is one event as stored, its data still JSON.
type row struct {
	position                                           int64
	at                                                 time.Time
	stream, actorKind, actorID, typ, correlation, data string
}

// size is about what the row takes in the file, before SQLite's own
// overhead, which plan measures on the copy.
func (r row) size() int {
	return len(r.data) + len(r.stream) + len(r.actorKind) + len(r.actorID) + len(r.typ) + len(r.correlation) + len(time.RFC3339Nano)
}

// unit is rows a round copies together or not at all: a task's stream, an
// agent's knowledge from one full copy up to the next (a patch needs the
// version before it), or one other row. Its rows are indexes into the
// copy's, in order.
type unit struct {
	rows []int
	task string // the task's stream, for a task's unit
}

// repeated are the rows of the shared streams a round copies, besides what
// agents learned.
var repeated = map[string][]events.Type{
	events.AgentsStream:   {events.AgentChanged},
	events.WorkersStream:  {events.WorkerConnected, events.WorkerDisconnected, events.ProposalRejected},
	events.PolicyStream:   {events.PolicyLoaded, events.PolicyRefused, events.ProposalRejected},
	events.SettingsStream: {events.SettingsChanged},
}

// unitsOf is every unit a round copies, in the order of their first rows.
func unitsOf(t testing.TB, rows []row) []*unit {
	t.Helper()
	var units []*unit
	tasks := map[string]*unit{}
	knowledge := map[string]*unit{} // by agent: the unit its next patch joins
	for i, r := range rows {
		typ := events.Type(r.typ)
		switch {
		case events.Core().KindOf(r.stream) == events.TaskStream:
			if tasks[r.stream] == nil {
				tasks[r.stream] = &unit{task: r.stream}
				units = append(units, tasks[r.stream])
			}
			tasks[r.stream].rows = append(tasks[r.stream].rows, i)
		case r.stream == events.AgentsStream && typ == events.AgentLearned:
			p := events.Proposal{Type: typ}
			if err := json.Unmarshal([]byte(r.data), &p.Data); err != nil {
				t.Fatalf("position %d: %v", r.position, err)
			}
			name, _ := p.Data["name"].(string)
			if !agents.IsPatch(p) || knowledge[name] == nil {
				knowledge[name] = &unit{}
				units = append(units, knowledge[name])
			}
			knowledge[name].rows = append(knowledge[name].rows, i)
		case slices.Contains(repeated[r.stream], typ):
			units = append(units, &unit{rows: []int{i}})
		}
	}
	return units
}

// mixOfRound is the mix of the rows a whole round copies.
func mixOfRound(rows []row, units []*unit) Mix {
	mix := Mix{Types: map[events.Type]Count{}}
	for _, u := range units {
		for _, i := range u.rows {
			typ := events.Type(rows[i].typ)
			c := mix.Types[typ]
			c.Events++
			c.Bytes += int64(len(rows[i].data))
			mix.Types[typ] = c
			mix.Events++
		}
	}
	return mix
}

// round is the copy itself, the first, or one copy of its work.
type round struct {
	units    []*unit           // what it copies: none for the copy itself
	part     float64           // of a whole round
	taskRows int               // of its units, the rows on task streams
	ids      map[string]string // each of the copy's tasks it has, to its id in this round
}

// plan is the rounds: the copy, then whole rounds while they fit, then
// part of one, its units evenly spaced so it keeps the mix. A row takes as
// many bytes of the file as a row of the copy does.
func plan(t testing.TB, source string, size int64, rows []row, units []*unit) []*round {
	t.Helper()
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	inRows, whole := 0, 0
	for _, r := range rows {
		inRows += r.size()
	}
	for _, u := range units {
		for _, i := range u.rows {
			whole += rows[i].size()
		}
	}
	perRowByte := float64(info.Size()) / float64(inRows)
	rounds := []*round{{part: 1}}
	left := float64(size-info.Size()) / perRowByte / float64(whole)
	for ; left >= 1; left-- {
		rounds = append(rounds, newRound(rows, units, 1))
	}
	if left > 0 {
		var some []*unit
		for i, u := range units {
			if math.Floor(float64(i+1)*left) > math.Floor(float64(i)*left) {
				some = append(some, u)
			}
		}
		rounds = append(rounds, newRound(rows, some, left))
	}
	return rounds
}

func newRound(rows []row, units []*unit, part float64) *round {
	r := &round{units: units, part: part}
	for _, u := range units {
		if u.task != "" {
			r.taskRows += len(u.rows)
		}
	}
	return r
}

// numberTasks gives each round's tasks their ids and returns how many tasks the
// log makes. The copy's own must be task-1 to task-N, created in that
// order, or the copies could take an id already used.
func numberTasks(t testing.TB, rows []row, rounds []*round) int {
	t.Helper()
	made := 0
	rounds[0].ids = map[string]string{}
	for _, r := range rows {
		if events.Type(r.typ) != events.TaskCreated {
			continue
		}
		if r.stream != events.Numbered(events.TaskPrefix, made) {
			t.Fatalf("task.created number %d is in %s: the copy's tasks aren't numbered in the order they were made", made+1, r.stream)
		}
		rounds[0].ids[r.stream] = r.stream
		made++
	}
	for _, r := range rounds[1:] {
		r.ids = map[string]string{}
		for _, u := range r.units {
			if u.task != "" {
				r.ids[u.task] = events.Numbered(events.TaskPrefix, made)
				made++
			}
		}
	}
	return made
}

// openAtEnd is the copy's tasks that are open after its last event: never
// done, or reopened since.
func openAtEnd(rows []row) map[string]bool {
	open := map[string]bool{}
	for _, r := range rows {
		switch events.Type(r.typ) {
		case events.TaskCreated, events.TaskReopened:
			open[r.stream] = true
		case events.TaskDone:
			delete(open, r.stream)
		}
	}
	return open
}

// keptOpen is the ids of the newest kept of every round's tasks that are
// open at the copy's end.
func keptOpen(rounds []*round, wasOpen map[string]bool, kept int) map[string]bool {
	var ids []string
	for _, r := range rounds {
		for old, id := range r.ids {
			if wasOpen[old] {
				ids = append(ids, id)
			}
		}
	}
	slices.SortFunc(ids, func(a, b string) int { return taskNumber(b) - taskNumber(a) })
	open := map[string]bool{}
	for _, id := range ids[:min(kept, len(ids))] {
		open[id] = true
	}
	return open
}

// taskID matches a task's id in a row's data, such as the epic a story
// names, so a copied task names the copy of its epic.
var taskID = regexp.MustCompile(`\btask-[0-9]+\b`)

// copied is the round's rows, in the copy's order, under their new ids and
// at the times moved gives.
func (r *round) copied(rows []row, moved func(time.Time) time.Time) []row {
	var picked []int
	for _, u := range r.units {
		picked = append(picked, u.rows...)
	}
	slices.Sort(picked)
	out := make([]row, len(picked))
	for i, index := range picked {
		c := rows[index]
		c.at = moved(c.at)
		if id, ok := r.ids[c.stream]; ok {
			c.stream = id
			c.data = taskID.ReplaceAllStringFunc(c.data, func(old string) string {
				if id, ok := r.ids[old]; ok {
					return id
				}
				return old // a task this round doesn't copy
			})
		}
		out[i] = c
	}
	return out
}

func taskNumber(id string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(id, events.TaskPrefix))
	return n
}

// closing is a task.done, at the end of the round, for each of its tasks
// that is open at the copy's end and not kept open.
func (r *round) closing(wasOpen, open map[string]bool, at time.Time) []row {
	var out []row
	for old, id := range r.ids {
		if wasOpen[old] && !open[id] {
			out = append(out, row{at: at, stream: id, actorKind: string(events.Agent), actorID: "synthetic", typ: string(events.TaskDone),
				data: `{"summary":"Done by the synthetic log, which keeps only the newest tasks open."}`})
		}
	}
	slices.SortFunc(out, func(a, b row) int { return taskNumber(a.stream) - taskNumber(b.stream) })
	return out
}

// appendRows appends rows to the log in one transaction, in order.
func appendRows(t testing.TB, db *sql.DB, rows []row) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	insert, err := tx.Prepare(`INSERT INTO events (at, stream, actor_kind, actor_id, type, correlation, data) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer insert.Close()
	for _, r := range rows {
		if _, err := insert.Exec(r.at.UTC().Format(time.RFC3339Nano), r.stream, r.actorKind, r.actorID, r.typ, r.correlation, r.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func readRows(t testing.TB, path string) []row {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	result, err := db.Query(`SELECT position, at, stream, actor_kind, actor_id, type, correlation, data FROM events ORDER BY position`)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	var rows []row
	for result.Next() {
		var r row
		var at string
		if err := result.Scan(&r.position, &at, &r.stream, &r.actorKind, &r.actorID, &r.typ, &r.correlation, &r.data); err != nil {
			t.Fatal(err)
		}
		if r.at, err = time.Parse(time.RFC3339Nano, at); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, r)
	}
	if err := result.Err(); err != nil {
		t.Fatal(err)
	}
	return rows
}

// copyFile copies a log, reading the source and writing only the copy.
func copyFile(t testing.TB, source, path string) {
	t.Helper()
	from, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer from.Close()
	to, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(to, from); err != nil {
		to.Close()
		t.Fatal(err)
	}
	if err := to.Close(); err != nil {
		t.Fatal(err)
	}
}

// Mix is what a log is made of: its events, its file's size, and each
// event type's events and bytes of data.
type Mix struct {
	Events int
	File   int64
	Types  map[events.Type]Count
}

// Count is one event type's events and bytes of data.
type Count struct {
	Events int
	Bytes  int64
}

// MixOf reads a log's mix, opening it read-only.
func MixOf(t testing.TB, path string) Mix {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	result, err := db.Query(`SELECT type, COUNT(*), SUM(length(CAST(data AS BLOB))) FROM events GROUP BY type`)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	mix := Mix{File: info.Size(), Types: map[events.Type]Count{}}
	for result.Next() {
		var typ events.Type
		var c Count
		if err := result.Scan(&typ, &c.Events, &c.Bytes); err != nil {
			t.Fatal(err)
		}
		mix.Types[typ] = c
		mix.Events += c.Events
	}
	if err := result.Err(); err != nil {
		t.Fatal(err)
	}
	return mix
}
