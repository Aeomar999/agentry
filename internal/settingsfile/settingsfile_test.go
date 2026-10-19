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

// A feature reads its own values and secrets, not another feature's.
func TestAFeatureReadsOnlyItsOwnTable(t *testing.T) {
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
		if strings.Count(said, "doesn't load") != 1 || !strings.Contains(said, c.want) || !strings.Contains(said, path) {
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

// A file that doesn't load at the start is no settings, and is reported.
func TestABrokenFileAtTheStartIsNoSettings(t *testing.T) {
	logged := captureLog(t)
	path := write(t, filepath.Join(t.TempDir(), "features.toml"), "[ideas\n")
	live := Open(path)
	if tables := live.Tables(); len(tables) != 0 {
		t.Errorf("tables %v, want none", tables)
	}
	if said := logged.String(); !strings.Contains(said, "doesn't load") {
		t.Errorf("it wasn't reported:\n%s", said)
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
