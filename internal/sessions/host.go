package sessions

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/aymanbagabas/go-pty"
	"github.com/charmbracelet/x/vt"
)

// The host is the process that keeps one session: Start runs this same
// program again, detached, with the session to host in its environment.
// It runs the command in a pseudo-terminal (a pty, or ConPTY on Windows),
// keeps its screen with a terminal emulator, and answers on the session's
// socket. It outlives the server that started it, and when the command
// exits it stays, with the last screen, until it is stopped (proof 03).

const (
	hostEnv = "AGENTRY_SESSION_HOST" // the name of the session this process is to host
	specEnv = "AGENTRY_SESSION_SPEC" // the session's Spec, as JSON
)

// A process started to host a session does only that. This runs before
// main (or a test's flags), in any program that has this package.
func init() {
	if name := os.Getenv(hostEnv); name != "" {
		os.Exit(runHost(name, os.Getenv(specEnv)))
	}
}

type host struct {
	pty      pty.Pty
	cmd      *pty.Cmd
	screen   *vt.SafeEmulator
	listener net.Listener

	mu         sync.Mutex
	alive      bool
	cols, rows int
	clients    map[*client]bool
	stopping   sync.Once
}

// client is one attached viewer: what is sent to it waits in out.
type client struct {
	conn net.Conn
	out  chan []byte // whole frames
}

func runHost(name, encodedSpec string) int {
	var spec Spec
	if err := json.Unmarshal([]byte(encodedSpec), &spec); err != nil || len(spec.Command) == 0 {
		fmt.Fprintln(os.Stderr, "session host: no command to run")
		return 2
	}
	os.MkdirAll(Dir(), 0o700)
	os.Remove(socketPath(name))
	listener, err := net.Listen("unix", socketPath(name))
	if err != nil {
		fmt.Fprintln(os.Stderr, "session host:", err)
		return 1
	}
	h := &host{listener: listener, cols: Columns, rows: Rows, clients: map[*client]bool{}}
	if err := h.start(spec); err != nil {
		fmt.Fprintln(os.Stderr, "session host:", err)
		listener.Close()
		os.Remove(socketPath(name))
		return 1
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			return 0 // stopped
		}
		go h.serve(conn)
	}
}

func (h *host) start(spec Spec) error {
	p, err := pty.New()
	if err != nil {
		return err
	}
	p.Resize(h.cols, h.rows)
	program, args, err := resolve(spec)
	if err != nil {
		p.Close()
		return err
	}
	cmd := p.Command(program, args...)
	cmd.Dir = spec.Dir
	cmd.Env = childEnv(spec.Env)
	if err := cmd.Start(); err != nil {
		p.Close()
		return err
	}
	h.pty, h.cmd, h.alive = p, cmd, true
	h.screen = vt.NewSafeEmulator(h.cols, h.rows)
	go io.Copy(p, h.screen) // the emulator's answers to the program's questions, such as where the cursor is
	go h.relay()
	go func() {
		cmd.Wait()
		h.mu.Lock()
		h.alive = false
		h.mu.Unlock()
		h.broadcast(frameDead, nil)
	}()
	return nil
}

// relay passes what the program prints to the screen and every client.
func (h *host) relay() {
	buffer := make([]byte, 32<<10)
	for {
		n, err := h.pty.Read(buffer)
		if n > 0 {
			data := append([]byte(nil), buffer[:n]...)
			h.screen.Write(data)
			h.broadcast(frameOutput, data)
		}
		if err != nil {
			return
		}
	}
}

// broadcast sends a frame to every client. One too slow to keep up is
// dropped, rather than holding up the program or the others: it can
// attach again, and starts from a snapshot.
func (h *host) broadcast(kind byte, payload []byte) {
	frame := encodeFrame(kind, payload)
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.out <- frame:
		default:
			h.dropLocked(c)
		}
	}
}

func (h *host) serve(conn net.Conn) {
	kind, payload, err := readFrame(conn)
	var r request
	if err != nil || kind != frameRequest || json.Unmarshal(payload, &r) != nil {
		conn.Close()
		return
	}
	switch r.Op {
	case "attach":
		h.attach(conn)
		return
	case "status":
		h.reply(conn, answer{})
	case "screen":
		h.reply(conn, answer{Text: h.screen.String()})
	case "snapshot":
		h.reply(conn, answer{Text: h.snapshot()})
	case "resize":
		h.resize(r.Cols, r.Rows)
		h.reply(conn, answer{})
	case "stop":
		h.reply(conn, answer{})
		conn.Close()
		h.stop()
		return
	default:
		h.reply(conn, answer{Error: "no op " + r.Op})
	}
	conn.Close()
}

