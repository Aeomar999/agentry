package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// TestLogGrowthBench measures, on a copy of a real agentry.db, what startup
// replay, snapshots, archiving and slimmer events would cost or save, for
// docs/design/log-growth.md. It is skipped unless AGENTRY_BENCH_DB names the
// copy, which it copies again so the run never writes to it:
//
//	AGENTRY_BENCH_DB=$PWD/.agentry/files/bench/agentry.db \
//	  go test ./internal/web -run TestLogGrowthBench -v -count=1
func TestLogGrowthBench(t *testing.T) {
	source := os.Getenv("AGENTRY_BENCH_DB")
	if source == "" {
		t.Skip("AGENTRY_BENCH_DB is not set")
	}
	path := filepath.Join(t.TempDir(), "agentry.db")
	copyFile(t, source, path)
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	rows := readRows(t, raw)
	t.Logf("%d events, %d bytes of data", len(rows), dataBytes(rows))

	state := benchReplay(t, path, len(rows))
	benchSnapshot(t, state)
	archived := benchArchive(t, rows, state)
	benchSlim(t, rows, archived)
}

// replayed is every stream's state, as startup with a read model builds it.
type replayed struct {
	tasks       map[string]*tasks.Task
	order       []string
	roster      agents.Roster
	workers     workers.Roster
	connections connections.Roster
}

// benchReplay times what startup does with the read model: open the store,
// read every stream and fold it. Min of 3, each on a fresh store.
func benchReplay(t *testing.T, path string, total int) replayed {
	var state replayed
	var read, fold time.Duration
	whole := minOf3(func() {
		start := time.Now()
		s, err := store.Open(path, rules.Check)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		ids, err := s.TaskIDs()
		if err != nil {
			t.Fatal(err)
		}
		streams := readStreams(t, s, append(slices.Clone(ids), events.AgentsStream, events.WorkersStream, events.ConnectionsStream))
		readTook := time.Since(start)
		start = time.Now()
		state = replayed{tasks: map[string]*tasks.Task{}, order: ids}
		for _, id := range ids {
			state.tasks[id] = tasks.FromEvents(streams[id])
		}
		state.roster = agents.FromEvents(streams[events.AgentsStream])
		state.workers = workers.FromEvents(streams[events.WorkersStream])
		state.connections = connections.FromEvents(streams[events.ConnectionsStream])
		foldTook := time.Since(start)
		if read == 0 || readTook < read {
			read = readTook
		}
		if fold == 0 || foldTook < fold {
			fold = foldTook
		}
	})
	t.Log("startup replay (min of 3):")
	t.Logf("  open, read and decode every stream: %s", read.Round(time.Millisecond))
	t.Logf("  fold every stream:                  %s", fold.Round(time.Millisecond))
	t.Logf("  whole:                              %s, %s per 100k events", whole.Round(time.Millisecond),
		(whole * 100_000 / time.Duration(total)).Round(time.Millisecond))
	// The state built from events that are then dropped, so the texts it
	// keeps count too (readmodel_bench_test.go kept the events alive).
	held := heldBy(func() any {
		s, err := store.Open(path, rules.Check)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		kept := map[string]*tasks.Task{}
		for _, id := range state.order {
			kept[id] = tasks.FromEvents(must(s.Stream(id)))
		}
		return []any{kept, agents.FromEvents(must(s.Stream(events.AgentsStream)))}
	})
	t.Logf("  heap held by the state alone: %d KB", held/1024)
	return state
}

// snapshotTask mirrors tasks.Task with its two unexported fields, which an
// encoder can't reach: a snapshot needs them, or an equal replay is lost.
type snapshotTask struct {
	Task      *tasks.Task
	Collected map[string]bool
	StoryNews bool
}

type snapshot struct {
	Position int64
	Tasks    []snapshotTask
	Roster   agents.Roster
	Workers  []workers.Worker
}

