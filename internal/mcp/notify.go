package mcp

import (
	"context"
	"os/exec"
	"runtime"
	"time"
)

// Notifications on this computer when something new waits for you (a
// question, an approval, an epic to close), while Claude Code runs, so you
// hear of it even with the page closed. Without a notifier (notify-send on
// Linux, osascript on macOS) it stays quiet; the status line still shows it.

const notifyEvery = 15 * time.Second

// Watch notifies of everything that starts waiting for you after it starts,
// until ctx ends. What waits already is on the status line.
func (s *Server) Watch(ctx context.Context) {
	seen := map[string]bool{}
	first := true
	for {
		if _, waiting, err := s.board(); err == nil {
			now := map[string]bool{}
			for _, q := range waiting {
				key := q.Task + "/" + q.Kind + "/" + q.Action + "/" + q.Text
				now[key] = true
				if !first && !seen[key] {
					notify("Agentry: "+q.Task+" needs you", describe(q)+"\n"+s.agentry.PageURL("/tasks/"+q.Task))
				}
			}
			seen, first = now, false
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(notifyEvery):
		}
	}
}

func notify(title, body string) {
	switch runtime.GOOS {
	case "linux":
		exec.Command("notify-send", "--app-name=Agentry", title, body).Run()
	case "darwin":
		exec.Command("osascript", "-e", "display notification "+appleQuote(body)+" with title "+appleQuote(title)).Run()
	}
}

func appleQuote(text string) string {
	quoted := []rune{'"'}
	for _, r := range text {
		if r == '"' || r == '\\' {
			quoted = append(quoted, '\\')
		}
		quoted = append(quoted, r)
	}
	return string(append(quoted, '"'))
}