func (h *host) reply(conn net.Conn, a answer) {
	a.Status = h.status()
	encoded, _ := json.Marshal(a)
	writeFrame(conn, frameAnswer, encoded)
}

func (h *host) status() Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Status{Alive: h.alive, Cols: h.cols, Rows: h.rows, PID: os.Getpid()}
}

// attach streams the session to conn and takes its keys and sizes.
func (h *host) attach(conn net.Conn) {
	c := &client{conn: conn, out: make(chan []byte, 4096)}
	h.reply(conn, answer{})
	h.mu.Lock()
	h.clients[c] = true
	dead := !h.alive
	h.mu.Unlock()
	if dead {
		c.out <- encodeFrame(frameDead, nil)
	}
	go func() {
		for frame := range c.out {
			if _, err := conn.Write(frame); err != nil {
				return
			}
		}
	}()
	defer func() {
		h.mu.Lock()
		h.dropLocked(c)
		h.mu.Unlock()
	}()
	for {
		kind, payload, err := readFrame(conn)
		if err != nil {
			return
		}
		switch kind {
		case frameKeys:
			h.pty.Write(payload)
		case frameSize:
			var cols, rows int
			if n, _ := fmt.Sscan(string(payload), &cols, &rows); n == 2 {
				h.resize(cols, rows)
			}
		}
	}
}

// dropLocked lets a client go: its writer ends, and so does its connection.
func (h *host) dropLocked(c *client) {
	if h.clients[c] {
		delete(h.clients, c)
		close(c.out)
		c.conn.Close()
	}
}

func (h *host) resize(cols, rows int) {
	if cols < 2 || rows < 2 || cols > 1000 || rows > 1000 {
		return
	}
	h.mu.Lock()
	h.cols, h.rows = cols, rows
	h.mu.Unlock()
	h.screen.Resize(cols, rows)
	h.pty.Resize(cols, rows) // the program gets SIGWINCH, as in any terminal
}

// snapshot is the screen as a viewer joining first sees it: with its
// colours, and the cursor where it is.
func (h *host) snapshot() string {
	lines := strings.Split(strings.ReplaceAll(h.screen.Render(), "\r", ""), "\n")
	cursor := h.screen.CursorPosition()
	return "\x1b[H\x1b[2J" + strings.Join(lines, "\r\n") + fmt.Sprintf("\x1b[%d;%dH", cursor.Y+1, cursor.X+1)
}

// stop ends the program and everything it started, and this host with it.
func (h *host) stop() {
	h.stopping.Do(func() {
		if h.status().Alive {
			killTree(h.cmd.Process)
		}
		h.pty.Close()
		h.listener.Close()
		os.Remove(h.listener.Addr().String())
		time.AfterFunc(time.Second, func() { os.Exit(0) }) // after the clients hear it close
	})
}

// resolve finds the program on the session's PATH (or this one's), as a
// shell would: the pty starts it by its full path. On Windows that finds
// claude.exe, and a .cmd or .bat launcher (as npm installs) runs in cmd.
func resolve(spec Spec) (string, []string, error) {
	if path, ok := spec.Env["PATH"]; ok {
		os.Setenv("PATH", path) // this process hosts this session alone
	}
	program, err := exec.LookPath(spec.Command[0])
	if err != nil {
		return "", nil, err
	}
	args := spec.Command[1:]
	if ext := strings.ToLower(filepath.Ext(program)); runtime.GOOS == "windows" && (ext == ".cmd" || ext == ".bat") {
		shell, err := exec.LookPath("cmd.exe")
		if err != nil {
			return "", nil, err
		}
		return shell, append([]string{"/c", program}, args...), nil
	}
	return program, args, nil
}

func encodeFrame(kind byte, payload []byte) []byte {
	var b strings.Builder
	writeFrame(&b, kind, payload)
	return []byte(b.String())
}

// childEnv is this environment, without what made this process a host,
// with the session's own, and a terminal the program knows how to draw on.
func childEnv(extra map[string]string) []string {
	set := map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}
	for name, value := range extra {
		set[envKey(name)] = value
	}
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		_, replaced := set[envKey(name)]
		if !replaced && name != hostEnv && name != specEnv && !strings.HasPrefix(name, "TMUX") {
			env = append(env, kv)
		}
	}
	for _, name := range sortedKeys(set) {
		env = append(env, name+"="+set[name])
	}
	return env
}

// envKey is a variable's name as the system compares it: on Windows, Path
// and PATH are one.
func envKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}
