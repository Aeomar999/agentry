package launch

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/procs"
)

// fakeServe, set in the environment, makes the test binary act as
// `agentry serve`: "serve" answers like one, "fail" exits at once,
// "grandchild" serves after starting a process that shares its output
// ("hold", which sleeps), and "lose" never serves, as one that lost a race.
const fakeServe = "AGENTRY_LAUNCH_FAKE_SERVE"

const fakeToken = "s3cr/et+1"

func TestMain(m *testing.M) {
	switch os.Getenv(fakeServe) {
	case "serve":
		serveFake(os.Args[2:])
	case "grandchild":
		serveFake(os.Args[2:])
	case "fail":
		fmt.Println("agentry: the port is taken")
		os.Exit(1)
	case "hold":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "lose":
		loseFake(os.Args[2:])
	}
	os.Exit(m.Run())
}

// serveFake answers /api/config at -addr, having saved server.json and
// ui-token in -data as serve does, and prints serve's signed-in link.
func serveFake(args []string) {
	address, dataDir := fakeFlags(args)
	if os.Getenv(fakeServe) == "grandchild" {
		self, _ := os.Executable()
		grandchild := exec.Command(self)
		grandchild.Env = append(os.Environ(), fakeServe+"=hold")
		grandchild.Stdout, grandchild.Stderr = os.Stdout, os.Stderr
		grandchild.Start()
		os.WriteFile(filepath.Join(dataDir, "grandchild.pid"), []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	page := "http://" + listener.Addr().String()
	os.WriteFile(filepath.Join(dataDir, LockFile), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
	os.WriteFile(filepath.Join(dataDir, "ui-token"), []byte(fakeToken+"\n"), 0o600)
	client.Save(dataDir, client.Where{URL: page, Page: page, Listen: listener.Addr().String()})
	fmt.Printf("Agentry is running. Open %s/?token=%s\n", page, fakeToken)
	http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{}"))
	}))
}

// loseFake says, in the data folder's "lost" file, its pid and the address
// it was given, then waits to be stopped without serving.
func loseFake(args []string) {
	address, dataDir := fakeFlags(args)
	os.WriteFile(filepath.Join(dataDir, "lost"), []byte(fmt.Sprintf("%d %s", os.Getpid(), address)), 0o600)
	time.Sleep(time.Minute)
	os.Exit(1)
}

func fakeFlags(args []string) (address, dataDir string) {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	flags.StringVar(&address, "addr", "", "")
	flags.StringVar(&dataDir, "data", "", "")
	flags.String("repo", "", "")
	flags.Parse(args)
	return address, dataDir
}

// A server already answering from the data folder is used, not started
// again, and stopping leaves it running.
func TestEnsureReusesARunningServerAndLeavesItRunning(t *testing.T) {
	running := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{}"))
	}))
	t.Cleanup(running.Close)
	dataDir := t.TempDir()
	client.Save(dataDir, client.Where{URL: running.URL, Page: running.URL})
	os.WriteFile(filepath.Join(dataDir, "ui-token"), []byte("abc\n"), 0o600)

	agentry, err := Ensure(Options{DataDir: dataDir, Repo: t.TempDir(), Agentry: filepath.Join(t.TempDir(), "no-agentry"), Host: "127.0.0.1", Port: 7930})
	if err != nil {
		t.Fatal(err)
	}
	if agentry.Started() || agentry.URL != running.URL || agentry.Token != "abc" {
		t.Errorf("got %+v, started %v; want the running server, not started", agentry.Client, agentry.Started())
	}
	agentry.Stop()
	if _, ok := Running(dataDir); !ok {
		t.Error("Stop stopped a server this program didn't start")
	}
}

// With none answering, Ensure starts `agentry serve` as its child on this
// computer only, and Stop stops it. The token stays out of its output.
func TestEnsureStartsAServerAndStopsOnlyThatOne(t *testing.T) {
	t.Setenv(fakeServe, "serve")
	self, _ := os.Executable()
	dataDir, repo := t.TempDir(), filepath.Join(t.TempDir(), "Agentry")
	var output bytes.Buffer
	shown := &Redactor{W: &output, Token: func() string { return readToken(dataDir) }}

	agentry, err := Ensure(Options{DataDir: dataDir, Repo: repo, Agentry: self, Host: "127.0.0.1", Port: 7930, Output: shown})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(agentry.Stop)
	if !agentry.Started() || agentry.Token != fakeToken || !strings.HasPrefix(agentry.URL, "http://127.0.0.1:") {
		t.Errorf("got %+v, started %v; want a started server on 127.0.0.1", agentry.Client, agentry.Started())
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		t.Errorf("the default repo wasn't made: %v", err)
	}
	agentry.Stop()
	select {
	case <-agentry.child.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the server is still running after Stop")
	}
	if _, ok := Running(dataDir); ok {
		t.Error("the server still answers after Stop")
	}
	shown.Close()
	if !strings.Contains(output.String(), "Agentry is running. Open http://127.0.0.1:") || strings.Contains(output.String(), "s3cr") {
		t.Errorf("output %q: want serve's line with no token", output.String())
	}
}