func mirror(task *tasks.Task) snapshotTask {
	v := reflect.ValueOf(task).Elem()
	unexported := func(name string) reflect.Value {
		f := v.FieldByName(name)
		return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	}
	return snapshotTask{Task: task,
		Collected: unexported("collected").Interface().(map[string]bool),
		StoryNews: unexported("storyNews").Bool()}
}

// restore sets the unexported fields of a decoded task back.
func restore(task snapshotTask) {
	v := reflect.ValueOf(task.Task).Elem()
	unexported := func(name string) reflect.Value {
		f := v.FieldByName(name)
		return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	}
	unexported("collected").Set(reflect.ValueOf(task.Collected))
	unexported("storyNews").SetBool(task.StoryNews)
}

func snapshotOf(state replayed) snapshot {
	snap := snapshot{Roster: state.roster, Workers: state.workers.Workers}
	for _, id := range state.order {
		snap.Tasks = append(snap.Tasks, mirror(state.tasks[id]))
	}
	return snap
}

func benchSnapshot(t *testing.T, state replayed) {
	snap := snapshotOf(state)
	t.Log("snapshot of the state (min of 3):")
	codecs := []struct {
		name   string
		encode func(any) []byte
		decode func([]byte, any)
	}{
		{"JSON", func(v any) []byte { return must(json.Marshal(v)) }, func(b []byte, v any) { check(json.Unmarshal(b, v)) }},
		{"gob", gobEncode, func(b []byte, v any) { check(gob.NewDecoder(bytes.NewReader(b)).Decode(v)) }},
	}
	for _, codec := range codecs {
		var plain, zipped []byte
		encode := minOf3(func() { plain = codec.encode(snap) })
		zip := minOf3(func() { zipped = gzipped(plain) })
		decode := minOf3(func() { var back snapshot; codec.decode(plain, &back) })
		unzip := minOf3(func() { gunzipped(zipped) })
		t.Logf("  %-4s %9d bytes, encode %s, decode %s; gzip %8d bytes, +%s to write, +%s to read",
			codec.name, len(plain), encode.Round(100*time.Microsecond), decode.Round(100*time.Microsecond),
			len(zipped), zip.Round(100*time.Microsecond), unzip.Round(100*time.Microsecond))
	}
	for _, codec := range codecs {
		var back snapshot
		codec.decode(codec.encode(snap), &back)
		equal := len(back.Tasks) == len(snap.Tasks) && reflect.DeepEqual(snap.Roster, back.Roster)
		for i, task := range back.Tasks {
			restore(task)
			equal = equal && reflect.DeepEqual(state.tasks[state.order[i]], task.Task)
		}
		t.Logf("  %s round trip equals the replay, tasks and agents, once collected and storyNews are set back: %v", codec.name, equal)
		t.Logf("  %s round trip equals the workers: %v (Worker.credential is unexported, and lost)", codec.name, reflect.DeepEqual(snap.Workers, back.Workers))
	}

	t.Log("  JSON bytes by part:")
	t.Logf("    roster (knowledge included): %d", len(must(json.Marshal(snap.Roster))))
	t.Logf("    workers:                     %d", len(must(json.Marshal(snap.Workers))))
	var actions, turns, rest int
	sizes := map[bool][]int{}
	for _, task := range snap.Tasks {
		size := len(must(json.Marshal(task)))
		a, tu := len(must(json.Marshal(task.Task.Actions))), len(must(json.Marshal(task.Task.Turns)))
		actions, turns, rest = actions+a, turns+tu, rest+size-a-tu
		sizes[task.Task.IsDone] = append(sizes[task.Task.IsDone], size)
	}
	t.Logf("    tasks: actions %d, turns %d, the rest %d", actions, turns, rest)
	for _, done := range []bool{false, true} {
		label := "open"
		if done {
			label = "done"
		}
		s := sizes[done]
		t.Logf("    per %s task: %d tasks, mean %d, largest %d", label, len(s), mean(s), slices.Max(append(s, 0)))
	}
	var withoutActions []int
	for _, task := range snap.Tasks {
		thin := *task.Task
		thin.Actions = nil
		withoutActions = append(withoutActions, len(must(json.Marshal(thin))))
	}
	t.Logf("    per task without its actions: mean %d, largest %d", mean(withoutActions), slices.Max(withoutActions))
}

