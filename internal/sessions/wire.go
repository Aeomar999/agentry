package sessions

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The host and its clients talk over the session's local socket in frames:
// one byte saying what it is, four bytes of length, then the bytes.
//
//	client → host: request (JSON, one per connection, first), keys, size
//	host → client: answer (JSON), output, dead (the program exited)
//
// A connection asks one thing and gets one answer, except "attach", which
// then streams the session's output, and takes keys and sizes, until either
// end closes it.

const (
	frameRequest = 'q'
	frameAnswer  = 'a'
	frameKeys    = 'k'
	frameSize    = 's'
	frameOutput  = 'o'
	frameDead    = 'x'
)

type request struct {
	Op   string `json:"op"` // status, screen, snapshot, resize, stop, attach
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

// Status is a session as its host reports it.
type Status struct {
	Alive bool `json:"alive"` // the program is still running
	Cols  int  `json:"cols"`
	Rows  int  `json:"rows"`
	PID   int  `json:"pid"` // the host's
}

type answer struct {
	Status
	Text  string `json:"text,omitempty"` // for screen and snapshot
	Error string `json:"error,omitempty"`
}

func writeFrame(w io.Writer, kind byte, payload []byte) error {
	header := make([]byte, 5)
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := w.Write(append(header, payload...)); err != nil {
		return err
	}
	return nil
}

func readFrame(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	size := binary.BigEndian.Uint32(header[1:])
	if size > 16<<20 {
		return 0, nil, errors.New("a frame too big")
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return header[0], payload, nil
}

// dirEnv names another folder for the sessions: a test run's own, so tests
// running at once never touch each other's sessions. Hosts get it from the
// program that started them.
const dirEnv = "AGENTRY_SESSIONS_DIR"

func init() {
	if testing.Testing() && os.Getenv(dirEnv) == "" {
		os.Setenv(dirEnv, filepath.Join(os.TempDir(), fmt.Sprintf("agentry-test-%d", os.Getpid())))
	}
}

// Dir is where the sessions' sockets are: this user's, on this machine.
func Dir() string {
	if dir := os.Getenv(dirEnv); dir != "" {
		return dir
	}
	return sessionsDir()
}

func socketPath(name string) string { return filepath.Join(Dir(), name+".sock") }

// ask sends one request to the session's host and returns its answer.
func ask(name string, r request) (answer, error) {
	conn, err := net.DialTimeout("unix", socketPath(name), 2*time.Second)
	if err != nil {
		return answer{}, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	encoded, _ := json.Marshal(r)
	if err := writeFrame(conn, frameRequest, encoded); err != nil {
		return answer{}, err
	}
	kind, payload, err := readFrame(conn)
	if err != nil {
		return answer{}, err
	}
	var a answer
	if kind != frameAnswer || json.Unmarshal(payload, &a) != nil {
		return answer{}, errors.New("the session's host answered something else")
	}
	if a.Error != "" {
		return a, errors.New(a.Error)
	}
	return a, nil
}
