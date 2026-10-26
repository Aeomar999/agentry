package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pkg/browser"

	"gitlab.com/dev.chempa/agentry/internal/launch"
)

const origin = "http://localhost:7800"

func TestTheAgentryBinaryIsNextToTheApp(t *testing.T) {
	exe := filepath.Join("apps", "Agentry", "agentry-desktop")
	if got, want := binaryNextTo(exe, "linux"), filepath.Join("apps", "Agentry", "agentry"); got != want {
		t.Errorf("linux: %s, want %s", got, want)
	}
	if got, want := binaryNextTo(exe+".exe", "windows"), filepath.Join("apps", "Agentry", "agentry.exe"); got != want {
		t.Errorf("windows: %s, want %s", got, want)
	}
}

// The app uses `agentry open`'s data folder and default repo, on this
// computer only, from port 7800.
func TestLaunchOptions(t *testing.T) {
	home := filepath.Join("home", "me")
	o := launchOptions(home, filepath.Join("bin", "agentry-desktop"), "linux", nil)
	want := launch.Options{DataDir: filepath.Join(home, ".agentry"), Repo: filepath.Join(home, "Agentry"), Binary: filepath.Join("bin", "agentry"), Host: "127.0.0.1", Port: 7800}
	if o != want {
		t.Errorf("got %+v, want %+v", o, want)
	}
}

// The start page asks the app to start Agentry and holds no token.
func TestStartPageAsksTheAppToStartAgentry(t *testing.T) {
	page := string(startPage())
	if !strings.Contains(page, `fetch("/agentry-desktop/start")`) || !strings.Contains(page, "Starting Agentry…") || strings.Contains(page, "token") {
		t.Errorf("page:\n%s", page)
	}
}

// startHarness runs the start page's script in node with the app's reply
// to its fetch, and reports where it went and what it said.
const startHarness = `
const asked = [], replaced = [], status = { textContent: "" };
global.document = { getElementById: () => status };
global.location = { replace: (u) => replaced.push(u) };
global.fetch = async (path) => { asked.push(path); return { json: async () => JSON.parse(process.argv[2]) }; };
eval(process.argv[1]);
setTimeout(() => console.log(JSON.stringify({ asked, replaced, said: status.textContent })), 10);
`

func runStartPage(t *testing.T, reply []byte) (run struct {
	Asked, Replaced []string
	Said            string
}) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node isn't installed")
	}
	page := string(startPage())
	script := page[strings.Index(page, "<script>")+len("<script>") : strings.Index(page, "</script>")]
	out, err := exec.Command(node, "-e", startHarness, script, string(reply)).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if err := json.Unmarshal(out, &run); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return run
}

// The start page goes on to the page the app replies with, or says why
// Agentry didn't start.
func TestStartPageGoesWhereTheAppSays(t *testing.T) {
	run := runStartPage(t, startReply(origin+"/?token=abc", nil, "abc"))
	if !slices.Equal(run.Asked, []string{startPath}) || !slices.Equal(run.Replaced, []string{origin + "/?token=abc"}) || run.Said != "" {
		t.Errorf("started: %+v", run)
	}
	run = runStartPage(t, startReply("", errors.New("the server didn't start"), ""))
	if len(run.Replaced) != 0 || run.Said != "Agentry didn't start: the server didn't start" {
		t.Errorf("failed: %+v", run)
	}
}

func TestStartReplyGoesOnToAgentry(t *testing.T) {
	if reply := string(startReply(origin+"/?token=abc", nil, "abc")); reply != `{"page":"http://localhost:7800/?token=abc"}` {
		t.Errorf("reply %s", reply)
	}
}

// An error that holds the signed-in link shows with the token hidden.
func TestStartReplyHidesTheTokenInAnError(t *testing.T) {
	reply := string(startReply("", errors.New("GET "+origin+"/?token=s3cret: refused; s3cret"), "s3cret"))
	if want := `{"failed":"GET http://localhost:7800/?token=` + launch.Hidden + `: refused; ` + launch.Hidden + `"}`; reply != want {
		t.Errorf("reply %s, want %s", reply, want)
	}
}

// launchNowhere are launch options whose server can't start: no binary.
func launchNowhere(t *testing.T) launch.Options {
	dir := t.TempDir()
	return launch.Options{DataDir: filepath.Join(dir, "data"), Repo: filepath.Join(dir, "repo"), Binary: filepath.Join(dir, "no-agentry"), Port: 7800}
}

