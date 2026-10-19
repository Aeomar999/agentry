package workerlink

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/pinned"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/terminal"
	"gitlab.com/dev.chempa/agentry/internal/workers"
	"gitlab.com/dev.chempa/agentry/internal/workspace"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// Worker is the worker's end of the link: `agentry worker`.
type Worker struct {
	Server     string // the server's address, as https://agentry.example.com
	Credential string
	// HTTP calls the server, and dials its websockets: pinned to its CA when
	// it is served over https with Agentry's own. Nil is the default client.
	HTTP       *http.Client
	Machine    *machines.Local // this machine, which does the work
	About      workers.Machine // what it says about itself
	Log        func(format string, args ...any)
	SelfUpdate bool // replace this agentry with the server's when they differ (see Run)
	// OnLink, if set, hears each time the link comes up (err nil) or goes down.
	OnLink func(up bool, err error)
}

func (w *Worker) linked(up bool, err error) {
	if w.OnLink != nil {
		w.OnLink(up, err)
	}
}

// ErrRemoved is a credential the server no longer takes: the worker was
// removed on the page, and has to join again.
var ErrRemoved = errors.New("the server no longer knows this worker; join it again")

// Run keeps the link up until ctx ends: it dials, says hello, answers, and
// dials again when the link drops, waiting a little longer each time.
// With SelfUpdate, before each dial it updates agentry to the server's if
// they differ, and then returns ErrUpdated, for the process to restart into
// the new one.
func (w *Worker) Run(ctx context.Context) error {
	wait := time.Second
	toldUntrusted := false
	for {
		if w.SelfUpdate {
			if updated, err := w.Update(ctx); updated {
				return ErrUpdated
			} else if err != nil {
				w.Log("couldn't check for a newer agentry: %v", err)
			}
		}
		connected, err := w.connect(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, ErrRemoved) {
			return err
		}
		w.linked(false, err)
		if connected {
			wait = time.Second // it was up: this is a new problem, so dial back at once
			toldUntrusted = false
		}
		if pinned.Untrusted(err) && !toldUntrusted {
			w.Log("the server's certificate isn't from the CA kept when this worker joined: was its CA regenerated? " +
				"Join again with `agentry worker join -ca-fingerprint <F> " + w.Server + " <code>`, F from `agentry ca` on the server and the code from its Workers page")
			toldUntrusted = true
		}
		w.Log("link down (%v); dialing again in %s", err, wait)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		wait = min(2*wait, 30*time.Second)
	}
}

// connect runs one link until it drops, and says whether it was up at all.
func (w *Worker) connect(ctx context.Context) (bool, error) {
	ws, response, err := websocket.Dial(ctx, w.Server+ConnectPath, w.dialOptions())
	if response != nil && response.StatusCode == http.StatusUnauthorized {
		return false, ErrRemoved
	}
	if err != nil {
		return false, err
	}
	defer ws.CloseNow()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	link := newConn(ws, w.handle, nil)
	if err := link.notify(ctx, noticeHello, w.About); err != nil {
		return false, err
	}
	w.Log("connected to %s", w.Server)
	w.linked(true, nil)
	go w.sendStatus(ctx, link)
	return true, link.run(ctx)
}

func (w *Worker) header() http.Header {
	return http.Header{"Authorization": {"Bearer " + w.Credential}}
}

func (w *Worker) client() *http.Client {
	if w.HTTP == nil {
		return http.DefaultClient
	}
	return w.HTTP
}

func (w *Worker) dialOptions() *websocket.DialOptions {
	return &websocket.DialOptions{HTTPHeader: w.header(), HTTPClient: w.client()}
}

