package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/platform/sessions"
	"gitlab.com/dev.chempa/agentry/internal/platform/sessions/sessionstest"
	"gitlab.com/dev.chempa/agentry/internal/platform/terminal"
	"gitlab.com/dev.chempa/agentry/internal/platform/workspace"
	"gitlab.com/dev.chempa/agentry/internal/platform/worktrees"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// A machine joins with the code from the page and runs a turn: the work
// moves to it over git, its agent's terminal shows on the server, the page
// peeks at its files, and the work comes back with what it did.
func TestAWorkerJoinsAndTakesWork(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	here := &machines.Local{Label: workers.Local, WorktreeRoot: t.TempDir()}
	fleet := machines.NewFleet(here)
	binary := filepath.Join(t.TempDir(), "agentry")
	os.WriteFile(binary, []byte("the agentry binary"), 0o755)
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, Fleet: fleet, Workers: workerlink.NewServer(s, fleet),
		Binary: binary}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)

	// The page makes a code; the command fetches the script and agentry with it.
	_, body := call(t, server, "POST", "/api/workers/invite", "")
	var invite struct{ Code, Command string }
	json.Unmarshal([]byte(body), &invite)
	if !strings.Contains(invite.Command, server.URL+"/worker/install.sh?code="+invite.Code) || !strings.HasSuffix(invite.Command, "| sh") {
		t.Fatalf("join command: %q", invite.Command)
	}
	if code, script := get(t, server.URL+"/worker/install.sh?code="+invite.Code); code != 200 || !strings.Contains(script, "code='"+invite.Code+"'") {
		t.Errorf("install script: %d\n%s", code, script)
	}
	if code, _ := get(t, server.URL+"/worker/install.sh?code=wrong"); code != http.StatusUnauthorized {
		t.Errorf("a wrong code got the script: %d", code)
	}
	if code, got := get(t, server.URL+"/worker/agentry?code="+invite.Code+"&os="+runtime.GOOS+"&arch="+runtime.GOARCH); code != 200 || got != "the agentry binary" {
		t.Errorf("download: %d %q", code, got)
	}

	// The machine joins, and its host's name is taken as its own.
	joined := post(t, server.URL+"/api/worker/join", `{"code":"`+invite.Code+`","machine":{"host":"Laptop.lan","engines":["claude 2.1.301"]}}`)
	var credential struct{ Name, Credential string }
	json.Unmarshal([]byte(joined), &credential)
	if credential.Name != "laptop" || credential.Credential == "" {
		t.Fatalf("join: %s", joined)
	}
	if again := post(t, server.URL+"/api/worker/join", `{"code":"`+invite.Code+`"}`); !strings.Contains(again, "unknown, used or expired") {
		t.Errorf("a used code joined again: %s", again)
	}

	// It connects and runs.
	fakeClaudeOnPath(t)
	root := t.TempDir()
	worker := &workerlink.Worker{Server: server.URL, Credential: credential.Credential,
		About: workers.Machine{Host: "Laptop.lan", Engines: []string{"claude 2.1.301"}}, Log: t.Logf,
		Machine: &machines.Local{Label: "laptop", ServerURL: server.URL, HookCommand: "agentry hook", WorktreeRoot: filepath.Join(root, "worktrees"),
			Hub: &machines.Hub{URL: workerlink.HubURL(server.URL), Credential: credential.Credential, MirrorRoot: filepath.Join(root, "repos")}}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go worker.Run(ctx)
	laptop := waitFor(t, "the worker to come online", func() (machines.Machine, bool) { return fleet.Get("laptop") })
	_, list := call(t, server, "GET", "/api/workers", "")
	if !strings.Contains(list, `"name":"laptop"`) || !strings.Contains(list, `"online":true`) {
		t.Errorf("the Workers page: %s", list)
	}
	// Its details are read from it, over its link.
	_, shown := call(t, server, "GET", "/api/workers/laptop", "")
	var detail workerDetail
	json.Unmarshal([]byte(shown), &detail)
	if detail.About == nil || detail.About.Host == "" || detail.About.CPUs == 0 || len(detail.History) == 0 || detail.History[0].What != "online" {
		t.Errorf("the worker's details: %s", shown)
	}

	// A task's work starts here and moves to the worker, over git.
	hub := gitRepo(t)
	w, err := worktrees.Create(hub, here.WorktreeRoot, "task-1", 7901)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: httpapi.You, Type: events.TaskCreated, Data: map[string]any{"title": "t", "repo": hub, "coordinator": "tech-lead"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Worker, ID: "local"}, Type: events.WorktreeReady,
			Data: map[string]any{"repo": hub, "worker": "local", "path": w.Path, "branch": w.Branch, "base": w.Base, "port": 7901}},
	} {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatal(rejection, err)
		}
	}
	os.WriteFile(filepath.Join(w.Path, "notes.md"), []byte("from here\n"), 0o644)
	if err := here.HandOff(ctx, w, "Agentry: moving"); err != nil {
		t.Fatal(err)
	}
	there, err := laptop.Prepare(ctx, machines.WorktreeRequest{Task: "task-1", Repo: hub, Base: w.Base, RepoKey: worktrees.RepoKey(hub)})
	if err != nil {
		t.Fatalf("preparing on the worker: %v", err)
	}
	s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Worker, ID: "laptop"}, Type: events.WorktreeMoved,
		Data: map[string]any{"worker": "laptop", "path": there.Path, "port": there.Port}})

	// The page peeks at its files there: notes.md, committed when the work
	// left, is a plain file, and the commit that holds it is in the history.
	ws := workspace.Workspace{Dir: there.Path, Base: w.Base}
	files, err := laptop.Files(ctx, ws)
	if err != nil || !slices.ContainsFunc(files, func(f workspace.File) bool { return f.Path == "notes.md" && f.Change == workspace.Unchanged }) {
		t.Errorf("files on the worker: %+v %v", files, err)
	}
	commits, err := laptop.Commits(ctx, ws)
	if err != nil || len(commits) == 0 || !slices.ContainsFunc(commits[0].Files, func(f workspace.File) bool { return f.Path == "notes.md" && f.Change == workspace.Added }) {
		t.Errorf("commits on the worker: %+v %v", commits, err)
	}

	// A turn runs there, going on with its session, and its terminal shows here.
	turn := machines.Turn{Task: "task-1", ID: "turn-1", Worktree: there.Path, Engine: "claude", Brief: "b", BriefFile: ".agentry/turn-1.md",
		Prompt: "Read .agentry/turn-1.md and do what it says.", Resume: "s-1", Env: map[string]string{"AGENTRY_TASK": "task-1"}}
	defer sessions.Stop(sessions.SocketFor("task-1"))
	if err := laptop.PrepareTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if err := laptop.StartTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the worker to say the agent runs", func() (bool, bool) { return true, laptop.IsRunning("task-1") })
	screen := watchTerminal(t, ctx, terminal.NewHub(), laptop)
	if !strings.Contains(screen, "fake claude is working --resume s-1 ") {
		t.Errorf("the worker's terminal, resuming the session: %q", screen)
	}

	// The work comes back, with what the agent left.
	os.WriteFile(filepath.Join(there.Path, "notes.md"), []byte("from the laptop\n"), 0o644)
	if err := laptop.HandOff(ctx, there, "Agentry: moving back"); err != nil {
		t.Fatal(err)
	}
	back, err := worktrees.Create(hub, here.WorktreeRoot, "task-1", 7901)
	if err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(filepath.Join(back.Path, "notes.md")); string(content) != "from the laptop\n" {
		t.Errorf("the work from the worker: %q", content)
	}

	// Removed, its credential stops working and its link drops.
	if code, body := call(t, server, "DELETE", "/api/workers/laptop", ""); code != 200 {
		t.Fatalf("remove: %d %s", code, body)
	}
	request, _ := http.NewRequest("GET", server.URL+workerlink.ConnectPath, nil)
	request.Header.Set("Authorization", "Bearer "+credential.Credential)
	if response, err := http.DefaultClient.Do(request); err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Errorf("a removed worker's credential: %v %v", response.StatusCode, err)
	}
}

