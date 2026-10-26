// Package launch is what a program that opens Agentry for you does: find
// the server running from a data folder, or start `agentry serve`, and
// build the signed-in link to its page. `agentry open` (Start Agentry) and
// the desktop app use it. It has no GUI and no cgo.
package launch

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/procs"
)

// Running is the server running from this data folder, if it answers.
func Running(dataDir string) (client.Client, bool) {
	c, err := client.Local(dataDir)
	if err != nil {
		return client.Client{}, false
	}
	c.Timeout = 2 * time.Second
	return c, c.Call("GET", "/api/config", nil, nil) == nil
}

// Wait is the server answering from this data folder, once it answers,
// for as long as within. It gives up early when gone closes, as when the
// server it waits for exits; gone may be nil.
func Wait(dataDir string, within time.Duration, gone <-chan struct{}) (client.Client, bool) {
	for deadline := time.Now().Add(within); time.Now().Before(deadline); {
		if running, ok := Running(dataDir); ok {
			return running, true
		}
		select {
		case <-gone:
			return client.Client{}, false
		case <-time.After(200 * time.Millisecond):
		}
	}
	return client.Client{}, false
}

// FreeAddress is this machine's first free port from first on, on host:
// "0.0.0.0" for every interface, "127.0.0.1" for this computer only.
func FreeAddress(host string, first int) (string, error) {
	for port := first; port < first+20; port++ {
		address := net.JoinHostPort(host, fmt.Sprint(port))
		if listener, err := net.Listen("tcp", address); err == nil {
			listener.Close()
			return address, nil
		}
	}
	return "", fmt.Errorf("no free port from %d to %d", first, first+19)
}

// DefaultRepo makes the repo new tasks work in when they name none, the
// first time: a git repo with one empty commit, so it has a branch. It
// runs the git in path, a PATH; empty, this program's.
func DefaultRepo(repo, path string) error {
	if _, err := os.Stat(filepath.Join(repo, ".git")); err == nil {
		return nil
	}
	git, err := lookPathIn("git", path)
	if err != nil {
		return errors.New("git isn't installed; " + InstallHint("git"))
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=Agentry", "-c", "user.email=agentry@localhost", "commit", "-q", "--allow-empty", "-m", "Start"},
	} {
		cmd := exec.Command(git, append([]string{"-C", repo}, args...)...)
		withPath(cmd, path)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v %s", args[0], err, out)
		}
	}
	return nil
}

