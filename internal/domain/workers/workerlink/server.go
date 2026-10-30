package workerlink

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

// StatusEvery is how often a worker says what it runs. A link silent for
// a few of these is taken as dead, even if the network never said so.
const StatusEvery = 2 * time.Second

const silentFor = 10 * StatusEvery

// Server is the server's end of every worker's link.
type Server struct {
	store   *store.Store
	fleet   *machines.Fleet
	streams *streams
}

func NewServer(s *store.Store, fleet *machines.Fleet) *Server {
	return &Server{store: s, fleet: fleet, streams: newStreams()}
}

// Connect serves a worker's link until it closes. The worker has already
// shown its credential (package web).
func (s *Server) Connect(w http.ResponseWriter, r *http.Request, worker workers.Worker) {
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	m := &remote{name: worker.Name, streams: s.streams}
	hello := make(chan workers.Machine, 1)
	var heard atomic.Int64 // when the worker last said anything, in Unix ns
	heard.Store(time.Now().UnixNano())
	m.link = newConn(ws, nil, func(method string, params []byte) {
		heard.Store(time.Now().UnixNano())
		switch method {
		case noticeHello:
			var machine workers.Machine
			if json.Unmarshal(params, &machine) == nil {
				select {
				case hello <- machine:
				default:
				}
			}
		case noticeStatus:
			var st status
			if json.Unmarshal(params, &st) == nil {
				m.setRunning(st.Running)
			}
		}
	})
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		m.link.run(ctx)
	}()
	go hangUpWhenSilent(ctx, cancel, &heard)

	select {
	case machine := <-hello:
		s.fleet.Connect(m, machine.Engines)
		s.propose(workers.Connected(worker.Name, machine))
	case <-time.After(10 * time.Second):
		ws.Close(websocket.StatusPolicyViolation, "say hello first")
		return
	case <-closed:
		return
	}
	<-closed
	if s.fleet.Disconnect(m) { // else a newer link replaced this one
		s.propose(workers.Disconnected(worker.Name))
	}
}

func hangUpWhenSilent(ctx context.Context, hangUp func(), heard *atomic.Int64) {
	ticker := time.NewTicker(StatusEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if now.Sub(time.Unix(0, heard.Load())) > silentFor {
				hangUp()
				return
			}
		}
	}
}

// Stream takes a terminal stream the worker dialed (StreamPath + id).
func (s *Server) Stream(w http.ResponseWriter, r *http.Request, worker workers.Worker, id string) {
	s.streams.serve(w, r, worker, id)
}

// Disconnect drops a worker's link at once, as when it is removed.
func (s *Server) Disconnect(name string) {
	if m, ok := s.fleet.Get(name); ok && !machines.IsLocal(name) {
		if r, ok := m.(*remote); ok {
			r.link.ws.Close(websocket.StatusNormalClosure, "removed")
		}
	}
}

func (s *Server) propose(p events.Proposal) {
	if _, rejection, err := s.store.Propose(p); err != nil || rejection != nil {
		slog.Warn("worker link: proposal refused", "type", p.Type, "rejection", rejection, "err", err)
	}
}

// ForgetConnections records every worker the log has online as offline. At
// start no link is open yet; they come back as their workers dial in, and
// the fleet expects every one meanwhile (machines.OfflineGrace): any of
// them may have agents running, whatever the log last said.
func ForgetConnections(s *store.Store, state *current.State, fleet *machines.Fleet) error {
	if err := state.Broken(events.WorkersStream); err != nil {
		return err
	}
	for _, w := range state.Workers().Workers {
		fleet.Expect(w.Name)
		if w.Online {
			if _, _, err := s.Propose(workers.Disconnected(w.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}
