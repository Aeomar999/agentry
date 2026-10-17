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

// Watch notifies of what starts waiting for you after it starts, and of
// tasks that finish, until ctx ends. What waits already is on the status line.
func (s *Server) Watch(ctx context.Context) {
	waiting, err := News(s.agentry)
	for err != nil && ctx.Err() == nil { // the server may be starting
		time.Sleep(notifyEvery)
		waiting, err = News(s.agentry)
	}
	seen := Seen(waiting)
	for ctx.Err() == nil {
		fresh, err := WaitForNew(ctx, s.agentry, seen)
		if err != nil {
			time.Sleep(notifyEvery)
			continue
		}
		for _, w := range fresh {
			title := map[string]string{"done": "Agentry: " + w.Task + " is done", "said": "Agentry: " + w.Task + "'s coordinator",
				"offline": "Agentry: a machine went offline"}[w.Kind]
			if title == "" {
				title = "Agentry: " + w.Task + " needs you"
			}
			notify(title, w.Line+"\n"+s.agentry.PageURL("/tasks/"+w.Task))
			seen[w.Key] = true
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