// row is one event as stored, with its data still JSON.
type row struct {
	position int64
	at       string
	stream   string
	kind, id string
	typ      string
	corr     string
	data     string
}

func readRows(t *testing.T, raw *sql.DB) []row {
	t.Helper()
	found, err := raw.Query(`SELECT ` + `position, at, stream, actor_kind, actor_id, type, correlation, data` + ` FROM events ORDER BY position`)
	if err != nil {
		t.Fatal(err)
	}
	defer found.Close()
	var rows []row
	for found.Next() {
		var r row
		check(found.Scan(&r.position, &r.at, &r.stream, &r.kind, &r.id, &r.typ, &r.corr, &r.data))
		rows = append(rows, r)
	}
	check(found.Err())
	return rows
}

func dataBytes(rows []row) int {
	n := 0
	for _, r := range rows {
		n += len(r.data)
	}
	return n
}

// benchArchive finds what share of the log belongs to done tasks, by how
// long ago they were done (from the log's last event), and returns the
// streams done at all.
func benchArchive(t *testing.T, rows []row, state replayed) map[string]bool {
	last, _ := time.Parse(time.RFC3339Nano, rows[len(rows)-1].at)
	doneAt := map[string]time.Time{}
	for _, r := range rows {
		if r.typ == string(events.TaskDone) {
			doneAt[r.stream], _ = time.Parse(time.RFC3339Nano, r.at)
		}
	}
	total, totalBytes := len(rows), dataBytes(rows)
	t.Logf("archiving done tasks (the log ends %s):", last.Format(time.RFC3339))
	doneAtAll := map[string]bool{}
	for _, age := range []time.Duration{0, 24 * time.Hour, 72 * time.Hour, 7 * 24 * time.Hour} {
		streams := map[string]bool{}
		for id, task := range state.tasks {
			if task.IsDone && last.Sub(doneAt[id]) >= age {
				streams[id] = true
			}
		}
		n, b := 0, 0
		for _, r := range rows {
			if streams[r.stream] {
				n, b = n+1, b+len(r.data)
			}
		}
		t.Logf("  done %-12s %2d tasks, %5d events (%4.1f%%), %8d bytes of data (%4.1f%%)",
			"over "+age.String()+":", len(streams), n, pct(n, total), b, pct(b, totalBytes))
		if age == 0 {
			doneAtAll = streams
		}
	}
	var summaries []int
	for id := range doneAtAll {
		summaries = append(summaries, len(must(json.Marshal(summaryOf(state.tasks[id])))))
	}
	t.Logf("  a summary per archived task (the list's JSON): mean %d bytes, largest %d", mean(summaries), slices.Max(summaries))
	return doneAtAll
}

// bigText names the field holding each big text.
var bigText = map[string]string{
	string(events.AgentLearned):    "knowledge",
	string(events.PlanUpdated):     "plan",
	string(events.TurnEnded):       "report",
	string(events.ActionRequested): "command",
	string(events.TaskAssigned):    "instructions",
}

// reference is what an event carries instead of a text moved to the blob
// store: the field becomes {"sha256": "<64 hex>", "size": N}.
const reference = len(`{"sha256":"","size":000000}`) + 64