// watchTerminal connects a viewer to the task's session through the hub
// and returns what it shows.
func watchTerminal(t *testing.T, ctx context.Context, hub *terminal.Hub, m machines.Machine) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /tasks/{task}/terminal", hub.Handler(m.Attach))
	view := httptest.NewServer(mux)
	defer view.Close()
	ws, _, err := websocket.Dial(ctx, view.URL+"/tasks/task-1/terminal", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	var screen strings.Builder
	read, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for !strings.Contains(screen.String(), "fake claude is working") {
		kind, data, err := ws.Read(read)
		if err != nil {
			break
		}
		if kind == websocket.MessageBinary {
			screen.Write(data)
		}
	}
	return screen.String()
}

// fakeClaudeOnPath puts a `claude` on PATH that says it works, then waits,
// in sessions of the test's own.
func fakeClaudeOnPath(t *testing.T) {
	t.Helper()
	sessionstest.Isolate(t)
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n[ \"$1\" = --version ] && { echo '2.1.301 (Claude Code)'; exit 0; }\necho fake claude is working \"$@\"\n"+sessionstest.Wait()+"\n"), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
}

func gitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "dev"}, {"commit", "-q", "--allow-empty", "-m", "first"}} {
		command := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@e"}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, output)
		}
	}
	return repo
}

func waitFor[T any](t *testing.T, what string, check func() (T, bool)) T {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if value, ok := check(); ok {
			return value
		}
	}
	t.Fatalf("timed out waiting for %s", what)
	var zero T
	return zero
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(body)
}

func post(t *testing.T, url, body string) string {
	t.Helper()
	response, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reply, _ := io.ReadAll(response.Body)
	return string(reply)
}
