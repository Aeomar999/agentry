package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/engines"
	"gitlab.com/dev.chempa/agentry/internal/platform/hostaddrs"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// Started from inside a Claude Code session, serve, open and the worker
// drop that session's markers and say which, by name.
func TestStartingDropsAParentSessionAndSaysWhat(t *testing.T) {
	for _, kv := range os.Environ() { // whoever runs the tests may be in a session
		if name, _, _ := strings.Cut(kv, "="); engines.IsParentSessionVar(name) {
			t.Setenv(name, "") // restored after the test
			os.Unsetenv(name)
		}
	}
	var quiet strings.Builder
	if dropParentSession(&quiet); quiet.Len() != 0 {
		t.Errorf("nothing to remove, but it said %q", quiet.String())
	}

	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "secret-value")
	t.Setenv("CLAUDE_CONFIG_DIR", "/claude-config")
	var said strings.Builder
	dropParentSession(&said)
	want := "agentry: removed variables inherited from a Claude Code or Codex session: CLAUDECODE, CLAUDE_CODE_CHILD_SESSION\n"
	if said.String() != want {
		t.Errorf("said %q, want %q", said.String(), want)
	}
	if _, set := os.LookupEnv("CLAUDE_CODE_CHILD_SESSION"); set || os.Getenv("CLAUDE_CONFIG_DIR") == "" {
		t.Error("it should remove the markers and keep the user's configuration")
	}
}

// Hooks and the desk call the server on loopback when it listens on every
// address, as it does in a container.
func TestLocalURL(t *testing.T) {
	for addr, want := range map[string]string{
		"0.0.0.0:7800":   "http://127.0.0.1:7800",
		"127.0.0.1:7801": "http://127.0.0.1:7801",
		"[::]:7802":      "http://127.0.0.1:7802",
	} {
		tcp, _ := net.ResolveTCPAddr("tcp", addr)
		if got := localURL(tcp, "http"); got != want {
			t.Errorf("localURL(%s) = %s, want %s", addr, got, want)
		}
	}
}

// A page on this machine opens the server at localhost, unless it listens on
// one address that isn't loopback.
func TestLocalhostURL(t *testing.T) {
	for addr, want := range map[string]string{
		"0.0.0.0:7800":     "http://localhost:7800",
		"127.0.0.1:7801":   "http://localhost:7801",
		"[::]:7802":        "http://localhost:7802",
		"192.168.1.5:7803": "",
	} {
		tcp, _ := net.ResolveTCPAddr("tcp", addr)
		if got := localhostURL(tcp, "http"); got != want {
			t.Errorf("localhostURL(%s) = %s, want %s", addr, got, want)
		}
	}
}

// Listening on every address, serve prints a link for each of this machine's
// own addresses other machines reach; listening on one, it prints none.
func TestNetworkURLs(t *testing.T) {
	one, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:7800")
	if urls := networkURLs(one, "http"); urls != nil {
		t.Errorf("on one address: %v", urls)
	}
	every, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:7800")
	for _, url := range networkURLs(every, "http") {
		if !strings.HasPrefix(url, "http://") || !strings.HasSuffix(url, ":7800") || strings.Contains(url, "127.0.0.1") {
			t.Errorf("a network URL: %s", url)
		}
	}
	var want []string
	for _, ip := range hostaddrs.Network() {
		want = append(want, "http://"+ip.String()+":7800")
	}
	if got := networkURLs(every, "http"); !slices.Equal(got, want) {
		t.Errorf("networkURLs = %v, want one for each network address: %v", got, want)
	}
}

// A log with streams of an extension this build doesn't have says so once per
// prefix at start, not once per event; an unknown type alone says nothing.
func TestStartingSaysOncePerSkippedPrefix(t *testing.T) {
	var said strings.Builder
	reportSkipped(&said, []current.Unknown{
		{Prefix: "idea-", Streams: 12, Events: 31, Last: 48211},
		{Prefix: "note-", Streams: 1, Events: 2, Last: 48100},
		{Type: "git.pr_opened", Events: 4, Last: 48190},
	})
	want := "agentry: skipped 31 events in 12 idea- streams, from a feature this build doesn't have; the log keeps them\n" +
		"agentry: skipped 2 events in 1 note- streams, from a feature this build doesn't have; the log keeps them\n"
	if said.String() != want {
		t.Errorf("said %q, want %q", said.String(), want)
	}
	said.Reset()
	if reportSkipped(&said, []current.Unknown{}); said.Len() != 0 {
		t.Errorf("nothing skipped, but it said %q", said.String())
	}
}

