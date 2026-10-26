// Command agentry-desktop shows Agentry's page in a window of its own. It
// uses the Agentry running from ~/.agentry, or starts the agentry binary
// beside it, and when it quits it stops only a server it started.
//
// The window is Wails v2: build it with `wails build`, or with
// `go build -tags desktop,production` (Linux adds webkit2_41). The files
// without a build tag hold what needs no window, so they test without cgo.
package main

// The Windows exe's icon and manifest, linked in by go build. Run go generate
// again when build/appicon.png changes.
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --icon build/appicon.png --manifest gui --product-name Agentry --file-description Agentry --original-filename agentry-desktop.exe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/browser"

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

// serverPath is the PATH for the server the app starts, and the git it
// runs. On a Mac an app opened from Finder or the Dock gets launchd's PATH,
// /usr/bin:/bin:/usr/sbin:/sbin, without the folders Claude Code and
// Homebrew install to, so there it is the PATH the user's shell sets up,
// as in Terminal; if the shell gives none in time, it stays current.
// Elsewhere it is current: the app gets the desktop session's PATH.
func serverPath(goos, shell, current string) string {
	if goos != "darwin" {
		return current
	}
	if shell == "" {
		shell = "/bin/zsh" // macOS's default shell
	}
	if path, err := shellPath(shell, 5*time.Second); err == nil && path != "" {
		return path
	}
	return current
}

// pathMark comes before the PATH shellPath asks the shell to print, so
// what its startup files print first is skipped.
const pathMark = "agentry-desktop-path:"

