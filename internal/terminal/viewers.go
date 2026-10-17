package terminal

import (
	"encoding/json"
	"fmt"
	"sync"
)

// viewers is who is watching one session, and who (if anyone) is in control.
// Output goes to every viewer; keystrokes are accepted only from the one in
// control, and everything else is dropped here, on the server (proof 04).
type viewers struct {
	mu         sync.Mutex
	all        map[string]*viewer
	controller string // id of the viewer in control, or ""
	cols, rows int    // the session's size, which every viewer shows
	nextID     int
	control    Pane
}

type viewer struct {
	id     string
	output chan []byte // terminal bytes for this viewer
	state  chan []byte // JSON state messages for this viewer
}

func newViewers() *viewers {
	return &viewers{all: map[string]*viewer{}}
}

// broadcast sends session output to everyone. A viewer too slow to keep up is
// disconnected (its browser reconnects and gets a fresh snapshot) rather than
// slowing everyone else down.
func (v *viewers) broadcast(data []byte) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for id, viewer := range v.all {
		select {
		case viewer.output <- data:
		default:
			v.removeLocked(id)
		}
	}
}

func (v *viewers) join() *viewer {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.nextID++
	viewer := &viewer{id: fmt.Sprintf("viewer-%d", v.nextID), output: make(chan []byte, 4096), state: make(chan []byte, 16)}
	v.all[viewer.id] = viewer
	v.sendStateLocked()
	return viewer
}

func (v *viewers) leave(viewer *viewer) (nobodyLeft bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.removeLocked(viewer.id)
	v.sendStateLocked()
	return len(v.all) == 0
}

// closeAll disconnects everyone, as when the session ends.
func (v *viewers) closeAll() {
	v.mu.Lock()
	defer v.mu.Unlock()
	for id := range v.all {
		v.removeLocked(id)
	}
}

func (v *viewers) removeLocked(id string) {
	viewer, ok := v.all[id]
	if !ok {
		return
	}
	if v.controller == id {
		v.controller = ""
	}
	delete(v.all, id)
	close(viewer.output)
}

// takeControl succeeds only if nobody else has control. One person at a time.
func (v *viewers) takeControl(viewer *viewer) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.controller == "" {
		v.controller = viewer.id
	}
	v.sendStateLocked()
}

func (v *viewers) releaseControl(viewer *viewer) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.controller == viewer.id {
		v.controller = ""
	}
	v.sendStateLocked()
}

// typeFrom passes keystrokes to the session only from the viewer in control.
func (v *viewers) typeFrom(viewer *viewer, keys []byte) {
	v.mu.Lock()
	inControl := v.controller == viewer.id
	v.mu.Unlock()
	if inControl {
		v.control.Type(keys)
	}
}

// The sizes a viewer may ask for: big enough to be usable, small enough
// that a bad request can't make tmux allocate a huge screen.
const (
	minCols, maxCols = 20, 500
	minRows, maxRows = 5, 200
)

// resizeFrom resizes the session for a viewer, unless someone else is in
// control: then the size is theirs to set.
func (v *viewers) resizeFrom(viewer *viewer, cols, rows int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.controller != "" && v.controller != viewer.id {
		return
	}
	cols, rows = max(minCols, min(maxCols, cols)), max(minRows, min(maxRows, rows))
	if cols == v.cols && rows == v.rows {
		return
	}
	v.cols, v.rows = cols, rows
	v.control.Resize(cols, rows)
	v.sendStateLocked()
}

// sendStateLocked tells every viewer who they are, who is in control, and
// the session's size.
func (v *viewers) sendStateLocked() {
	for _, viewer := range v.all {
		select {
		case viewer.state <- v.stateLocked(viewer):
		default:
		}
	}
}

// stateFor is the state message for one viewer, as sendStateLocked sends it.
func (v *viewers) stateFor(viewer *viewer) []byte {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.stateLocked(viewer)
}

func (v *viewers) stateLocked(viewer *viewer) []byte {
	message, _ := json.Marshal(map[string]any{
		"type": "state", "you": viewer.id, "controller": v.controller, "viewers": len(v.all),
		"cols": v.cols, "rows": v.rows,
	})
	return message
}