// servedOnce starts serve on dataDir, waits until it is running and stops
// it cleanly. It returns what it logged.
func servedOnce(t *testing.T, dataDir string) string {
	t.Helper()
	return servedWith(t, serveOptions{dataDir: dataDir})
}

// servedWith is servedOnce with more options, listening on any free port
// of loopback.
func servedWith(t *testing.T, o serveOptions) string {
	t.Helper()
	return startServing(t, o)()
}

// startServing starts serve as servedWith does and waits until it is
// running. Its stop stops it cleanly and returns what it logged; the test's
// cleanup calls it too, so a test that fails first still stops the server.
func startServing(t *testing.T, o serveOptions) (stop func() string) {
	t.Helper()
	var logged, out syncBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	o.address, o.repo = "127.0.0.1:0", t.TempDir() // made before the cleanup below, so removed after it stops serve
	go func() { done <- serve(ctx, o, &out) }()
	stop = sync.OnceValue(func() string {
		t.Helper()
		defer slog.SetDefault(previous)
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("serve: %v", err)
		}
		return logged.String()
	})
	t.Cleanup(func() { stop() })
	for deadline := time.Now().Add(10 * time.Second); !strings.Contains(out.String(), "Agentry is running"); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Errorf("serve didn't start in time; it printed:\n%s", out.String())
			stop()
			t.FailNow()
		}
	}
	return stop
}

// serve keeps the state's snapshot in the data folder, readable by its
// owner alone: it removes the temporary files of writes that never
// finished, writes a snapshot at a clean shutdown, and the next start goes
// on from it with no fallback.
func TestServeKeepsTheSnapshotAcrossARestart(t *testing.T) {
	dataDir := t.TempDir()
	leftover := filepath.Join(dataDir, snapshotFile+".tmp-123")
	if err := os.WriteFile(leftover, []byte("half a snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	if said := servedOnce(t, dataDir); !strings.Contains(said, "there is no snapshot") {
		t.Errorf("the first start should say there is no snapshot; it logged:\n%s", said)
	}
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Errorf("the unfinished write is still there: %v", err)
	}
	info, err := os.Stat(filepath.Join(dataDir, snapshotFile))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the snapshot's mode is %v, want 0600", mode)
	}
	if said := servedOnce(t, dataDir); strings.Contains(said, "replayed") {
		t.Errorf("the second start didn't go on from the snapshot; it logged:\n%s", said)
	}
}

// A binary serve can run but not read gets a build no snapshot was written
// under, not even by the start before: every start replays the whole log.
func TestAnUnhashedBuildNeverUsesASnapshot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "agentry")
	if _, err := buildOf(missing); err == nil {
		t.Fatal("buildOf hashed a binary that isn't there")
	}
	logPath := filepath.Join(t.TempDir(), "agentry.db")
	snapshot := filepath.Join(filepath.Dir(logPath), snapshotFile)
	startedAs := func(build string) (resumed int64) {
		t.Helper()
		state := current.Restore(snapshot, build, storetest.WithExtensions(), extensions.StreamsOf(installedExtensions()...)...)
		log, err := store.Open(logPath, storetest.WithExtensions(), state, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer log.Close()
		writer := current.NewSnapshotWriter(snapshot, build, state, log)
		writer.Start()
		defer writer.Close()
		if err := seedAgents(log, state); err != nil {
			t.Fatal(err)
		}
		return state.Resumed()
	}
	unhashed := buildOrUnhashed(missing)
	startedAs(unhashed)
	if startedAs(unhashed) == 0 {
		t.Fatal("the same build didn't go on from its snapshot, so this test shows nothing")
	}
	if startedAs(buildOrUnhashed(missing)) != 0 {
		t.Error("an unhashed start went on from the snapshot of the start before")
	}
}

