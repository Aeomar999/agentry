package launch

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// NotStarted is Ensure's error when the server it started exited, or
// didn't answer in time: the log its output went to, and the last lines
// this start wrote there, with the token hidden. Log is empty when
// Options.Log is.
type NotStarted struct {
	Log, Said string
}

// Reason is what went wrong, without the log.
func (e *NotStarted) Reason() string { return "the server didn't start" }

func (e *NotStarted) Error() string {
	switch {
	case e.Log == "":
		return e.Reason()
	case e.Said == "":
		return fmt.Sprintf("%s, and wrote nothing to its log, %s", e.Reason(), e.Log)
	}
	return fmt.Sprintf("%s. Its log, %s, says:\n%s", e.Reason(), e.Log, e.Said)
}

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

// readLimit is the most of a log notStarted reads: enough for its last lines.
const readLimit = 64 << 10

// notStarted is the error for a start whose output went to the log at path
// from its size from on, the token in the data folder hidden. A log now
// smaller than from was started again, so all of it is this start's.
func notStarted(path string, from int64, dataDir string) *NotStarted {
	if path == "" {
		return &NotStarted{}
	}
	log, err := os.Open(path)
	if err != nil {
		return &NotStarted{Log: path}
	}
	defer log.Close()
	size := logSize(path)
	if size < from {
		from = 0
	}
	log.Seek(max(from, size-readLimit), io.SeekStart)
	said, _ := io.ReadAll(io.LimitReader(log, readLimit))
	return &NotStarted{Log: path, Said: Redact(LastLines(string(said), shownLines), TokenIn(dataDir))}
}
