package workerlink

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/coder/websocket"

	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/terminal"
	"gitlab.com/dev.chempa/agentry/internal/workspace"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// remote is a connected worker, as the server uses it: a machines.Machine
// whose every method is a request over the link.
type remote struct {
	name    string
	link    *conn
	streams *streams

	mu      sync.Mutex
	running []string // from the worker's last status
}

func (r *remote) Name() string { return r.name }

func (r *remote) Prepare(ctx context.Context, request machines.WorktreeRequest) (worktrees.Worktree, error) {
	var w worktrees.Worktree
	err := r.link.call(ctx, methodPrepare, request, &w)
	return w, err
}

func (r *remote) HandOff(ctx context.Context, w worktrees.Worktree, message string) error {
	return r.link.call(ctx, methodHandOff, handOff{Worktree: w, Message: message}, nil)
}

func (r *remote) Remove(ctx context.Context, w worktrees.Worktree) error {
	return r.link.call(ctx, methodRemove, w, nil)
}

func (r *remote) PrepareTurn(ctx context.Context, turn machines.Turn) error {
	return r.link.call(ctx, methodPrepareTurn, turn, nil)
}

func (r *remote) StartTurn(ctx context.Context, turn machines.Turn) error {
	err := r.link.call(ctx, methodStartTurn, turn, nil)
	if err == nil {
		r.setRunning(append(r.runningNow(), turn.Task)) // until its next status says
	}
	return err
}

func (r *remote) Stop(task string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r.link.call(ctx, methodStop, taskOnly{task}, nil)
}

// IsRunning goes by the worker's last status, a few seconds old at most,
// so the watchdog's tick never waits on the network.
func (r *remote) IsRunning(task string) bool { return slices.Contains(r.runningNow(), task) }

func (r *remote) setRunning(tasks []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = tasks
}

func (r *remote) runningNow() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.running)
}

func (r *remote) Files(ctx context.Context, w workspace.Workspace) ([]workspace.File, error) {
	var files []workspace.File
	err := r.link.call(ctx, methodFiles, w, &files)
	return files, err
}

func (r *remote) Read(ctx context.Context, w workspace.Workspace, path string) (workspace.Contents, error) {
	var contents workspace.Contents
	err := r.link.call(ctx, methodRead, read{Workspace: w, Path: path}, &contents)
	return contents, err
}

func (r *remote) About(ctx context.Context) (machines.About, error) {
	var about machines.About
	err := r.link.call(ctx, methodAbout, nil, &about)
	return about, err
}

// Attach asks the worker to attach to the task's session and dial a stream
// back with it; the pane is that stream.
func (r *remote) Attach(task string, onOutput func([]byte)) (terminal.Pane, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	id, arrived := r.streams.expect(r.name)
	defer r.streams.forget(id)

	var at size
	if err := r.link.call(ctx, methodAttach, attach{Task: task, Stream: id}, &at); err != nil {
		select {
		case stream := <-arrived: // dialed before it failed
			stream.close()
		default:
		}
		return nil, err
	}
	select {
	case stream := <-arrived:
		return newRemotePane(stream, at, onOutput), nil
	case <-ctx.Done():
		return nil, errors.New("the worker didn't send its terminal")
	}
}

// remotePane is a session on a worker, over its stream:
//
//	worker → server  binary: what the pane prints   text: {"type":"snapshot","data":base64}
//	server → worker  binary: keystrokes             text: {"type":"resize","cols","rows"} / {"type":"snapshot"}
type remotePane struct {
	stream    *stream
	onOutput  func([]byte)
	asking    sync.Mutex // one snapshot asked for at a time
	snapshots chan []byte
	mu        sync.Mutex
	size      size
}

func newRemotePane(s *stream, at size, onOutput func([]byte)) *remotePane {
	p := &remotePane{stream: s, onOutput: onOutput, snapshots: make(chan []byte, 1), size: at}
	go p.read()
	return p
}

func (p *remotePane) read() {
	defer p.stream.close()
	for {
		kind, data, err := p.stream.ws.Read(context.Background())
		if err != nil {
			return
		}
		if kind == websocket.MessageBinary {
			p.onOutput(data)
			continue
		}
		var m struct{ Type, Data string }
		if json.Unmarshal(data, &m) == nil && m.Type == "snapshot" {
			screen, _ := base64.StdEncoding.DecodeString(m.Data)
			select {
			case p.snapshots <- screen:
			default:
			}
		}
	}
}

func (p *remotePane) write(kind websocket.MessageType, data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p.stream.ws.Write(ctx, kind, data)
}

func (p *remotePane) Type(keys []byte) { p.write(websocket.MessageBinary, keys) }

func (p *remotePane) Resize(cols, rows int) {
	p.mu.Lock()
	p.size = size{cols, rows}
	p.mu.Unlock()
	data, _ := json.Marshal(map[string]any{"type": "resize", "cols": cols, "rows": rows})
	p.write(websocket.MessageText, data)
}

func (p *remotePane) Snapshot() []byte {
	p.asking.Lock()
	defer p.asking.Unlock()
	p.write(websocket.MessageText, []byte(`{"type":"snapshot"}`))
	select {
	case screen := <-p.snapshots:
		return screen
	case <-time.After(5 * time.Second):
		return nil
	case <-p.stream.done:
		return nil
	}
}

func (p *remotePane) Size() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.size.Cols, p.size.Rows
}

func (p *remotePane) Detach()                { p.stream.close() }
func (p *remotePane) Ended() <-chan struct{} { return p.stream.done }

var _ machines.Machine = (*remote)(nil)