func benchSlim(t *testing.T, rows []row, archived map[string]bool) {
	t.Log("slimmer events, per type: texts, exact repeats, line diffs of successive versions:")
	texts := map[string][]string{}  // by type, in log order
	previous := map[string]string{} // by type and series, the last version
	diffs := map[string]int{}       // by type, bytes of line diffs
	diffSize := map[int64]int{}     // by position
	for _, r := range rows {
		field, ok := bigText[r.typ]
		if !ok {
			continue
		}
		text := textOf(r, field)
		texts[r.typ] = append(texts[r.typ], text)
		// Knowledge versions follow each other per agent; a plan per turn.
		series := r.typ + "/" + r.stream + "/" + r.corr
		if r.typ == string(events.AgentLearned) {
			var d map[string]any
			check(json.Unmarshal([]byte(r.data), &d))
			series = r.typ + "/" + d["name"].(string)
		}
		diffSize[r.position] = lineDiffSize(previous[series], text)
		diffs[r.typ] += diffSize[r.position]
		previous[series] = text
	}
	for _, typ := range []string{"agent.learned", "action.requested", "turn.ended", "plan.updated", "task.assigned"} {
		all := texts[typ]
		total, repeated := 0, 0
		seen := map[[32]byte]bool{}
		for _, text := range all {
			total += len(text)
			sum := sha256.Sum256([]byte(text))
			if seen[sum] {
				repeated += len(text)
			}
			seen[sum] = true
		}
		t.Logf("  %-17s %5d texts, %8d bytes, %8d (%4.1f%%) exact repeats, line diffs %8d (%4.1f%%)",
			typ, len(all), total, repeated, pct(repeated, total), diffs[typ], pct(diffs[typ], total))
	}

	t.Log("texts moved to the blob store above a threshold (each unique text once):")
	totalBytes := dataBytes(rows)
	for _, threshold := range []int{1024, 4096} {
		left, blobs, blobBytes, blocks, zipped := totalBytes, 0, 0, 0, 0
		byType := map[string]int{}
		seen := map[[32]byte]bool{}
		for _, r := range rows {
			field, ok := bigText[r.typ]
			if !ok {
				continue
			}
			text := textOf(r, field)
			if len(text) <= threshold {
				continue
			}
			left += reference - len(must(json.Marshal(text)))
			sum := sha256.Sum256([]byte(text))
			if !seen[sum] {
				seen[sum] = true
				blobs, blobBytes = blobs+1, blobBytes+len(text)
				blocks += (len(text) + 4095) / 4096
				zipped += len(gzipped([]byte(text)))
				byType[r.typ] += len(text)
			}
		}
		t.Logf("  over %4d bytes: data left in the log %8d (%4.1f%%); %4d blobs, %8d bytes, %8d on disk in 4 KB blocks, %8d gzipped one by one",
			threshold, left, pct(left, totalBytes), blobs, blobBytes, blocks*4096, zipped)
		t.Logf("    blob bytes by type: %v", byType)
	}

	t.Log("database files rebuilt from the copy, then VACUUM:")
	asIs := func(r row) (string, bool) { return r.data, true }
	moved := func(threshold int) func(r row) (string, bool) {
		return func(r row) (string, bool) {
			if field, ok := bigText[r.typ]; ok {
				if text := textOf(r, field); len(text) > threshold {
					return withText(r, field, map[string]any{"sha256": strings.Repeat("0", 64), "size": len(text)}), true
				}
			}
			return r.data, true
		}
	}
	// Knowledge and plans as line diffs from the version before (filler of
	// the diff's size: SQLite doesn't compress), the rest moved over 1 KB.
	diffed := func(r row) (string, bool) {
		if r.typ == string(events.AgentLearned) || r.typ == string(events.PlanUpdated) {
			return withText(r, bigText[r.typ], strings.Repeat("x", diffSize[r.position])), true
		}
		return moved(1024)(r)
	}
	// Each archived stream leaves one summary event, about the size of the
	// list's JSON for a task.
	archive := func(keep func(r row) (string, bool)) func(r row) (string, bool) {
		summarised := map[string]bool{}
		return func(r row) (string, bool) {
			if !archived[r.stream] {
				return keep(r)
			}
			if summarised[r.stream] {
				return "", false
			}
			summarised[r.stream] = true
			return strings.Repeat("x", 600), true
		}
	}
	for _, c := range []struct {
		label     string
		transform func(r row) (string, bool)
	}{{"as it is", asIs}, {"texts over 4 KB moved", moved(4096)}, {"texts over 1 KB moved", moved(1024)},
		{"knowledge, plans as diffs; rest over 1 KB moved", diffed},
		{"done tasks archived", archive(asIs)}, {"archived, texts over 1 KB moved", archive(moved(1024))},
		{"archived, diffs, rest over 1 KB moved", archive(diffed)}} {
		t.Logf("  %-48s %9d bytes", c.label+":", rebuiltSize(t, rows, c.transform))
	}
}

