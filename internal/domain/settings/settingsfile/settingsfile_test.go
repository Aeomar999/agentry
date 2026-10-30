package settingsfile

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const secret = "s3cr3t-do-not-log-7f2c"

const good = `
[ideas]
host = "gitlab.com"

[ideas.secrets]
token = "` + secret + `"

[other]
[other.secrets]
hook = "other-secret"
`

// A missing file is no settings, and nothing to report.
func TestAMissingFileIsNoSettings(t *testing.T) {
	logged := captureLog(t)
	live := Open(filepath.Join(t.TempDir(), "features.toml"))
	if tables := live.Tables(); len(tables) != 0 {
		t.Errorf("tables %v, want none", tables)
	}
	if _, found := live.For("ideas").Secret("token"); found {
		t.Error("a missing file holds a secret")
	}
	if said := logged.String(); said != "" {
		t.Errorf("a missing file was reported:\n%s", said)
	}
}

// An extension reads its own values and secrets, not another extension's.
func TestAnExtensionReadsOnlyItsOwnTable(t *testing.T) {
	path := write(t, filepath.Join(t.TempDir(), "features.toml"), good)
	ideas := Open(path).For("ideas")
	if host, _ := ideas.Setting("host"); host != "gitlab.com" {
		t.Errorf("host %q", host)
	}
	if token, _ := ideas.Secret("token"); token != secret {
		t.Errorf("token %q", token)
	}
	if hook, found := ideas.Secret("hook"); found {
		t.Errorf("ideas read other's secret %q", hook)
	}
}

// An edit to the file reaches settings already handed out, with no restart.
func TestAnEditReachesSettingsHandedOutBefore(t *testing.T) {
	captureLog(t)
	path := write(t, filepath.Join(t.TempDir(), "features.toml"), good)
	live := Open(path)
	ideas := live.For("ideas")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go live.Watch(ctx, 5*time.Millisecond)
	write(t, path, strings.Replace(good, `host = "gitlab.com"`, `host = "example.org"`, 1))
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if host, _ := ideas.Setting("host"); host == "example.org" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the edit never reached the settings")
		}
	}
	os.Remove(path)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, found := ideas.Secret("token"); !found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("removing the file kept its secret")
		}
	}
}

// A file that goes missing takes its secrets with it, and that is said once,
// naming only the file.
func TestARemovedFileIsSaidOnce(t *testing.T) {
	logged := captureLog(t)
	path := write(t, filepath.Join(t.TempDir(), "features.toml"), good)
	live := Open(path)
	logged.Reset()
	os.Remove(path)
	live.reload()
	live.reload()
	if tables := live.Tables(); len(tables) != 0 {
		t.Errorf("tables %v, want none", tables)
	}
	said := logged.String()
	if strings.Count(said, "the settings file is gone") != 1 || !strings.Contains(said, path) {
		t.Errorf("it logged\n%s\nwant one line saying the file is gone", said)
	}
	if strings.Contains(said, secret) {
		t.Errorf("the log quotes the file:\n%s", said)
	}
	logged.Reset()
	write(t, path, good)
	live.reload()
	if token, _ := live.For("ideas").Secret("token"); token != secret {
		t.Errorf("putting the file back gave token %q", token)
	}
	if said := logged.String(); !strings.Contains(said, "settings file loaded") {
		t.Errorf("putting the file back wasn't said:\n%s", said)
	}
}

// A version that doesn't load leaves the one before it in force and is
// reported once, naming the file and where, never what the file holds.
func TestABrokenFileKeepsTheLastGoodVersionAndIsNotQuoted(t *testing.T) {
	logged := captureLog(t)
	path := write(t, filepath.Join(t.TempDir(), "features.toml"), good)
	live := Open(path)
	cases := []struct{ text, want string }{
		{good + "\ntoken = \"" + secret + "\" oops\n", "line 12, column 34"},
		{good + "\n[" + secret + "\n", "line 12, column 24"},
		{good + "\n[ideas]\nhost = \"" + secret + "\"\n", "it isn't valid TOML"}, // a table defined twice: go-toml gives no position
		{strings.Replace(good, `host = "gitlab.com"`, `host = 3`, 1), "a setting in [ideas] should be a string"},
		{strings.Replace(good, "[ideas.secrets]\ntoken", "[ideas.secrets.token]\nvalue", 1), "a secret in [ideas.secrets] should be a string"},
		{"[ideas]\nsecrets = \"" + secret + "\"\n", "[ideas.secrets] should be a table"},
		// a token pasted where a name goes
		{strings.Replace(good, `host = "gitlab.com"`, secret+` = 3`, 1), "a setting in [ideas] should be a string"},
		{strings.Replace(good, "token = ", secret+" = 3\n#", 1), "a secret in [ideas.secrets] should be a string"},
		{good + "\n[ideas." + secret + "]\nhost = \"x\"\n", "a setting in [ideas] should be a string"},
		{secret + " = \"x\"\n" + good, "the top of the file should hold only tables"},
	}
	for _, c := range cases {
		logged.Reset()
		write(t, path, c.text)
		live.reload()
		live.reload() // the same version again: said once
		said := logged.String()
		if strings.Count(said, "doesn't load; the last version that loaded stays in force") != 1 || !strings.Contains(said, c.want) || !strings.Contains(said, path) {
			t.Errorf("for\n%s\nit logged\n%s\nwant one line naming the file and %q", c.text, said, c.want)
		}
		if strings.Contains(said, secret) {
			t.Errorf("the log quotes the file:\n%s", said)
		}
		if token, _ := live.For("ideas").Secret("token"); token != secret {
			t.Errorf("the last good version isn't in force: token %q", token)
		}
	}
}