// -archive-after is a duration, 30 days by default; 0 turns archiving off,
// and a negative one is refused. What open passes on parses too.
func TestServeParsesArchiveAfter(t *testing.T) {
	for args, want := range map[string]time.Duration{"": 720 * time.Hour, "-archive-after=48h": 48 * time.Hour, "-archive-after=48h0m0s": 48 * time.Hour,
		"-archive-after=0": 0, "-archive-after=0s": 0} {
		var said strings.Builder
		options, ok := parseServe(strings.Fields(args), &said)
		if !ok || options.archiveAfter != want {
			t.Errorf("%q: archive after %v, ok %v, want %v; it said %q", args, options.archiveAfter, ok, want, said.String())
		}
	}
	var said strings.Builder
	if code := runServe([]string{"-archive-after=-1h"}, nil, &said, &said); code != 2 || !strings.Contains(said.String(), "-archive-after is 0 or above") {
		t.Errorf("a negative age: exit %d, said %q; want 2 and why", code, said.String())
	}
}

// serve archives a task done for -archive-after, at start, and logs that it
// does; with 0 it archives nothing, however long a task has been done, and
// logs that archiving is off.
func TestServeArchivesOnlyWithArchiveAfter(t *testing.T) {
	for _, after := range []time.Duration{0, time.Nanosecond} {
		dataDir := t.TempDir()
		withADoneTask(t, dataDir, "task-1")
		logged := servedWith(t, serveOptions{dataDir: dataDir, archiveAfter: after})
		if archived := isArchived(t, dataDir, "task-1"); archived != (after > 0) {
			t.Errorf("with -archive-after=%v, task-1 archived %v", after, archived)
		}
		if said := strings.Contains(logged, "archiving tasks done for "+after.String()); said != (after > 0) {
			t.Errorf("with -archive-after=%v, logged that it archives: %v; log:\n%s", after, said, logged)
		}
		if off := strings.Contains(logged, "archiving is off (-archive-after=0)"); off != (after == 0) {
			t.Errorf("with -archive-after=%v, logged that archiving is off: %v; log:\n%s", after, off, logged)
		}
	}
}