// The start page is served for any path, and Agentry starts only when the
// page asks at startPath, with the reason it didn't start, token hidden.
func TestTheAppStartsAgentryWhenTheStartPageAsks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	o := launchNowhere(t)
	a := &agentry{launch: o, token: func() string { return "s3cret" }}
	page := httptest.NewRecorder()
	a.serve(page, httptest.NewRequest("GET", "/", nil))
	if !bytes.Equal(page.Body.Bytes(), startPage()) {
		t.Errorf("start page %s", page.Body)
	}
	if _, err := os.Stat(o.Repo); err == nil {
		t.Fatal("Agentry started before the start page asked")
	}
	reply := httptest.NewRecorder()
	a.serve(reply, httptest.NewRequest("GET", startPath, nil))
	if body := reply.Body.String(); !strings.HasPrefix(body, `{"failed":`) || !strings.Contains(body, "no-agentry") || reply.Header().Get("Content-Type") != "application/json" {
		t.Errorf("reply %s", body)
	}
	if page, origin := a.signedIn(); page != "" || origin != "" {
		t.Errorf("signed in at %q %q", page, origin)
	}
	a.stop()
	a.stop()
}

// A quit before the start page asks starts no server, then or later.
func TestStopBeforeStartStartsNothing(t *testing.T) {
	o := launchNowhere(t)
	a := &agentry{launch: o, token: func() string { return "" }}
	a.stop()
	reply := httptest.NewRecorder()
	a.serve(reply, httptest.NewRequest("GET", startPath, nil))
	if _, err := os.Stat(o.Repo); err == nil {
		t.Error("Agentry started after the app stopped")
	}
	if body := reply.Body.String(); body != "{}" {
		t.Errorf("reply %s", body)
	}
}

func TestBindingsOrigins(t *testing.T) {
	if got := bindingsOrigins(""); got != "http://localhost:*" {
		t.Errorf("none running: %s", got)
	}
	if got := bindingsOrigins("https://studio.example.com:7800"); got != "http://localhost:*,https://studio.example.com:7800" {
		t.Errorf("running: %s", got)
	}
}

// What the command that opens the browser prints goes through the
// redactor, not to the app's stdout and stderr.
func TestBrowserOutputHidesTheToken(t *testing.T) {
	stdout, stderr := browser.Stdout, browser.Stderr
	t.Cleanup(func() { browser.Stdout, browser.Stderr = stdout, stderr })
	var out bytes.Buffer
	output := hideTokenInBrowserOutput(&out, func() string { return "s3cret" })
	if browser.Stdout != output || browser.Stderr != output {
		t.Fatalf("browser output goes to %v and %v", browser.Stdout, browser.Stderr)
	}
	io.WriteString(browser.Stderr, "xdg-open: no method available for opening '"+origin+"/?token=s3cret'\n")
	if got, want := out.String(), "xdg-open: no method available for opening '"+origin+"/?token="+launch.Hidden+"'\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestZoomSteps(t *testing.T) {
	for _, c := range []struct {
		step     func(float64) float64
		from, to float64
	}{
		{zoomIn, 1, 1.1}, {zoomIn, 1.05, 1.1}, {zoomIn, 3, 3},
		{zoomOut, 1, 0.9}, {zoomOut, 1.05, 1}, {zoomOut, 0.5, 0.5},
	} {
		if got := c.step(c.from); got != c.to {
			t.Errorf("from %v: got %v, want %v", c.from, got, c.to)
		}
	}
}

// A link the page gives up goes where launch.Route says, and nothing else
// does anything.
func TestLinksFollowTheRoute(t *testing.T) {
	var browser, window []string
	l := links{origin: origin, browser: func(link string) { browser = append(browser, link) }, window: func(link string) { window = append(window, link) }}
	l.follow("https://github.com/francischempa/agentry")
	l.follow("mailto:someone@example.com")
	l.follow(origin + "/tasks/task-3")
	l.follow("file:///etc/passwd")
	l.follow(42)
	l.follow("a", "b")
	l.follow()
	if want := []string{"https://github.com/francischempa/agentry", "mailto:someone@example.com"}; !slices.Equal(browser, want) {
		t.Errorf("browser %q, want %q", browser, want)
	}
	if want := []string{origin + "/tasks/task-3"}; !slices.Equal(window, want) {
		t.Errorf("window %q, want %q", window, want)
	}
}

func TestLogHidesTheToken(t *testing.T) {
	var out bytes.Buffer
	log := redactedLog{w: &out, token: func() string { return "s3cret" }}
	log.Error("navigation to " + origin + "/?token=s3cret failed")
	log.Info("the token s3cret")
	if strings.Contains(out.String(), "s3cret") || out.String() != "ERROR navigation to "+origin+"/?token="+launch.Hidden+" failed\nINFO the token "+launch.Hidden+"\n" {
		t.Errorf("logged %q", out.String())
	}
}

func TestTokenIn(t *testing.T) {
	dir := t.TempDir()
	if token := tokenIn(dir); token != "" {
		t.Errorf("before the server made it: %q", token)
	}
	os.WriteFile(filepath.Join(dir, "ui-token"), []byte("abc\n"), 0o600)
	if token := tokenIn(dir); token != "abc" {
		t.Errorf("got %q", token)
	}
}

func TestOriginOf(t *testing.T) {
	if got := originOf(origin + "/?token=abc"); got != origin {
		t.Errorf("got %s", got)
	}
}

