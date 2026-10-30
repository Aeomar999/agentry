// Package terminal shows a task's agent session live in the browser. One
// watch per session (sessions.Attach) gets everything the program prints;
// it is fanned out to every viewer, and keystrokes go back only from the
// one viewer in control (proof 04).
package terminal

import (
	"errors"

	"gitlab.com/dev.chempa/agentry/internal/platform/sessions"
)

// Pane is a session being watched: what the hub needs from it. On this
// machine it is a watch on the session's host; a worker's sessions come
// through its link (package workerlink).
type Pane interface {
	Type(keys []byte)
	Resize(cols, rows int)
	Snapshot() []byte // the current screen, for a viewer joining
	Size() (cols, rows int)
	Detach()                // stop watching; the session keeps running
	Ended() <-chan struct{} // closed when the session ends, or the pane is detached
}

// ErrNoSession is a session that isn't running.
var ErrNoSession = errors.New("no session is running")

// Opener attaches to the named session, sending what it prints to onOutput.
type Opener func(name string, onOutput func([]byte)) (Pane, error)

// Attach opens a session on this machine: a task's (by its ID) or the desk's.
func Attach(name string, onOutput func([]byte)) (Pane, error) {
	session := sessions.SocketFor(name)
	if !sessions.Exists(session) {
		return nil, ErrNoSession
	}
	watch, err := sessions.Attach(session, onOutput, nil)
	if err != nil {
		return nil, ErrNoSession
	}
	return &local{Watch: watch, session: session}, nil
}

// local is a session on this machine, watched.
type local struct {
	*sessions.Watch
	session string
}

func (l *local) Snapshot() []byte       { return sessions.Snapshot(l.session) }
func (l *local) Size() (cols, rows int) { return sessions.Size(l.session) }
func (l *local) Detach()                { l.Close() }
