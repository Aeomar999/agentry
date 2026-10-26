package main

import (
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
	"gitlab.com/dev.chempa/agentry/internal/launch"
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
	flags.Bool("resume-sessions", true, "an agent's next turn on a task goes on with its last session when it can (serve's default when not given)")
	flags.Duration("archive-after", 0, "archive tasks done for this long; 0 never archives (serve's default, 30 days, when not given)")
	if flags.Parse(args) != nil {
		return 2
	}
	passed := passedOn(flags, "https", "budget", "max-turns", "resume-sessions", "archive-after")
	dropParentSession(stderr) // before the server starts, so it doesn't inherit them
	for _, tool := range []string{"git", "claude"} {
		if _, err := exec.LookPath(tool); err != nil {
			fmt.Fprintf(stdout, "Note: %s isn't installed here, and agents need it. %s\n", tool, launch.InstallHint(tool))
		}
	}

	if running, ok := launch.Running(*dataDir); ok {
		if len(passed) > 0 {
			fmt.Fprintln(stdout, waitingForNextStart(passed))
		}
		return openPage(running, stdout, stderr)
	}
	if err := launch.DefaultRepo(*repo, ""); err != nil {
		fmt.Fprintln(stderr, "agentry open: making the default repo:", err)
		return 1
	}
	address, err := launch.FreeAddress("0.0.0.0", *port)
	if err != nil {
		fmt.Fprintln(stderr, "agentry open:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Starting Agentry…")
	if err := startServer(*dataDir, *repo, address, passed); err != nil {
		fmt.Fprintln(stderr, "agentry open:", err)
		return 1
	}
	if running, ok := launch.Wait(*dataDir, 30*time.Second, nil); ok {
		return openPage(running, stdout, stderr)
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
	server := exec.Command(self, launch.ServeArgs(dataDir, repo, address, extra)...)
	server.Dir = repo
	server.Stdout, server.Stderr = log, log
	procs.Detach(server)
	if err := server.Start(); err != nil {
		return err
	}
	os.WriteFile(filepath.Join(dataDir, pidFile), []byte(strconv.Itoa(server.Process.Pid)), 0o600)
	return server.Process.Release()
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
	signedIn := launch.SignIn(c.Token)
	links := []pageLink{{"This computer", launch.PageURL(c)}}
	here, err := url.Parse(c.URL)
	if err != nil {
		return links
	}
	if c.HTTPS {
		here.Scheme = "https"
	}
	own := strings.TrimSuffix(here.String(), "/") // serve's page when it has no public address
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

func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.Join(lines[max(0, len(lines)-n):], "\n")
}