func textOf(r row, field string) string {
	var d map[string]any
	check(json.Unmarshal([]byte(r.data), &d))
	text, _ := d[field].(string)
	return text
}

// lineDiffSize is the size of an ed-style diff from one version to the
// next: every added line, plus a 12-byte range per changed hunk. Deleted
// lines cost only their range. Lines are matched by a longest common
// subsequence.
func lineDiffSize(from, to string) int {
	if from == "" {
		return len(to)
	}
	a, b := strings.SplitAfter(from, "\n"), strings.SplitAfter(to, "\n")
	// Trim the common head and tail, then run the LCS on what is left.
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && a[len(a)-1] == b[len(b)-1] {
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	size, inHunk := 0, false
	for i, j := 0, 0; i < len(a) || j < len(b); {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			i, j, inHunk = i+1, j+1, false
			continue
		case j < len(b) && (i == len(a) || lcs[i][j+1] >= lcs[i+1][j]):
			size += len(b[j])
			j++
		default:
			i++
		}
		if !inHunk {
			size, inHunk = size+12, true
		}
	}
	return size
}

// rebuiltSize writes the rows into a new database, each as transform
// gives its data or leaves it out, then VACUUMs it and returns its size.
func rebuiltSize(t *testing.T, rows []row, transform func(r row) (data string, keep bool)) int64 {
	path := filepath.Join(t.TempDir(), "rebuilt.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE events (position INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL,
		stream TEXT NOT NULL, actor_kind TEXT NOT NULL, actor_id TEXT NOT NULL, type TEXT NOT NULL,
		correlation TEXT NOT NULL DEFAULT '', data TEXT NOT NULL DEFAULT '{}');
		CREATE INDEX events_by_stream ON events (stream, position);`)
	check(err)
	tx := must(db.Begin())
	insert := must(tx.Prepare(`INSERT INTO events VALUES (?, ?, ?, ?, ?, ?, ?, ?)`))
	for _, r := range rows {
		if data, keep := transform(r); keep {
			must(insert.Exec(r.position, r.at, r.stream, r.kind, r.id, r.typ, r.corr, data))
		}
	}
	check(tx.Commit())
	must(db.Exec(`VACUUM`))
	return must(os.Stat(path)).Size()
}

// withText is the row's data with one field replaced.
func withText(r row, field string, value any) string {
	var d map[string]any
	check(json.Unmarshal([]byte(r.data), &d))
	d[field] = value
	return string(must(json.Marshal(d)))
}

func gobEncode(v any) []byte {
	var b bytes.Buffer
	check(gob.NewEncoder(&b).Encode(v))
	return b.Bytes()
}

func gzipped(b []byte) []byte {
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	must(w.Write(b))
	check(w.Close())
	return out.Bytes()
}

func gunzipped(b []byte) []byte {
	r := must(gzip.NewReader(bytes.NewReader(b)))
	var out bytes.Buffer
	must(out.ReadFrom(r))
	return out.Bytes()
}

func minOf3(run func()) time.Duration {
	var best time.Duration
	for i := range 3 {
		start := time.Now()
		run()
		if took := time.Since(start); i == 0 || took < best {
			best = took
		}
	}
	return best
}

func mean(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sum := 0
	for _, v := range values {
		sum += v
	}
	return sum / len(values)
}

func pct(part, whole int) float64 { return 100 * float64(part) / float64(whole) }

func must[T any](v T, err error) T {
	check(err)
	return v
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