// With no Host, the server Ensure starts listens on this computer only.
func TestEnsureListensOnThisComputerByDefault(t *testing.T) {
	t.Setenv(fakeServe, "serve")
	self, _ := os.Executable()
	agentry, err := Ensure(Options{DataDir: t.TempDir(), Repo: t.TempDir(), Agentry: self, Port: 7930})
	if err != nil {
		t.Fatal(err)
	}
	defer agentry.Stop()
	if !strings.HasPrefix(agentry.Listen, "127.0.0.1:") {
		t.Errorf("listens on %s, want 127.0.0.1", agentry.Listen)
	}
}

// Stop returns although a process the server started still holds its output.
func TestStopDoesNotWaitForAProcessHoldingTheOutput(t *testing.T) {
	t.Setenv(fakeServe, "grandchild")
	self, _ := os.Executable()
	dataDir := t.TempDir()
	agentry, err := Ensure(Options{DataDir: dataDir, Repo: t.TempDir(), Agentry: self, Host: "127.0.0.1", Port: 7930, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		saved, _ := os.ReadFile(filepath.Join(dataDir, "grandchild.pid"))
		if pid, _ := strconv.Atoi(string(saved)); pid > 0 {
			if process, err := os.FindProcess(pid); err == nil {
				process.Kill()
				process.Wait()
			}
		}
	})
	stopped := make(chan struct{})
	go func() {
		agentry.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(8 * time.Second):
		t.Fatal("Stop is still waiting for the output of a process the server started")
	}
}

// When another launch's server answers first, on another port or on the
// very port ours was given, Ensure uses it as one it found running, and
// stops its own child.
func TestEnsureUsesAServerAnotherLaunchStarted(t *testing.T) {
	t.Setenv(fakeServe, "lose")
	self, _ := os.Executable()
	for _, samePort := range []bool{false, true} {
		dataDir := t.TempDir()
		winner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("{}"))
		}))
		childPid := make(chan int, 1)
		go func() { // the other launch's server comes up once our child has started
			for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
				lost, err := os.ReadFile(filepath.Join(dataDir, "lost"))
				if err != nil {
					continue
				}
				var pid int
				var address string
				fmt.Sscanf(string(lost), "%d %s", &pid, &address)
				childPid <- pid
				listen := strings.TrimPrefix(winner.URL, "http://")
				if samePort {
					listen = address
				}
				os.WriteFile(filepath.Join(dataDir, LockFile), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
				os.WriteFile(filepath.Join(dataDir, "ui-token"), []byte("abc\n"), 0o600)
				client.Save(dataDir, client.Where{URL: winner.URL, Page: winner.URL, Listen: listen})
				return
			}
		}()

		agentry, err := Ensure(Options{DataDir: dataDir, Repo: t.TempDir(), Agentry: self, Host: "127.0.0.1", Port: 7930})
		winner.Close()
		if err != nil {
			t.Fatalf("same port %v: %v", samePort, err)
		}
		if agentry.Started() || agentry.URL != winner.URL {
			t.Errorf("same port %v: got %s, started %v; want the other launch's server, not started", samePort, agentry.URL, agentry.Started())
		}
		if pid := <-childPid; procs.Alive(pid) {
			t.Errorf("same port %v: our child (pid %d) wasn't stopped", samePort, pid)
		}
	}
}