// harness runs pageScript in node, with just enough of a page around it,
// and reports what the script did.
const harness = `
const sent = [], assigned = [], replaced = [], listeners = {};
global.Element = class {};
class A extends Element {
  constructor(href, target) { super(); this.href = new URL(href, location.href).href; this.target = target || ""; }
  closest() { return this; }
}
global.window = global;
global.location = { origin: process.argv[1], href: process.argv[2], assign: (u) => assigned.push(u), replace: (u) => replaced.push(u) };
global.document = { documentElement: { style: {} }, addEventListener: (type, f) => (listeners[type] = f) };
global.chrome = { webview: { postMessage: (m) => sent.push(JSON.parse(m.slice(2))) } };
window.agentryStart = true; // a page can set any global; the script mustn't trust one
eval(process.argv[3]);
const click = (href, target, type = "click", button = 0) => {
  let prevented = false;
  if (listeners[type]) listeners[type]({ target: new A(href, target), button, preventDefault: () => (prevented = true) });
  return prevented;
};
const prevented = [
  click("https://github.com/francischempa/agentry", "_blank"),
  click("/tasks/task-3"),
  click("/tasks/task-3/workspace", "_blank"),
  click("mailto:someone@example.com", "", "auxclick", 1),
  click("https://elsewhere.example.com/menu", "", "auxclick", 2),
];
if (window.open !== undefined && listeners.click) { window.open("https://docs.example.com/"); window.open("/tasks/task-4", "_blank"); }
console.log(JSON.stringify({ zoom: document.documentElement.style.zoom, sent, assigned, replaced, prevented }));
`

type pageRun struct {
	Zoom string
	Sent []struct {
		Name string
		Data []string
	}
	Assigned  []string
	Replaced  []string
	Prevented []bool
}

func runPage(t *testing.T, at string, zoom float64) pageRun {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node isn't installed")
	}
	out, err := exec.Command(node, "-e", harness, originOf(at), at, pageScript(startURL("linux"), origin, zoom)).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	var run pageRun
	if err := json.Unmarshal(out, &run); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return run
}

// On Agentry's page, links that leave it go to the app, never followed in
// the window; Agentry's own links stay, and a new window on Agentry's
// origin opens in this one. A right click is left to the page. The zoom is
// set.
func TestPageScriptSendsOutsideLinksToTheApp(t *testing.T) {
	run := runPage(t, origin+"/tasks", 1.25)
	var sent []string
	for _, s := range run.Sent {
		if s.Name != linkEvent || len(s.Data) != 1 {
			t.Errorf("sent %+v", s)
			continue
		}
		sent = append(sent, s.Data[0])
	}
	if want := []string{"https://github.com/francischempa/agentry", "mailto:someone@example.com", "https://docs.example.com/"}; !slices.Equal(sent, want) {
		t.Errorf("sent %q, want %q", sent, want)
	}
	if want := []string{origin + "/tasks/task-3/workspace", origin + "/tasks/task-4"}; !slices.Equal(run.Assigned, want) {
		t.Errorf("opened in the window %q, want %q", run.Assigned, want)
	}
	if want := []bool{true, false, true, true, false}; !slices.Equal(run.Prevented, want) {
		t.Errorf("prevented %v, want %v", run.Prevented, want)
	}
	if run.Zoom != "1.25" || len(run.Replaced) != 0 {
		t.Errorf("zoom %q, replaced %q", run.Zoom, run.Replaced)
	}
}

// A page from anywhere else that got into the window goes back to Agentry,
// without the token, and the script does nothing more there, even when
// the page set the global the start page once did.
func TestPageScriptSendsOtherPagesBackToAgentry(t *testing.T) {
	run := runPage(t, "https://elsewhere.example.com/x", 1)
	if want := []string{origin + "/"}; !slices.Equal(run.Replaced, want) || len(run.Sent) != 0 || run.Zoom != "" {
		t.Errorf("got %+v", run)
	}
}

// The app's start page, on its own origin, is left to go on by itself.
func TestPageScriptLeavesTheStartPageAlone(t *testing.T) {
	run := runPage(t, startURL("linux"), 1)
	if len(run.Replaced) != 0 || len(run.Sent) != 0 || len(run.Assigned) != 0 || run.Zoom != "" {
		t.Errorf("got %+v", run)
	}
}

func TestStartURL(t *testing.T) {
	if startURL("windows") != "http://wails.localhost/" || startURL("linux") != "wails://wails/" || startURL("darwin") != "wails://wails/" {
		t.Error(startURL("windows"), startURL("linux"), startURL("darwin"))
	}
}

func TestScriptsHoldNoToken(t *testing.T) {
	for _, script := range []string{pageScript(startURL("linux"), origin, 1), zoomScript(1.5), reloadScript, goScript(origin + "/tasks")} {
		if strings.Contains(script, "token") {
			t.Errorf("script names a token: %s", script)
		}
	}
}