// shellPath is the PATH that shell sets up, as a login and interactive
// shell reads both its profile and rc files, within that time.
func shellPath(shell string, within time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-ilc", `printf '%s%s' "`+pathMark+`" "$PATH"`)
	cmd.WaitDelay = time.Second // a process the shell started may keep stdout open
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	i := bytes.LastIndex(out, []byte(pathMark))
	if i < 0 {
		return "", fmt.Errorf("%s printed no PATH", shell)
	}
	return strings.TrimSpace(string(out[i+len(pathMark):])), nil
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

// agentry is the Agentry the window shows. It is found or started once,
// when the start page asks: after Wails' single-instance check, which ends
// a second launch before then, so that launch never starts a server.
type agentry struct {
	launch launch.Options
	token  func() string
	done   chan struct{} // closed once the app quits; launch.Options.Quit

	starting, stopping, quitting sync.Once

	mu           sync.Mutex
	running      *launch.Agentry
	failed       error
	page, origin string // Agentry's signed-in page and its origin; empty until it answers
}

// newAgentry is the Agentry found or started with o, the page token from
// token.
func newAgentry(o launch.Options, token func() string) *agentry {
	done := make(chan struct{})
	o.Quit = done
	return &agentry{launch: o, token: token, done: done}
}

// serve is the app's own pages: the start page, and at startPath the
// answer once Agentry answers.
func (a *agentry) serve(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Cache-Control", "no-store")
	if r.URL.Path != startPath {
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.Write(startPage())
		return
	}
	a.start()
	a.mu.Lock()
	page, failed := a.page, a.failed
	a.mu.Unlock()
	if a.hasQuit() {
		page, failed = "", nil // its server is stopped, or never started
	}
	reply := startReply(page, failed, a.token())
	rw.Header().Set("Content-Type", "application/json")
	rw.Write(reply)
}

// start finds the Agentry running from the data folder, or starts one, the
// first time it is called; later calls wait for that.
func (a *agentry) start() {
	a.starting.Do(func() {
		running, err := launch.Ensure(a.launch)
		a.mu.Lock()
		defer a.mu.Unlock()
		a.running, a.failed = running, err
		if err == nil {
			a.page = launch.PageURL(running.Client)
			a.origin = originOf(a.page)
		}
	})
}

// markQuit says the app quits: no server starts after it, and one a start
// under way waits for is stopped at once.
func (a *agentry) markQuit() {
	a.quitting.Do(func() { close(a.done) })
}

// hasQuit says whether the app has begun to quit.
func (a *agentry) hasQuit() bool {
	select {
	case <-a.done:
		return true
	default:
		return false
	}
}

// stop stops the server if the app started it, once, on every way out. It
// first waits for a start under way, and no server starts after it.
func (a *agentry) stop() {
	a.stopping.Do(func() {
		a.markQuit()
		a.starting.Do(func() {})
		a.mu.Lock()
		running := a.running
		a.mu.Unlock()
		if running != nil {
			running.Stop() // only a server this app started
		}
	})
}

// signedIn is Agentry's signed-in page and its origin, once it answers.
func (a *agentry) signedIn() (page, origin string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.page, a.origin
}

// startURL is where Wails v2 serves the app's own pages on goos.
func startURL(goos string) string {
	if goos == "windows" {
		return "http://wails.localhost/"
	}
	return "wails://wails/"
}

// startPath is the app's own address the start page asks to start Agentry.
const startPath = "/agentry-desktop/start"

var startTemplate = template.Must(template.New("start").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>Agentry</title>
<style>body{font:15px system-ui,sans-serif;margin:0;display:grid;place-items:center;height:100vh;background:#fff;color:#111}
@media (prefers-color-scheme:dark){body{background:#111;color:#eee}}</style></head>
<body><p id="status">Starting Agentry…</p>
<script>
const say = (text) => { document.getElementById("status").textContent = "Agentry didn't start: " + text; };
fetch({{.}}).then((reply) => reply.json()).then((started) => {
  if (started.page) location.replace(started.page); else say(started.failed);
}, say);
</script></body></html>
`))

// startPage is the page the window opens on, served by the app itself. It
// asks the app at startPath to start Agentry, then goes on to Agentry's
// page, signed in, or says why Agentry didn't start.
func startPage() []byte {
	var out bytes.Buffer
	startTemplate.Execute(&out, startPath)
	return out.Bytes()
}

// startReply is the app's answer at startPath: Agentry's page, signed in,
// or why Agentry didn't start, with the token hidden. Neither is set when
// the app quit before the page asked.
func startReply(page string, failed error, token string) []byte {
	var reply struct {
		Page   string `json:"page,omitempty"`
		Failed string `json:"failed,omitempty"`
	}
	switch {
	case failed != nil:
		reply.Failed = launch.Redact(failed.Error(), token)
	case page != "":
		reply.Page = page
	default:
		reply.Failed = "the app is quitting"
	}
	data, _ := json.Marshal(reply)
	return data
}

// bindingsOrigins are the origins whose pages may send the app linkEvent.
// Wails fixes them before the window opens, but Agentry starts after, so
// they are any port of localhost, where a server the app starts serves,
// and running, the origin of the server answering at launch, if any.
func bindingsOrigins(running string) string {
	origins := "http://localhost:*"
	if running != "" {
		origins += "," + running
	}
	return origins
}

// linkEvent is the event the page sends with a link that leaves Agentry.
const linkEvent = "agentry:link"

// pageScript runs in the window after every page load. On Agentry's page it
// sets the zoom and sends links that leave Agentry's origin, clicked or
// opened with window.open or with the middle button, to the app as
// linkEvent; a new window on Agentry's own origin opens in this one. Any
// other page but the app's start page, at start, goes back to Agentry. It
// holds no token.
func pageScript(start, origin string, zoom float64) string {
	return fmt.Sprintf(`(() => {
  const origin = %s;
  if (location.href.startsWith(%s)) return;
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
  document.addEventListener("auxclick", (event) => { if (event.button === 1) follow(event); }, true);
  window.open = (link) => {
    const to = new URL(link || "", location.href).href;
    if (outside(to)) send(to); else location.assign(to);
    return null;
  };
})();`, quoted(origin), quoted(start), zoomScript(zoom), quoted(linkEvent))
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

// wailsRefuses are the characters Wails v2's BrowserOpenURL refuses in a
// link, for fear of a shell: it logs an error and opens nothing, though
// links to GitHub or docs can hold them.
const wailsRefuses = ";|`$\\<>*{}[]()~! \t\n\r"

// opensItself says whether the app opens link in the system browser itself
// instead of through Wails, and the link to open: Go's encoding of it,
// which is what Wails checks and opens. It does so when Wails would refuse
// the link for those characters alone. pkg/browser hands the link to the
// browser as one argument, with no shell. A link with control or other
// unprinted characters is left to Wails, which refuses it.
func opensItself(link string) (string, bool) {
	u, err := url.Parse(link)
	if err != nil {
		return "", false
	}
	encoded := u.String()
	for _, r := range encoded {
		if r <= ' ' || r > '~' {
			return "", false
		}
	}
	return encoded, strings.ContainsAny(encoded, wailsRefuses)
}

// goScript is script that opens link in the window.
func goScript(link string) string { return "location.assign(" + quoted(link) + ");" }

func quoted(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// hideTokenInBrowserOutput sends what the command that opens the system
// browser prints, which can hold the signed-in link, to w with the token
// hidden; it would go to the app's stdout and stderr.
func hideTokenInBrowserOutput(w io.Writer, token func() string) *launch.Redactor {
	output := &launch.Redactor{W: w, Token: token}
	browser.Stdout, browser.Stderr = output, output
	return output
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
