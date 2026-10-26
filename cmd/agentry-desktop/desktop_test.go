package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

func TestStartPageGoesOnToAgentry(t *testing.T) {
	page := string(startPage(origin+"/?token=abc", nil, "abc"))
	if !strings.Contains(page, `location.replace("http://localhost:7800/?token=abc")`) {
		t.Errorf("no way on to the page:\n%s", page)
	}
}

// An error that holds the signed-in link shows with the token hidden.
func TestStartPageHidesTheTokenInAnError(t *testing.T) {
	page := string(startPage("", errors.New("GET "+origin+"/?token=s3cret: refused; s3cret"), "s3cret"))
	if strings.Contains(page, "s3cret") || !strings.Contains(page, "Agentry didn't start: GET "+origin+"/?token="+launch.Hidden) || strings.Contains(page, "location.replace") {
		t.Errorf("page:\n%s", page)
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
eval(process.argv[3]);
const click = (href, target, type = "click") => {
  let prevented = false;
  if (listeners[type]) listeners[type]({ target: new A(href, target), preventDefault: () => (prevented = true) });
  return prevented;
};
const prevented = [
  click("https://github.com/francischempa/agentry", "_blank"),
  click("/tasks/task-3"),
  click("/tasks/task-3/workspace", "_blank"),
  click("mailto:someone@example.com", "", "auxclick"),
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
	out, err := exec.Command(node, "-e", harness, originOf(at), at, pageScript(origin, zoom)).CombinedOutput()
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
// origin opens in this one. The zoom is set.
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
	if want := []bool{true, false, true, true}; !slices.Equal(run.Prevented, want) {
		t.Errorf("prevented %v, want %v", run.Prevented, want)
	}
	if run.Zoom != "1.25" || len(run.Replaced) != 0 {
		t.Errorf("zoom %q, replaced %q", run.Zoom, run.Replaced)
	}
}

// A page from anywhere else that got into the window goes back to Agentry,
// without the token, and the script does nothing more there.
func TestPageScriptSendsOtherPagesBackToAgentry(t *testing.T) {
	run := runPage(t, "https://elsewhere.example.com/x", 1)
	if want := []string{origin + "/"}; !slices.Equal(run.Replaced, want) || len(run.Sent) != 0 || run.Zoom != "" {
		t.Errorf("got %+v", run)
	}
}

func TestScriptsHoldNoToken(t *testing.T) {
	for _, script := range []string{pageScript(origin, 1), zoomScript(1.5), reloadScript, goScript(origin + "/tasks")} {
		if strings.Contains(script, "token") {
			t.Errorf("script names a token: %s", script)
		}
	}
}
