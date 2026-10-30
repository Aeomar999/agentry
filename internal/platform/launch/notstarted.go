package launch

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Tail is what a server wrote to its log from a point on: the log's path,
// its last lines with the token hidden, and whether the log couldn't be
// read. Log is empty when Options.Log is.
type Tail struct {
	Log, Said string
	Unread    bool
}

// after is reason with the tail after it, as an error says it.
func (t Tail) after(reason string) string {
	switch {
	case t.Log == "":
		return reason
	case t.Unread:
		return fmt.Sprintf("%s, and its log, %s, couldn't be read", reason, t.Log)
	case t.Said == "":
		return fmt.Sprintf("%s, and wrote nothing to its log, %s", reason, t.Log)
	}
	return fmt.Sprintf("%s. Its log, %s, says:\n%s", reason, t.Log, t.Said)
}

// NotStarted is Ensure's error when the server it started exited, or
// didn't answer in time, with the lines this start wrote to its log.
type NotStarted struct {
	Tail
}

// Reason is what went wrong, without the log.
func (e *NotStarted) Reason() string { return "the server didn't start" }

func (e *NotStarted) Error() string { return e.after(e.Reason()) }

// Stopped is why a server Ensure gave stopped, once its Exited has closed.
// Started says this program started it, and then Tail holds the lines it
// wrote to its log since it started.
type Stopped struct {
	Started bool
	Tail
}

// Reason is what went wrong, without the log.
func (e *Stopped) Reason() string {
	if e.Started {
		return "the server this program started stopped"
	}
	return "the server that was running went away"
}

func (e *Stopped) Error() string { return e.after(e.Reason()) }

// shownLines is how many of a log's last lines a start that failed shows.
const shownLines = 15

// LastLines is the last n lines of text, without blank lines at its ends.
func LastLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.Join(lines[max(0, len(lines)-n):], "\n")
}

// TokenIn is the page token in the data folder, once the server has made it.
func TokenIn(dataDir string) string {
	token, _ := os.ReadFile(filepath.Join(dataDir, "ui-token"))
	return strings.TrimSpace(string(token))
}

// logSize is the size of the log at path now, 0 if there is none yet.
func logSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// readLimit is the most of a log tailOf reads: enough for its last lines.
const readLimit = 64 << 10

// tailOf is what the log at path holds from its size from on, the token
// in the data folder hidden. A log now smaller than from was started again,
// so all of it counts.
func tailOf(path string, from int64, dataDir string) Tail {
	if path == "" {
		return Tail{}
	}
	log, err := os.Open(path)
	if err != nil {
		return Tail{Log: path, Unread: true}
	}
	defer log.Close()
	size := logSize(path)
	if size < from {
		from = 0
	}
	log.Seek(max(from, size-readLimit), io.SeekStart)
	said, err := io.ReadAll(io.LimitReader(log, readLimit))
	if err != nil {
		return Tail{Log: path, Unread: true}
	}
	return Tail{Log: path, Said: Redact(LastLines(string(said), shownLines), TokenIn(dataDir))}
}
