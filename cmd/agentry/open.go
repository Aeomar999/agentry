package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/procs"
)

// `agentry open` is what Start Agentry, in the downloaded zip, runs: it
// starts the server in the background if it isn't running, and opens the
// page in your browser, signed in. Run again, it only opens the page.
// `agentry stop` stops the server it started.

const pidFile = "serve.pid"

func runOpen(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	home, _ := os.UserHomeDir()
	flags := flag.NewFlagSet("open", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data", filepath.Join(home, ".agentry"), "where Agentry keeps its log and the worktrees")
	repo := flags.String("repo", filepath.Join(home, "Agentry"), "the repo a new task works on when it names none")
	port := flags.Int("port", 7800, "the first port to try; Agentry listens on it on every interface")
	flags.Bool("https", false, "serve over https with Agentry's own local CA (see agentry serve -h)")
	flags.Int("budget", 0, "the tokens a new task may use before it asks you; 0 for no limit (serve's default when not given)")
	flags.Int("max-turns", 0, "the turns a new task may have without your answer; 0 for no limit (serve's default when not given)")
	if flags.Parse(args) != nil {
		return 2
	}
	passed := passedOn(flags, "https", "budget", "max-turns")
	dropParentSession(stderr) // before the server starts, so it doesn't inherit them
	for _, tool := range []string{"git", "claude"} {
		if _, err := exec.LookPath(tool); err != nil {
			fmt.Fprintf(stdout, "Note: %s isn't installed here, and agents need it. %s\n", tool, installHint(tool))
		}
	}

	if running, ok := runningServer(*dataDir); ok {
		if len(passed) > 0 {
			fmt.Fprintln(stdout, waitingForNextStart(passed))
		}
		return openPage(running, stdout, stderr)
	}
	if err := defaultRepo(*repo); err != nil {
		fmt.Fprintln(stderr, "agentry open: making the default repo:", err)
		return 1
	}
	address, err := freeAddress(*port)
	if err != nil {
		fmt.Fprintln(stderr, "agentry open:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Starting Agentry…")
	if err := startServer(*dataDir, *repo, address, passed); err != nil {
		fmt.Fprintln(stderr, "agentry open:", err)
		return 1
	}
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if running, ok := runningServer(*dataDir); ok {
			return openPage(running, stdout, stderr)
		}
	}
	said, _ := os.ReadFile(filepath.Join(*dataDir, "serve.log"))
	fmt.Fprintf(stderr, "agentry open: the server didn't start. Its log, %s, says:\n%s\n", filepath.Join(*dataDir, "serve.log"), lastLines(string(said), 15))
	return 1
}

func runStopServer(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	home, _ := os.UserHomeDir()
	flags := flag.NewFlagSet("stop", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data", filepath.Join(home, ".agentry"), "where Agentry keeps its log and the worktrees")
	if flags.Parse(args) != nil {
		return 2
	}
	saved, err := os.ReadFile(filepath.Join(*dataDir, pidFile))
	pid, _ := strconv.Atoi(strings.TrimSpace(string(saved)))
	if err != nil || pid == 0 || !procs.Alive(pid) {
		fmt.Fprintln(stdout, "Agentry isn't running (from Start Agentry).")
		return 0
	}
	process, _ := os.FindProcess(pid)
	procs.Terminate(process)
	for i := 0; i < 50 && procs.Alive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	os.Remove(filepath.Join(*dataDir, pidFile))
	fmt.Fprintln(stdout, "Agentry stopped. Agents' sessions that were running carry on, and the next start picks them up.")
	return 0
}

// runningServer is the server running from this data folder, if it answers.
func runningServer(dataDir string) (client.Client, bool) {
	c, err := client.Local(dataDir)
	if err != nil {
		return client.Client{}, false
	}
	c.Timeout = 2 * time.Second
	return c, c.Call("GET", "/api/config", nil, nil) == nil
}

// passedOn is the flags named that were given, as serve takes them, so
// serve's own defaults hold for the rest.
func passedOn(flags *flag.FlagSet, names ...string) []string {
	var args []string
	flags.Visit(func(f *flag.Flag) {
		if slices.Contains(names, f.Name) {
			args = append(args, "-"+f.Name+"="+f.Value.String())
		}
	})
	return args
}

// waitingForNextStart says that the flags passed, as passedOn gives them,
// don't change a server that is already running.
func waitingForNextStart(passed []string) string {
	names := make([]string, len(passed))
	for i, arg := range passed {
		names[i], _, _ = strings.Cut(arg, "=")
	}
	verb := "apply"
	if len(names) == 1 {
		verb = "applies"
	}
	said := fmt.Sprintf("Agentry is already running; %s %s only when it next starts.", strings.Join(names, " "), verb)
	if slices.Contains(names, "-https") {
		// Nothing saves -https, and Start Agentry runs plain `agentry open`.
		said += " Give -https on every start: Start Agentry doesn't pass it."
	}
	return said
}

// startServer runs `agentry serve` in the background, its output in the
// data folder's serve.log, with extra flags after its own; tests replace it.
var startServer = func(dataDir, repo, address string, extra []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(dataDir, "serve.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	server := exec.Command(self, append([]string{"serve", "-addr", address, "-repo", repo, "-data", dataDir}, extra...)...)
	server.Dir = repo
	server.Stdout, server.Stderr = log, log
	procs.Detach(server)
	if err := server.Start(); err != nil {
		return err
	}
	os.WriteFile(filepath.Join(dataDir, pidFile), []byte(strconv.Itoa(server.Process.Pid)), 0o600)
	return server.Process.Release()
}

// freeAddress is this machine's first free port from first on, on every
// interface, so other machines (workers, a phone) reach it too.
func freeAddress(first int) (string, error) {
	for port := first; port < first+20; port++ {
		address := fmt.Sprintf("0.0.0.0:%d", port)
		if listener, err := net.Listen("tcp", address); err == nil {
			listener.Close()
			return address, nil
		}
	}
	return "", fmt.Errorf("no free port from %d to %d", first, first+19)
}

// defaultRepo makes the repo new tasks work in when they name none, the
// first time: a git repo with one empty commit, so it has a branch.
func defaultRepo(repo string) error {
	if _, err := os.Stat(filepath.Join(repo, ".git")); err == nil {
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git isn't installed; " + installHint("git"))
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=Agentry", "-c", "user.email=agentry@localhost", "commit", "-q", "--allow-empty", "-m", "Start"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v %s", args[0], err, out)
		}
	}
	return nil
}

func openPage(c client.Client, stdout, stderr io.Writer) int {
	links := pageLinks(c)
	fmt.Fprintln(stdout, "Agentry is running. These links sign you in, so share them only with people you trust:")
	for _, link := range links {
		fmt.Fprintf(stdout, "  %-17s%s\n", link.label+":", link.url)
	}
	if err := openBrowser(links[0].url); err != nil {
		fmt.Fprintln(stdout, "Open the first link in your browser. Agentry keeps running; Stop Agentry stops it.")
		return 0
	}
	fmt.Fprintln(stdout, "Opened the first one in your browser. You can close this window: Agentry keeps running; Stop Agentry stops it.")
	return 0
}

type pageLink struct{ label, url string }

// pageLinks are the signed-in links to a running server: this computer's
// first, then its public address if it was given one, and one for each
// network address when it listens on every interface. They are https when
// the server serves it.
func pageLinks(c client.Client) []pageLink {
	signedIn := "/?token=" + c.Token
	here, err := url.Parse(c.URL)
	if err != nil {
		return []pageLink{{"This computer", c.PageURL(signedIn)}}
	}
	if c.HTTPS {
		here.Scheme = "https"
	}
	own := strings.TrimSuffix(here.String(), "/") // serve's page when it has no public address
	if ip := net.ParseIP(here.Hostname()); ip != nil && ip.IsLoopback() {
		here.Host = net.JoinHostPort("localhost", here.Port())
	}
	links := []pageLink{{"This computer", strings.TrimSuffix(here.String(), "/") + signedIn}}
	if page := strings.TrimSuffix(c.Page, "/"); page != "" && page != c.URL && page != own {
		links = append(links, pageLink{"Public address", c.PageURL(signedIn)})
	}
	for _, other := range networkURLs(listenAddress(c.Listen), here.Scheme) {
		links = append(links, pageLink{"On your network", other + signedIn})
	}
	return links
}

// listenAddress is where the server listens. A server started before it
// saved that gets no network links, rather than links that may not work.
func listenAddress(listen string) net.Addr {
	addr, err := net.ResolveTCPAddr("tcp", listen)
	if listen == "" || err != nil {
		return nil
	}
	return addr
}

// openBrowser opens link in the default browser; tests replace it.
var openBrowser = func(link string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", link).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", link).Start()
	}
	return exec.Command("xdg-open", link).Start()
}

func installHint(tool string) string {
	switch tool {
	case "git":
		return "Get it from https://git-scm.com/downloads."
	case "claude":
		return "Install Claude Code from https://claude.com/claude-code and sign in."
	}
	return ""
}

func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.Join(lines[max(0, len(lines)-n):], "\n")
}