// sendStatus says which tasks have a live agent here, every StatusEvery.
func (w *Worker) sendStatus(ctx context.Context, link *conn) {
	ticker := time.NewTicker(StatusEvery)
	defer ticker.Stop()
	for {
		running := sessions.Running()
		if running == nil {
			running = []string{}
		}
		if link.notify(ctx, noticeStatus, status{Running: running}) != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// handle answers one of the server's requests with this machine.
func (w *Worker) handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	m := w.Machine
	switch method {
	case methodPrepare:
		var request machines.WorktreeRequest
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		return m.Prepare(ctx, request)
	case methodHandOff:
		var request handOff
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		return nil, m.HandOff(ctx, request.Worktree, request.Message)
	case methodRemove:
		var wt worktrees.Worktree
		if err := json.Unmarshal(params, &wt); err != nil {
			return nil, err
		}
		return nil, m.Remove(ctx, wt)
	case methodPrepareTurn, methodStartTurn:
		var turn machines.Turn
		if err := json.Unmarshal(params, &turn); err != nil {
			return nil, err
		}
		if method == methodPrepareTurn {
			return nil, m.PrepareTurn(ctx, turn)
		}
		w.Log("%s: starting %s", turn.Task, turn.ID)
		return nil, m.StartTurn(ctx, turn)
	case methodStop:
		var request taskOnly
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		m.Stop(request.Task)
		return nil, nil
	case methodFiles:
		var ws workspace.Workspace
		if err := json.Unmarshal(params, &ws); err != nil {
			return nil, err
		}
		return m.Files(ctx, ws)
	case methodCommits:
		var ws workspace.Workspace
		if err := json.Unmarshal(params, &ws); err != nil {
			return nil, err
		}
		return m.Commits(ctx, ws)
	case methodStartHelper:
		var h machines.Helper
		if err := json.Unmarshal(params, &h); err != nil {
			return nil, err
		}
		w.Log("%s: starting Claude in the Workspace", h.Task)
		return nil, m.StartHelper(ctx, h)
	case methodRead:
		var request read
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		return m.Read(ctx, request.Workspace, request.Path)
	case methodAbout:
		return m.About(ctx)
	case methodSharedManifest, methodSharedDeliver, methodSharedUpload:
		var request sharedIn
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		switch method {
		case methodSharedManifest:
			return m.SharedManifest(ctx, request.Worktree)
		case methodSharedDeliver:
			return nil, m.SharedDeliver(ctx, request.Worktree, request.Files)
		}
		return m.SharedUpload(ctx, request.Worktree, request.Names)
	case methodSharedFetch:
		var request sharedFetch
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		w.Log("sending %s to the server, as a shared file", request.Path)
		return m.SharedFetch(ctx, request.Path, request.As, request.Anywhere)
	case methodAttach:
		var request attach
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		return w.stream(ctx, request)
	}
	return nil, fmt.Errorf("this worker doesn't know %q; update agentry on it", method)
}

// stream dials the terminal stream the server asked for and carries the
// task's session over it (the shape is on remotePane), until either ends.
func (w *Worker) stream(ctx context.Context, request attach) (size, error) {
	ws, _, err := websocket.Dial(ctx, w.Server+StreamPath+request.Stream, w.dialOptions())
	if err != nil {
		return size{}, err
	}
	ws.SetReadLimit(1 << 20)
	pane, err := w.Machine.Attach(request.Task, func(data []byte) {
		write, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ws.Write(write, websocket.MessageBinary, data)
	})
	if err != nil {
		ws.Close(websocket.StatusNormalClosure, err.Error())
		return size{}, err
	}
	go carry(ws, pane)
	cols, rows := pane.Size()
	return size{Cols: cols, Rows: rows}, nil
}

func carry(ws *websocket.Conn, pane terminal.Pane) {
	defer pane.Detach()
	go func() {
		<-pane.Ended()
		ws.Close(websocket.StatusNormalClosure, "the session ended")
	}()
	for {
		kind, data, err := ws.Read(context.Background())
		if err != nil {
			return
		}
		if kind == websocket.MessageBinary {
			pane.Type(data)
			continue
		}
		var m struct {
			Type       string
			Cols, Rows int
		}
		json.Unmarshal(data, &m)
		switch m.Type {
		case "resize":
			pane.Resize(m.Cols, m.Rows)
		case "snapshot":
			reply, _ := json.Marshal(map[string]string{"type": "snapshot", "data": base64.StdEncoding.EncodeToString(pane.Snapshot())})
			ws.Write(context.Background(), websocket.MessageText, reply)
		}
	}
}

// HubURL is the server's git address for a task's repo, as a worker reaches it.
func HubURL(server string) func(task string) string {
	return func(task string) string { return strings.TrimSuffix(server, "/") + GitPath + task }
}

// GitPath is where the server serves tasks' repos to workers: + the task's id.
const GitPath = "/api/worker/git/"