// A wildcard listen address is one, however serve saves it.
func TestSameAddress(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"127.0.0.1:7800", "127.0.0.1:7800", true},
		{"[::]:7800", "0.0.0.0:7800", true},
		{"127.0.0.1:7800", "127.0.0.1:7801", false},
		{"127.0.0.1:7800", "0.0.0.0:7800", false},
		{"", "127.0.0.1:7800", false},
	} {
		if got := sameAddress(c.a, c.b); got != c.want {
			t.Errorf("sameAddress(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// A server that exits at once fails Ensure without waiting out its 30 seconds.
func TestEnsureSaysWhenTheServerDidNotStart(t *testing.T) {
	t.Setenv(fakeServe, "fail")
	self, _ := os.Executable()
	began := time.Now()
	if _, err := Ensure(Options{DataDir: t.TempDir(), Repo: t.TempDir(), Agentry: self, Host: "127.0.0.1", Port: 7930}); err == nil {
		t.Fatal("no error from a server that exited")
	}
	if waited := time.Since(began); waited > 10*time.Second {
		t.Errorf("waited %v for a server that had exited", waited)
	}
}

func readToken(dataDir string) string {
	token, _ := os.ReadFile(filepath.Join(dataDir, "ui-token"))
	return strings.TrimSpace(string(token))
}

// The signed-in link is at localhost for a loopback server, https when
// served over it, and the server's own address otherwise.
func TestPageURL(t *testing.T) {
	for _, c := range []struct {
		where client.Where
		want  string
	}{
		{client.Where{URL: "http://127.0.0.1:7800"}, "http://localhost:7800/?token=abc"},
		{client.Where{URL: "http://127.0.0.1:7800", HTTPS: true}, "https://localhost:7800/?token=abc"},
		{client.Where{URL: "http://192.168.1.4:7800/"}, "http://192.168.1.4:7800/?token=abc"},
	} {
		if got := PageURL(client.Client{Where: c.where, Token: "abc"}); got != c.want {
			t.Errorf("%+v: got %s, want %s", c.where, got, c.want)
		}
	}
}

// FreeAddress skips a port in use.
func TestFreeAddressSkipsATakenPort(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:7931")
	if err != nil {
		t.Skip("port 7931 is busy:", err)
	}
	defer taken.Close()
	if address, err := FreeAddress("127.0.0.1", 7931); err != nil || address == "127.0.0.1:7931" {
		t.Errorf("got %q, %v; want a port after 7931", address, err)
	}
}

func TestRoute(t *testing.T) {
	const origin = "http://localhost:7800"
	for link, want := range map[string]Destination{
		"http://localhost:7800/tasks/task-3":       Window,
		"HTTP://LOCALHOST:7800/":                   Window,
		"http://localhost:7801/":                   Browser,
		"http://127.0.0.1:7800/":                   Browser,
		"https://localhost:7800/":                  Browser,
		"https://github.com/francischempa/agentry": Browser,
		"mailto:someone@example.com":               Browser,
		"file:///etc/passwd":                       Nowhere,
		"javascript:alert(1)":                      Nowhere,
		"/tasks":                                   Nowhere,
		"http://%zz":                               Nowhere,
	} {
		if got := Route(origin, link); got != want {
			t.Errorf("%s: got %d, want %d", link, got, want)
		}
	}
	if Route("https://agentry.example.com", "https://agentry.example.com:443/x") != Window {
		t.Error("https's default port named is the same origin")
	}
}

// The token never shows: as is, URL-encoded, or as a link's token= value
// before the token is known.
func TestRedactHidesTheToken(t *testing.T) {
	for _, text := range []string{
		"Open http://localhost:7800/?token=" + fakeToken,
		"Open http://localhost:7800/?token=" + "s3cr%2Fet%2B1",
		"Open http://localhost:7800/?next=%2F%3Ftoken%3Ds3cr%252Fet",
		"the token is " + fakeToken + ".",
	} {
		got := Redact(text, fakeToken)
		if strings.Contains(got, "s3cr") || !strings.Contains(got, Hidden) {
			t.Errorf("Redact(%q) = %q", text, got)
		}
	}
	if got := Redact("Open http://localhost:7800/?token=abc&tab=1 now", ""); got != "Open http://localhost:7800/?token="+Hidden+"&tab=1 now" {
		t.Errorf("an unknown token: %q", got)
	}
}

// A token written in pieces is still hidden: the Redactor writes whole lines.
func TestRedactorHidesATokenSplitAcrossWrites(t *testing.T) {
	var out bytes.Buffer
	r := &Redactor{W: &out, Token: func() string { return "secret" }}
	for _, piece := range []string{"one sec", "ret here\ntwo se", "cret"} {
		r.Write([]byte(piece))
	}
	if out.String() != "one "+Hidden+" here\n" {
		t.Errorf("before Close: %q", out.String())
	}
	r.Close()
	if out.String() != "one "+Hidden+" here\ntwo "+Hidden {
		t.Errorf("after Close: %q", out.String())
	}
}
