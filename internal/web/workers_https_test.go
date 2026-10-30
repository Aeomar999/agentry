package web

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/pinned"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/terminal"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
	"gitlab.com/dev.chempa/agentry/internal/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/workers"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// Served over https with Agentry's own CA, a worker that pins the CA links,
// takes work over git, shows its terminal and gives the work back; one that
// trusts only the system's store can't link.
func TestAWorkerWorksOverHTTPSPinnedToTheCA(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	here := &machines.Local{Label: workers.Local, WorktreeRoot: t.TempDir()}
	fleet := machines.NewFleet(here)
	ca, err := tlsca.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(NewServer(s, state, Config{UIToken: token, Fleet: fleet, Workers: workerlink.NewServer(s, fleet)},
		http.NotFoundHandler(), nil).Handler())
	issuer := tlsca.NewIssuer(ca, func() []string { return nil }, time.Now)
	server.Listener = tls.NewListener(server.Listener, &tls.Config{GetCertificate: issuer.GetCertificate, NextProtos: []string{"h2", "http/1.1"}})
	server.Start()
	t.Cleanup(server.Close)
	url := "https://" + server.Listener.Addr().String()
	client := pinned.Client(ca.PEM(), 10*time.Second)

	request, _ := http.NewRequest("POST", url+"/api/workers/invite", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var invite struct{ Code string }
	json.NewDecoder(response.Body).Decode(&invite)
	response.Body.Close()
	response, err = client.Post(url+"/api/worker/join", "application/json", strings.NewReader(`{"code":"`+invite.Code+`","name":"laptop"}`))
	if err != nil {
		t.Fatal(err)
	}
	var credential struct{ Credential string }
	json.NewDecoder(response.Body).Decode(&credential)
	response.Body.Close()

	// Without the CA, the link can't come up.
	refused := make(chan error, 1)
	unpinned := &workerlink.Worker{Server: url, Credential: credential.Credential, Log: t.Logf,
		OnLink: func(up bool, err error) {
			if !up {
				select {
				case refused <- err:
				default:
				}
			}
		}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	unpinnedCtx, stopUnpinned := context.WithCancel(ctx)
	go unpinned.Run(unpinnedCtx)
	select {
	case err := <-refused:
		if err == nil || !strings.Contains(err.Error(), "certificate") {
			t.Errorf("the unpinned worker's link went down with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the unpinned worker neither linked nor failed")
	}
	stopUnpinned()

	// With it, everything goes over https.
	fakeClaudeOnPath(t)
	worker, err := workerlink.Setup{Server: url, Name: "laptop", Credential: credential.Credential, CA: string(ca.PEM()),
		Dir: t.TempDir(), HookCommand: "agentry hook"}.Worker() // as `agentry worker run` makes it
	if err != nil {
		t.Fatal(err)
	}
	worker.Log = t.Logf
	go worker.Run(ctx)
	laptop := waitFor(t, "the pinned worker to come online", func() (machines.Machine, bool) { return fleet.Get("laptop") })

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
	if err := here.HandOff(ctx, w, "Agentry: moving"); err != nil {
		t.Fatal(err)
	}
	prepare := machines.WorktreeRequest{Task: "task-1", Repo: hub, Base: w.Base, RepoKey: worktrees.RepoKey(hub)}
	// git with no CA file trusts the system's store, so it can't fetch.
	noCAFile := *worker.Machine
	noCAFile.WorktreeRoot = t.TempDir()
	noCAFile.Hub = &machines.Hub{URL: workerlink.HubURL(url), Credential: credential.Credential, MirrorRoot: t.TempDir(), HTTP: worker.HTTP}
	if _, err := noCAFile.Prepare(ctx, prepare); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("git without the CA file: %v, want a certificate error", err)
	}
	there, err := laptop.Prepare(ctx, prepare)
	if err != nil {
		t.Fatalf("git on the worker, over https: %v", err)
	}
	s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Worker, ID: "laptop"}, Type: events.WorktreeMoved,
		Data: map[string]any{"worker": "laptop", "path": there.Path, "port": there.Port}})

	// The terminal comes over a second websocket, dialled the same way.
	turn := machines.Turn{Task: "task-1", ID: "turn-1", Worktree: there.Path, Engine: "claude", Brief: "b", BriefFile: ".agentry/turn-1.md",
		Prompt: "Read .agentry/turn-1.md and do what it says.", Env: map[string]string{"AGENTRY_TASK": "task-1"}}
	defer sessions.Stop(sessions.SocketFor("task-1"))
	if err := laptop.PrepareTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if err := laptop.StartTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the worker to say the agent runs", func() (bool, bool) { return true, laptop.IsRunning("task-1") })
	if screen := watchTerminal(t, ctx, terminal.NewHub(), laptop); !strings.Contains(screen, "fake claude is working") {
		t.Errorf("the worker's terminal: %q", screen)
	}

	// The work comes back as a bundle, posted over https.
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
}
