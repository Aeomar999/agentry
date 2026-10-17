package workerlink

import (
	"net/http"
	"sync"

	"github.com/coder/websocket"

	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// stream is one terminal's own websocket, from a worker.
type stream struct {
	ws   *websocket.Conn
	done chan struct{}
	once sync.Once
}

func (s *stream) close() {
	s.once.Do(func() {
		s.ws.Close(websocket.StatusNormalClosure, "")
		close(s.done)
	})
}

// streams are the terminal streams the server has asked workers for and
// is waiting on, by id.
type streams struct {
	mu      sync.Mutex
	waiting map[string]expected
}

type expected struct {
	worker  string // only this worker may send it
	arrived chan *stream
}

func newStreams() *streams { return &streams{waiting: map[string]expected{}} }

func (s *streams) expect(worker string) (string, <-chan *stream) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := workers.NewSecret()
	arrived := make(chan *stream, 1)
	s.waiting[id] = expected{worker: worker, arrived: arrived}
	return id, arrived
}

func (s *streams) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.waiting, id)
}

// serve takes a stream a worker dialed, and keeps it open until its pane is done.
func (s *streams) serve(w http.ResponseWriter, r *http.Request, worker workers.Worker, id string) {
	s.mu.Lock()
	want, ok := s.waiting[id]
	delete(s.waiting, id)
	s.mu.Unlock()
	if !ok || want.worker != worker.Name {
		http.Error(w, "no terminal was asked for with that id", http.StatusNotFound)
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(1 << 20)
	st := &stream{ws: ws, done: make(chan struct{})}
	want.arrived <- st // buffered: never blocks
	select {
	case <-st.done:
	case <-r.Context().Done(): // the server is stopping
		st.close()
	}
}
