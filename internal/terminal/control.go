// Package terminal shows a task's agent session live in the browser. One tmux
// control-mode client per session (tmux -C) gets everything the pane prints;
// it is fanned out to every viewer, and keystrokes go back only from the one
// viewer in control (proof 04).
package terminal

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

// control is the one control-mode client attached to a task's session.
type control struct {
	socket   string
	process  *exec.Cmd
	commands io.WriteCloser // the client's stdin
	mu       sync.Mutex
	ended    chan struct{} // closed when the client exits, as when the session ends
	detached sync.Once
}

func attach(socket string, onOutput func([]byte)) (*control, error) {
	process := exec.Command("tmux", "-L", socket, "-C", "attach-session", "-t", sessions.Pane)
	stdin, err := process.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := process.Start(); err != nil {
		return nil, err
	}
	c := &control{socket: socket, process: process, commands: stdin, ended: make(chan struct{})}
	// If a viewer can't keep up, tmux pauses the pane instead of piling up
	// output; it is resumed at once (see read).
	c.run("refresh-client -f pause-after=5")
	go c.read(stdout, onOutput)
	return c, nil
}

func (c *control) run(command string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	io.WriteString(c.commands, command+"\n")
}

// detach ends this client only; the session keeps running.
func (c *control) detach() {
	c.detached.Do(func() {
		c.process.Process.Kill()
		c.process.Wait()
	})
}

// read handles the lines tmux sends a control client, until it exits.
func (c *control) read(stdout io.Reader, onOutput func([]byte)) {
	defer close(c.ended)
	lines := bufio.NewReaderSize(stdout, 1<<20)
	for {
		line, err := lines.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSuffix(line, "\n")
		switch {
		case strings.HasPrefix(line, "%output "):
			// %output %<pane> <escaped bytes>
			_, data, _ := strings.Cut(strings.TrimPrefix(line, "%output "), " ")
			onOutput(unescape(data))
		case strings.HasPrefix(line, "%extended-output "):
			// with pause-after set: %extended-output %<pane> <age> : <escaped bytes>
			_, data, _ := strings.Cut(line, " : ")
			onOutput(unescape(data))
		case strings.HasPrefix(line, "%pause "):
			c.run("refresh-client -A '" + strings.TrimPrefix(line, "%pause ") + ":continue'")
		case strings.HasPrefix(line, "%exit"):
			return
		}
		// %begin / %end (command replies), %layout-change and the rest are ignored.
	}
}

// unescape reverses tmux's control-mode escaping: bytes below 32 and the
// backslash are written as \ooo (three octal digits).
func unescape(data string) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == '\\' && i+3 < len(data) {
			if value, err := strconv.ParseUint(data[i+1:i+4], 8, 8); err == nil {
				out = append(out, byte(value))
				i += 3
				continue
			}
		}
		out = append(out, data[i])
	}
	return out
}

// typeBytes sends keystrokes to the pane exactly as given, as hex, so arrows,
// Ctrl-C and pastes all arrive unchanged.
func (c *control) typeBytes(keys []byte) {
	for len(keys) > 0 {
		chunk := keys[:min(len(keys), 256)]
		keys = keys[len(chunk):]
		hex := make([]string, len(chunk))
		for i, b := range chunk {
			hex[i] = fmt.Sprintf("%02x", b)
		}
		c.run("send-keys -t " + sessions.Pane + " -H " + strings.Join(hex, " "))
	}
}

// snapshot is what a new viewer sees first: the current screen with colours,
// and the cursor where it really is.
func (c *control) snapshot() []byte {
	screen, _ := exec.Command("tmux", "-L", c.socket, "capture-pane", "-p", "-e", "-t", sessions.Pane).Output()
	cursor, _ := exec.Command("tmux", "-L", c.socket, "display-message", "-p", "-t", sessions.Pane, "#{cursor_x} #{cursor_y}").Output()
	var x, y int
	fmt.Sscan(string(cursor), &x, &y)
	lines := strings.Split(strings.TrimSuffix(string(screen), "\n"), "\n")
	return []byte("\x1b[H\x1b[2J" + strings.Join(lines, "\r\n") + fmt.Sprintf("\x1b[%d;%dH", y+1, x+1))
}
