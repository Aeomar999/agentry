package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	if flags.Parse(args) != nil {
		return 2
	}
	for _, tool := range []string{"git", "claude"} {
		if _, err := exec.LookPath(tool); err != nil {
			fmt.Fprintf(stdout, "Note: %s isn't installed here, and agents need it. %s\n", tool, installHint(tool))
		}
	}

	if running, ok := runningServer(*dataDir); ok {
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
	if err := startServer(*dataDir, *repo, address); err != nil {
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

// startServer runs `agentry serve` in the background, its output in the
// data folder's serve.log.
func startServer(dataDir, repo, address string) error {
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
	server := exec.Command(self, "serve", "-addr", address, "-repo", repo, "-data", dataDir)
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
	link := c.PageURL("/?token=" + c.Token)
	if err := openBrowser(link); err != nil {
		fmt.Fprintf(stdout, "Agentry is running. Open %s in your browser.\n", link)
		return 0
	}
	fmt.Fprintf(stdout, "Agentry is running at %s and opened in your browser.\nYou can close this window: it keeps running. Stop Agentry stops it.\n", c.PageURL(""))
	return 0
}

func openBrowser(link string) error {
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