// lookPathIn is the program name in path, a PATH, as exec.LookPath finds it
// in this program's when path is empty. Entries that aren't absolute, such
// as ".", are skipped, as exec.LookPath refuses a program found through one.
func lookPathIn(name, path string) (string, error) {
	if path == "" {
		return exec.LookPath(name)
	}
	for _, dir := range filepath.SplitList(path) {
		if !filepath.IsAbs(dir) {
			continue
		}
		if found, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("%s isn't in %s", name, path)
}

// withPath gives cmd path as its PATH, when path isn't empty.
func withPath(cmd *exec.Cmd, path string) {
	if path != "" {
		cmd.Env = append(os.Environ(), "PATH="+path) // the later PATH wins
	}
}

// InstallHint says where to get a tool agents need.
func InstallHint(tool string) string {
	switch tool {
	case "git":
		return "Get it from https://git-scm.com/downloads."
	case "claude":
		return "Install Claude Code from https://claude.com/claude-code and sign in."
	}
	return ""
}

// ServeArgs are the arguments that run `agentry serve` on address, with
// this data folder and default repo, and extra flags after its own.
func ServeArgs(dataDir, repo, address string, extra []string) []string {
	return append([]string{"serve", "-addr", address, "-repo", repo, "-data", dataDir}, extra...)
}

// LockFile is the file in the data folder that the one serve using it
// holds locked, with its pid inside.
const LockFile = "serve.lock"

// Server is an `agentry serve` this program started as its child. Unlike
// Start Agentry's, it isn't detached and writes no serve.pid: it is this
// program's to stop.
type Server struct {
	process *os.Process
	exited  chan struct{}
	stop    sync.Once
}

// Start runs agentry, the binary at that path, as `agentry serve` with
// ServeArgs, in the repo, its output to output (nil drops it), with path as
// its PATH (empty: this program's).
func Start(agentry, dataDir, repo, address string, extra []string, output io.Writer, path string) (*Server, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	cmd := exec.Command(agentry, ServeArgs(dataDir, repo, address, extra)...)
	cmd.Dir = repo
	cmd.Stdout, cmd.Stderr = output, output
	withPath(cmd, path)
	// A process serve starts may keep its output open after serve exits;
	// don't wait on that, or Exited and Stop never return.
	cmd.WaitDelay = 2 * time.Second
	procs.NoConsole(cmd) // the desktop app has no console to lend it
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &Server{process: cmd.Process, exited: make(chan struct{})}
	go func() {
		cmd.Wait()
		close(s.exited)
	}()
	return s, nil
}

// Exited closes when the server has exited.
func (s *Server) Exited() <-chan struct{} { return s.exited }

// Stop asks the server to stop and waits for it, ending it after 10
// seconds if it hasn't.
func (s *Server) Stop() {
	s.stop.Do(func() {
		procs.Terminate(s.process)
		select {
		case <-s.exited:
		case <-time.After(10 * time.Second):
			s.process.Kill()
			<-s.exited
		}
	})
}

// serves says whether the server answering from the data folder is this
// child: still running, listening on address, and holding serve's lock.
// Windows keeps the lock file unreadable while it is held, so there the
// first two decide.
func (s *Server) serves(dataDir string, running client.Client, address string) bool {
	select {
	case <-s.exited:
		return false
	default:
	}
	if !sameAddress(running.Listen, address) {
		return false
	}
	held, err := os.ReadFile(filepath.Join(dataDir, LockFile))
	pid := strings.TrimSpace(string(held))
	return err != nil || pid == "" || pid == strconv.Itoa(s.process.Pid)
}

// sameAddress says whether two listen addresses are one: the same port and
// host, where every wildcard is the same host (serve saves 0.0.0.0 as [::]).
func sameAddress(a, b string) bool {
	hostA, portA, errA := net.SplitHostPort(a)
	hostB, portB, errB := net.SplitHostPort(b)
	if errA != nil || errB != nil || portA != portB {
		return false
	}
	ipA, ipB := net.ParseIP(hostA), net.ParseIP(hostB)
	return hostA == hostB || ipA != nil && ipB != nil && (ipA.Equal(ipB) || ipA.IsUnspecified() && ipB.IsUnspecified())
}

// Options are where Ensure finds or starts Agentry: the data folder and
// default repo, the path of the agentry binary to start, the host (127.0.0.1 when
// empty) and first port it listens on, where its output goes, and the PATH
// the server and the git that makes the repo get (this program's when
// empty). Once
// Quit closes, Ensure starts no server and stops one it is waiting for;
// it may be nil.
type Options struct {
	DataDir, Repo string
	Binary        string
	Host          string
	Port          int
	Output        io.Writer
	Path          string
	Quit          <-chan struct{}
}

// ErrQuit is Ensure's error once Options.Quit has closed.
var ErrQuit = errors.New("quitting, so no server was started")

// Agentry is a running server, and the child to stop when this program
// started it.
type Agentry struct {
	client.Client
	child *Server
}

// Started says whether this program started the server.
func (a *Agentry) Started() bool { return a.child != nil }

// Stop stops the server if this program started it, and leaves one it
// found running alone.
func (a *Agentry) Stop() {
	if a.child != nil {
		a.child.Stop()
	}
}

// Ensure is the server running from the data folder if one answers, or
// else one it starts as its child, once that answers. The child listens on
// this computer only unless Host says otherwise.
func Ensure(o Options) (*Agentry, error) {
	if closed(o.Quit) {
		return nil, ErrQuit
	}
	if running, ok := Running(o.DataDir); ok {
		return &Agentry{Client: running}, nil
	}
	if err := DefaultRepo(o.Repo, o.Path); err != nil {
		return nil, fmt.Errorf("making the default repo: %w", err)
	}
	host := o.Host
	if host == "" {
		host = "127.0.0.1"
	}
	address, err := FreeAddress(host, o.Port)
	if err != nil {
		return nil, err
	}
	if closed(o.Quit) {
		return nil, ErrQuit
	}
	child, err := Start(o.Binary, o.DataDir, o.Repo, address, nil, o.Output, o.Path)
	if err != nil {
		return nil, err
	}
	running, ok := Wait(o.DataDir, 30*time.Second, either(child.Exited(), o.Quit))
	if !ok {
		child.Stop()
		if closed(o.Quit) {
			return nil, ErrQuit
		}
		return nil, errors.New("the server didn't start")
	}
	if !child.serves(o.DataDir, running, address) {
		// Another launch started a server from this data folder at the same
		// time, and it answered: use that one, and stop ours.
		child.Stop()
		return &Agentry{Client: running}, nil
	}
	return &Agentry{Client: running, child: child}, nil
}

// closed says whether c has closed; a nil c never does.
func closed(c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

// either closes when a or b closes; b may be nil.
func either(a, b <-chan struct{}) <-chan struct{} {
	if b == nil {
		return a
	}
	both := make(chan struct{})
	go func() {
		select {
		case <-a:
		case <-b:
		}
		close(both)
	}()
	return both
}

// SignIn is the path that opens the page signed in with the token.
func SignIn(token string) string { return "/?token=" + token }

// PageURL is the link that opens a running server's page on this computer,
// signed in: at localhost when the server is on this computer's loopback,
// and https when the server serves it.
func PageURL(c client.Client) string {
	signedIn := SignIn(c.Token)
	here, err := url.Parse(c.URL)
	if err != nil {
		return c.PageURL(signedIn)
	}
	if c.HTTPS {
		here.Scheme = "https"
	}
	if ip := net.ParseIP(here.Hostname()); ip != nil && ip.IsLoopback() {
		here.Host = net.JoinHostPort("localhost", here.Port())
	}
	return strings.TrimSuffix(here.String(), "/") + signedIn
}