// A file that doesn't load at the start is no settings, and is reported as
// that, not as a last good version staying in force.
func TestABrokenFileAtTheStartIsNoSettings(t *testing.T) {
	logged := captureLog(t)
	path := write(t, filepath.Join(t.TempDir(), "features.toml"), "[ideas\n")
	live := Open(path)
	if tables := live.Tables(); len(tables) != 0 {
		t.Errorf("tables %v, want none", tables)
	}
	said := logged.String()
	if !strings.Contains(said, "doesn't load; features have no settings until it is fixed") || strings.Contains(said, "stays in force") {
		t.Errorf("it logged\n%s\nwant the file doesn't load and features have no settings until it is fixed", said)
	}
}

// A file that can't be read at the start, here a folder in its place, is no
// settings, and the log says so.
func TestAnUnreadableFileAtTheStartIsNoSettings(t *testing.T) {
	logged := captureLog(t)
	path := filepath.Join(t.TempDir(), "features.toml")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	live := Open(path)
	if tables := live.Tables(); len(tables) != 0 {
		t.Errorf("tables %v, want none", tables)
	}
	said := logged.String()
	if !strings.Contains(said, "can't be read; features have no settings until it is fixed") || strings.Contains(said, "stays in force") {
		t.Errorf("it logged\n%s\nwant the file can't be read and features have no settings until it is fixed", said)
	}
}

// A file that comes back broken after it was gone is no settings too.
func TestABrokenFileAfterItWasGoneIsNoSettings(t *testing.T) {
	logged := captureLog(t)
	path := write(t, filepath.Join(t.TempDir(), "features.toml"), good)
	live := Open(path)
	os.Remove(path)
	live.reload()
	logged.Reset()
	write(t, path, "[ideas\n")
	live.reload()
	said := logged.String()
	if !strings.Contains(said, "features have no settings until it is fixed") || strings.Contains(said, "stays in force") {
		t.Errorf("it logged\n%s\nwant features have no settings until it is fixed", said)
	}
}

// A header that isn't an extension's name, such as a token pasted as one, is
// never named in the log: the problem says only that a table is wrong.
func TestAHeaderThatIsntAnExtensionNameIsNotLogged(t *testing.T) {
	logged := captureLog(t)
	path := write(t, filepath.Join(t.TempDir(), "features.toml"), good)
	live := Open(path)
	const longWord = "abcdefghijklmnopqrstuvwxyzabcdefg" // the shape of a name, but too long for one
	cases := []struct{ text, header, want string }{
		{"[TOKEN]\nx = 3\n", "TOKEN", "a setting in a table in the file should be a string"},
		{"[TOKEN.secrets]\nx = 3\n", "TOKEN", "a secret in a secrets table in the file should be a string"},
		{"[TOKEN]\nsecrets = \"x\"\n", "TOKEN", "a secrets table in the file should be a table"},
		{"[" + secret + "]\nx = 3\n", secret, "a setting in a table in the file should be a string"},
		{"[" + longWord + "]\nx = 3\n", longWord, "a setting in a table in the file should be a string"},
		{"[git-host]\nx = 3\n", "", "a setting in [git-host] should be a string"},
	}
	for _, c := range cases {
		logged.Reset()
		write(t, path, c.text)
		live.reload()
		said := logged.String()
		if !strings.Contains(said, c.want) {
			t.Errorf("for\n%s\nit logged\n%s\nwant %q", c.text, said, c.want)
		}
		if c.header != "" && strings.Contains(said, c.header) {
			t.Errorf("the log names the header %q:\n%s", c.header, said)
		}
	}
}

func write(t *testing.T, path, text string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// captureLog sends slog's default logger to a buffer until the test ends.
func captureLog(t *testing.T) *syncBuffer {
	logged := &syncBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return logged
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Reset()
}