// A second serve on a data folder already served, at the same busy address,
// fails to bind before it touches the folder: it archives nothing, seeds
// nothing and neither writes nor removes a snapshot, so the first server's
// state stays true.
func TestServeOnABusyAddressLeavesTheDataFolderAlone(t *testing.T) {
	dataDir := t.TempDir()
	withADoneTask(t, dataDir, "task-1")
	servedOnce(t, dataDir) // writes a snapshot
	leftover := filepath.Join(dataDir, snapshotFile+".tmp-123")
	if err := os.WriteFile(leftover, []byte("the first server's write under way"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "files", "task-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	before := filesIn(t, dataDir)
	if _, ok := before[snapshotFile]; !ok {
		t.Fatal("no snapshot to keep, so this test shows nothing")
	}

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	o := serveOptions{address: busy.Addr().String(), repo: t.TempDir(), dataDir: dataDir, archiveAfter: time.Nanosecond}
	var bind *net.OpError
	if err := serve(context.Background(), o, io.Discard); !errors.As(err, &bind) || bind.Op != "listen" {
		t.Fatalf("serve on a busy address returned %v, want its listen error", err)
	}
	unchangedSince(t, dataDir, before, "failing to bind")
	if isArchived(t, dataDir, "task-1") {
		t.Error("serve archived task-1 before failing to bind")
	}
}

// A second serve on a data folder another serve is using, on another port,
// fails before it touches the folder, and once the first stops the folder
// serves again.
func TestASecondServeOnTheSameFolderLeavesItAlone(t *testing.T) {
	dataDir := t.TempDir()
	withADoneTask(t, dataDir, "task-1")
	stopFirst := startServing(t, serveOptions{dataDir: dataDir})
	// What the second serve would clear or make, were it let in.
	leftover := filepath.Join(dataDir, snapshotFile+".tmp-123")
	if err := os.WriteFile(leftover, []byte("the first server's write under way"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "files", "task-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	before := filesIn(t, dataDir)

	o := serveOptions{address: "127.0.0.1:0", repo: t.TempDir(), dataDir: dataDir, archiveAfter: time.Nanosecond}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // were it let in, it would serve until stopped
	defer cancel()
	err := serve(ctx, o, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "another agentry serve is using "+dataDir) {
		t.Errorf("a second serve on the folder returned %v, want that another serve is using it", err)
	}
	unchangedSince(t, dataDir, before, "finding the folder in use")
	stopFirst()
	if isArchived(t, dataDir, "task-1") {
		t.Error("the second serve archived task-1")
	}
	servedOnce(t, dataDir) // the first serve let the folder go
}

// unchangedSince fails t for each file under dataDir that serve made,
// changed or removed since before was taken, ahead of what it was doing.
func unchangedSince(t *testing.T, dataDir string, before map[string]string, doing string) {
	t.Helper()
	after := filesIn(t, dataDir)
	for name := range maps.Keys(before) {
		if contents, ok := after[name]; !ok || contents != before[name] {
			t.Errorf("serve changed or removed %s before %s", name, doing)
		}
	}
	for name := range maps.Keys(after) {
		if _, ok := before[name]; !ok {
			t.Errorf("serve made %s before %s", name, doing)
		}
	}
}

// filesIn is every file and folder under dir, by path, with its contents.
func filesIn(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name, _ := filepath.Rel(dir, path)
		if entry.IsDir() {
			files[name+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(path)
		files[name] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// withADoneTask starts the log in dataDir with the default agents and a
// task of yours, done.
func withADoneTask(t *testing.T, dataDir, id string) {
	t.Helper()
	log, state := openLog(t, dataDir)
	defer log.Close()
	if err := seedAgents(log, state); err != nil {
		t.Fatal(err)
	}
	you := events.Actor{Kind: events.Human, ID: "you"}
	for _, p := range []events.Proposal{
		{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "A task", "repo": "/r", "coordinator": "tech-lead"}},
		{Stream: id, Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Done.", "moved": true}},
	} {
		if _, rejection, err := log.Propose(p); rejection != nil || err != nil {
			t.Fatalf("%s: %v, %v", p.Type, rejection, err)
		}
	}
}

// serve empties a WAL that a crash left holding old text as it starts: an
// erasure or a redaction a reader kept from emptying it leaves the old
// rows there, and only an archiving round would empty it otherwise. The
// crash is a copy of the log and its WAL taken while it is open, and the
// text is in a done task's summary, in overflow pages, which the writes of
// the start don't put back in the WAL.
func TestServeEmptiesTheWALACrashLeft(t *testing.T) {
	const left = "left-in-the-wal-5e1d"
	running := t.TempDir()
	log, state := openLog(t, running)
	defer log.Close()
	if err := seedAgents(log, state); err != nil {
		t.Fatal(err)
	}
	you := events.Actor{Kind: events.Human, ID: "you"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "A task", "repo": "/r", "coordinator": "tech-lead"}},
		{Stream: "task-1", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": strings.Repeat("Done. ", 2000) + left, "moved": true}},
		{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Another", "repo": "/r", "coordinator": "tech-lead"}},
		{Stream: "task-2", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Done.", "moved": true}},
	} {
		if _, rejection, err := log.Propose(p); rejection != nil || err != nil {
			t.Fatalf("%s: %v, %v", p.Type, rejection, err)
		}
	}
	dataDir := t.TempDir()
	for _, name := range []string{"agentry.db", "agentry.db-wal"} {
		content, err := os.ReadFile(filepath.Join(running, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dataDir, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	wal := filepath.Join(dataDir, "agentry.db-wal")
	if !fileHolds(t, wal, left) {
		t.Fatal("the WAL the crash left doesn't hold the text, so this test shows nothing")
	}
	stop := startServing(t, serveOptions{dataDir: dataDir})
	if fileHolds(t, wal, left) {
		t.Error("serve started with the old text still in the WAL")
	}
	stop()
}

// fileHolds says whether the file at path holds text; one that isn't there
// holds nothing.
func fileHolds(t *testing.T, path, text string) bool {
	t.Helper()
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(content), text)
}

// isArchived replays the log in dataDir and says whether it holds the task
// as its summary alone.
func isArchived(t *testing.T, dataDir, id string) bool {
	t.Helper()
	log, state := openLog(t, dataDir)
	defer log.Close()
	task, err := state.Task(id)
	if task == nil && !errors.Is(err, current.ErrArchived) {
		t.Fatalf("%s is neither in the state nor archived: %v", id, err)
	}
	return errors.Is(err, current.ErrArchived)
}

// openLog opens the log in dataDir as serve does, replayed from the start.
func openLog(t *testing.T, dataDir string) (*store.Store, *current.State) {
	t.Helper()
	catalogue := storetest.WithExtensions()
	state := storetest.NewState(catalogue)
	log, err := store.Open(filepath.Join(dataDir, "agentry.db"), catalogue, state, func(p events.Proposal) *events.Rejection {
		return rules.Check(p, state, catalogue, installedExtensions()...)
	})
	if err != nil {
		t.Fatal(err)
	}
	return log, state
}
