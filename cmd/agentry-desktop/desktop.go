// Command agentry-desktop shows Agentry's page in a window of its own. It
// uses the Agentry running from ~/.agentry, or starts the agentry binary
// beside it, and when it quits it stops only a server it started.
//
// The window is Wails v2: build it with `wails build`, or with
// `go build -tags desktop,production` (Linux adds webkit2_41). The files
// without a build tag hold what needs no window, so they test without cgo.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/launch"
)

// binaryNextTo is the agentry binary in the folder of exe, the app's own
// executable.
func binaryNextTo(exe, goos string) string {
	name := "agentry"
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name)
}

// launchOptions are where the app finds or starts Agentry: the data folder
// and default repo `agentry open` uses, listening on this computer only.
func launchOptions(home, exe, goos string, output io.Writer) launch.Options {
	return launch.Options{
		DataDir: filepath.Join(home, ".agentry"),
		Repo:    filepath.Join(home, "Agentry"),
		Binary:  binaryNextTo(exe, goos),
		Host:    "127.0.0.1",
		Port:    7800,
		Output:  output,
	}
}

// tokenIn is the page token in the data folder, once the server has made it.
func tokenIn(dataDir string) string {
	token, _ := os.ReadFile(filepath.Join(dataDir, "ui-token"))
	return strings.TrimSpace(string(token))
}

// originOf is the scheme, host and port of link.
func originOf(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

var startTemplate = template.Must(template.New("start").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>Agentry</title>
<style>body{font:15px system-ui,sans-serif;margin:0;display:grid;place-items:center;height:100vh;background:#fff;color:#111}
@media (prefers-color-scheme:dark){body{background:#111;color:#eee}}</style></head>
<body>{{if .Failed}}<p>Agentry didn't start: {{.Failed}}</p>
{{else}}<p>Opening Agentry…</p>
<script>window.agentryStart = true; location.replace({{.Page}});</script>{{end}}</body></html>
`))

// startPage is the page the window opens on, served by the app itself: it
// goes on to Agentry's page, signed in, or says why Agentry didn't start,
// with the token hidden.
func startPage(page string, failed error, token string) []byte {
	var data struct{ Page, Failed string }
	if failed != nil {
		data.Failed = launch.Redact(failed.Error(), token)
	} else {
		data.Page = page
	}
	var out bytes.Buffer
	startTemplate.Execute(&out, data)
	return out.Bytes()
}

// linkEvent is the event the page sends with a link that leaves Agentry.
const linkEvent = "agentry:link"

// pageScript runs in the window after every page load. On Agentry's page it
// sets the zoom and sends links that leave Agentry's origin, clicked or
// opened with window.open, to the app as linkEvent; a new window on
// Agentry's own origin opens in this one. Any other page but the app's
// start page goes back to Agentry. It holds no token.
func pageScript(origin string, zoom float64) string {
	return fmt.Sprintf(`(() => {
  const origin = %s;
  if (window.agentryStart) return;
  if (location.origin !== origin) { location.replace(origin + "/"); return; }
  %s
  if (window.agentryDesktop) return;
  window.agentryDesktop = true;
  const send = (link) => {
    const message = "EE" + JSON.stringify({ name: %s, data: [link] });
    if (window.chrome && window.chrome.webview) window.chrome.webview.postMessage(message);
    else window.webkit.messageHandlers.external.postMessage(message);
  };
  const outside = (link) => { try { return new URL(link).origin !== origin; } catch { return true; } };
  const follow = (event) => {
    const a = event.target instanceof Element && event.target.closest("a[href]");
    if (!a) return;
    if (outside(a.href)) { event.preventDefault(); send(a.href); }
    else if (a.target && a.target !== "_self") { event.preventDefault(); location.assign(a.href); }
  };
  document.addEventListener("click", follow, true);
  document.addEventListener("auxclick", follow, true);
  window.open = (link) => {
    const to = new URL(link || "", location.href).href;
    if (outside(to)) send(to); else location.assign(to);
    return null;
  };
})();`, quoted(origin), zoomScript(zoom), quoted(linkEvent))
}

// reloadScript reloads the page; pageScript sets the zoom again after.
const reloadScript = "location.reload();"

// zoomScript sets the page's zoom.
func zoomScript(zoom float64) string {
	return fmt.Sprintf("document.documentElement.style.zoom = %s;", quoted(fmt.Sprint(zoom)))
}

// zoomLevels are the zooms Zoom in and Zoom out step through, as browsers do.
var zoomLevels = []float64{0.5, 0.67, 0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2, 2.5, 3}

// zoomIn is the next level above zoom, or the largest.
func zoomIn(zoom float64) float64 {
	for _, level := range zoomLevels {
		if level > zoom {
			return level
		}
	}
	return zoomLevels[len(zoomLevels)-1]
}

// zoomOut is the next level below zoom, or the smallest.
func zoomOut(zoom float64) float64 {
	for i := len(zoomLevels) - 1; i >= 0; i-- {
		if zoomLevels[i] < zoom {
			return zoomLevels[i]
		}
	}
	return zoomLevels[0]
}

// links sends a link the page gave up to where launch.Route says: the
// system browser, the window, or nowhere.
type links struct {
	origin  string
	browser func(link string)
	window  func(link string)
}

// follow takes the data of a linkEvent.
func (l links) follow(data ...any) {
	if len(data) != 1 {
		return
	}
	link, ok := data[0].(string)
	if !ok {
		return
	}
	switch launch.Route(l.origin, link) {
	case launch.Browser:
		l.browser(link)
	case launch.Window:
		l.window(link)
	}
}

// goScript is script that opens link in the window.
func goScript(link string) string { return "location.assign(" + quoted(link) + ");" }

func quoted(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// redactedLog is Wails' logger: each message on its own line to w, with the
// token hidden.
type redactedLog struct {
	w     io.Writer
	token func() string
}

func (l redactedLog) write(level, message string) {
	fmt.Fprintln(l.w, launch.Redact(level+message, l.token()))
}

func (l redactedLog) Print(message string)   { l.write("", message) }
func (l redactedLog) Trace(message string)   { l.write("TRACE ", message) }
func (l redactedLog) Debug(message string)   { l.write("DEBUG ", message) }
func (l redactedLog) Info(message string)    { l.write("INFO ", message) }
func (l redactedLog) Warning(message string) { l.write("WARNING ", message) }
func (l redactedLog) Error(message string)   { l.write("ERROR ", message) }
func (l redactedLog) Fatal(message string)   { l.write("FATAL ", message) }
