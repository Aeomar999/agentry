package mcp

import (
	"context"
	"os"
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
				"offline": "Agentry: a machine went offline", "ideas": "Agentry: a new idea"}[w.Kind]
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
	case "windows":
		// A toast, through PowerShell, which every Windows has. The words go
		// in the environment, so nothing in them is read as script.
		toast := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", windowsToast)
		toast.Env = append(os.Environ(), "AGENTRY_TITLE="+title, "AGENTRY_BODY="+body)
		toast.Run()
	}
}

// windowsToast shows $env:AGENTRY_TITLE and $env:AGENTRY_BODY as a toast,
// as PowerShell's, since an app needs to be registered to show its own.
const windowsToast = `[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$text = $xml.GetElementsByTagName('text')
$text.Item(0).AppendChild($xml.CreateTextNode($env:AGENTRY_TITLE)) > $null
$text.Item(1).AppendChild($xml.CreateTextNode($env:AGENTRY_BODY)) > $null
$app = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($app).Show([Windows.UI.Notifications.ToastNotification]::new($xml))`

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
