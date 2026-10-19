package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/coordinator"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/desk"
	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/hooks"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/terminal"
	"gitlab.com/dev.chempa/agentry/internal/turns"
	"gitlab.com/dev.chempa/agentry/internal/watchdog"
	"gitlab.com/dev.chempa/agentry/internal/web"
	"gitlab.com/dev.chempa/agentry/internal/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

type serveOptions struct {
	address   string
	repo      string // the default repo for new tasks
	dataDir   string // the log and the worktrees
	budget    int    // tokens a new task may use before it asks you; 0 is no limit
	maxTurns  int    // turns a new task may have without your answer before it asks you; 0 is no limit
	publicURL string // the address people open; "" for the address listened on
}

func runServe(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	home, _ := os.UserHomeDir()
	here, _ := os.Getwd()
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var options serveOptions
	flags.StringVar(&options.address, "addr", "0.0.0.0:7800", "where to listen: every interface by default, so other machines reach it; 127.0.0.1:7800 for this machine only")
	flags.StringVar(&options.publicURL, "public-url", "", "the address people open, for the printed link, such as https://agentry.example.com")
	flags.StringVar(&options.repo, "repo", here, "the repo a new task works on when it names none")
	flags.StringVar(&options.dataDir, "data", filepath.Join(home, ".agentry"), "where Agentry keeps its log and the worktrees")
	flags.IntVar(&options.budget, "budget", 5_000_000, "the tokens a new task may use before it asks you; 0 for no limit")
	flags.IntVar(&options.maxTurns, "max-turns", coordinator.DefaultMaxTurns, "the turns a new task may have without your answer before it asks you; 0 for no limit")
	if flags.Parse(args) != nil {
		return 2
	}
	if options.budget < 0 || options.maxTurns < 0 {
		fmt.Fprintln(stderr, "agentry serve: -budget and -max-turns are 0 or above, 0 for no limit")
		return 2
	}

	dropParentSession(stderr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, options, stdout); err != nil {
		fmt.Fprintln(stderr, "agentry serve:", err)
		return 1
	}
	return 0
}

// dropParentSession removes what ties this process to an engine session it
// was started from, such as an assistant running `agentry open` from its
// Bash tool, so the sessions it starts aren't taken for that one's children.
func dropParentSession(log io.Writer) {
	if removed := engines.DropParentSession(); len(removed) > 0 {
		fmt.Fprintln(log, "agentry: removed variables inherited from a Claude Code or Codex session:", strings.Join(removed, ", "))
	}
}

// reportSkipped says once per prefix that the log holds streams of a
// feature this build doesn't have: skipped, not lost.
func reportSkipped(w io.Writer, unknown []current.Unknown) {
	for _, u := range unknown {
		if u.Prefix != "" {
			fmt.Fprintf(w, "agentry: skipped %d events in %d %s streams, from a feature this build doesn't have; the log keeps them\n", u.Events, u.Streams, u.Prefix)
		}
	}
}

func serve(ctx context.Context, o serveOptions, stdout io.Writer) error {
	if err := os.MkdirAll(o.dataDir, 0o700); err != nil {
		return err
	}
	catalogue, err := events.NewCatalogue(ideas.Registration())
	if err != nil {
		return fmt.Errorf("the event catalogue: %w", err)
	}
	state := current.New(catalogue, ideas.Streams())
	log, err := store.Open(filepath.Join(o.dataDir, "agentry.db"), catalogue, state, func(p events.Proposal) *events.Rejection { return rules.Check(p, state, catalogue) })
	if err != nil {
		return err
	}
	defer log.Close()
	reportSkipped(stdout, state.Unknown())
	if err := seedAgents(log, state); err != nil {
		return err
	}
	if err := hooks.HoldOrphanedAsks(log, state); err != nil {
		return err
	}
	agentPolicy, err := openPolicy(o.dataDir, log)
	if err != nil {
		return err
	}

	// Listen before anything starts, so a busy port fails at once.
	listener, err := net.Listen("tcp", o.address)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	uiToken, err := keptToken(filepath.Join(o.dataDir, "ui-token"))
	if err != nil {
		return err
	}
	// Kept across restarts like the UI token: a session outlives the server
	// that started it, and a new token would lock its hooks out (fail closed).
	hookToken, err := keptToken(filepath.Join(o.dataDir, hookTokenFile))
	if err != nil {
		return err
	}
	deskToken, err := keptToken(filepath.Join(o.dataDir, deskTokenFile)) // the same, for the desk's session
	if err != nil {
		return err
	}
	helperToken, err := keptToken(filepath.Join(o.dataDir, helperTokenFile)) // the Claude sessions beside tasks, which only draft
	if err != nil {
		return err
	}
	serverURL := localURL(listener.Addr())  // what agents' hooks and the desk call, on this machine
	network := networkURLs(listener.Addr()) // what other machines reach, when listening on every address
	var theme atomic.Value                  // the page's theme, which the sessions it watches start in
	theme.Store("dark")
	currentTheme := func() string { return theme.Load().(string) }

	// Agents work here, and on any worker that joins and is online.
	clones := filepath.Join(o.dataDir, "repos")                       // repos named by a git address or an archive link
	blobs := sharedfiles.Disk{Dir: filepath.Join(o.dataDir, "files")} // tasks' shared files
	worktreeRoot := filepath.Join(o.dataDir, "worktrees")
	here := &machines.Local{Label: workers.Local, ServerURL: serverURL, HookCommand: shellQuote(self) + " hook", Blobs: blobs,
		WorktreeRoot: worktreeRoot}
	fleet := machines.NewFleet(here)
	if err := workerlink.ForgetConnections(log, state, fleet); err != nil {
		return err
	}
	links := workerlink.NewServer(log, fleet)

	runner := turns.NewRunner(log, state, fleet, turns.Config{HookToken: hookToken, Theme: currentTheme})
	loop := coordinator.NewLoop(log, state, runner, fleet)
	loop.Clones = clones
	loop.Defaults = tasks.Limits{Budget: o.budget, MaxTurns: o.maxTurns}
	go loop.Run(ctx)
	go watchdog.New(log, state, watchdog.DefaultLimits, runner.IsRunning, runner.Stop).Run(ctx)
	go agentPolicy.Watch(ctx, 2*time.Second, func(loaded *policy.Change, refused error) { recordOnDisk(log, agentPolicy, loaded, refused) })

	// The desk starts when you first open it on the page, and stops with the server.
	hub := terminal.NewHub()
	deskConfig := desk.Config{Dir: o.repo, ServerURL: serverURL, Token: deskToken, Binary: self}
	deskTerminal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("theme"); t == "light" || t == "dark" {
			theme.Store(t) // the page opening the desk says its theme
		}
		config := deskConfig
		config.Theme = currentTheme()
		if err := desk.Start(config); err != nil {
			http.Error(w, "starting Claude: "+err.Error(), http.StatusInternalServerError)
			return
		}
		hub.Serve(w, r, desk.Name, terminal.Attach)
	})
	// A task's terminal is on the machine its work is on.
	taskTerminal := hub.Handler(func(task string, onOutput func([]byte)) (terminal.Pane, error) {
		return runner.MachineOf(task).Attach(task, onOutput)
	})
	// Your own Claude session beside a task, on the machine its work is on:
	// started when you first open it, and kept, so you come back to it.
	helperTerminal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("task")
		task, err := state.Task(id)
		if err != nil || task == nil || task.Worktree == nil {
			http.Error(w, "the task has no worktree to open Claude in", http.StatusNotFound)
			return
		}
		machine := runner.MachineOf(id)
		start, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		err = machine.StartHelper(start, machines.Helper{Task: id, Worktree: task.Worktree.Path, Theme: currentTheme(),
			Prompt: machines.HelperPrompt(id, task.Title, task.Worktree.Branch),
			Env:    map[string]string{machines.EnvHelperToken: helperToken, hookcall.EnvTask: id}})
		if err != nil {
			http.Error(w, "starting Claude: "+err.Error(), http.StatusInternalServerError)
			return
		}
		hub.Serve(w, r, machines.HelperSession(id), machine.Attach)
	})
	defer desk.Stop()

	// Every feature in this binary, written out here: no feature registers itself.
	installed := []features.Feature{
		ideas.Feature(state),
	}
	server := web.NewServer(log, state, web.Config{UIToken: uiToken, DeskToken: deskToken, HelperToken: helperToken, Helper: helperTerminal,
		DefaultRepo: o.repo, Clones: clones, Budget: o.budget, MaxTurns: o.maxTurns, Policy: agentPolicy, Desk: deskTerminal,
		SetTheme: func(t string) { theme.Store(t) }, Fleet: fleet, Workers: links, PublicURL: o.publicURL,
		NetworkURLs: network, LocalURL: localhostURL(listener.Addr()), Binary: self, Local: machines.Describe(), Blobs: blobs,
		Features: installed, Worktrees: worktreeRoot},
		hooks.HTTP(hooks.NewHandler(log, state, agentPolicy.Decide), hookToken), taskTerminal)
	publicURL := cmp.Or(strings.TrimSuffix(o.publicURL, "/"), serverURL)
	if err := client.Save(o.dataDir, client.Where{URL: serverURL, Page: publicURL, Listen: listener.Addr().String()}); err != nil {
		return err // `agentry mcp` and the status line find the server by it
	}
	fmt.Fprintf(stdout, "Agentry is running. Open %s/?token=%s\n", publicURL, uiToken)
	if o.publicURL == "" {
		for _, other := range network {
			fmt.Fprintf(stdout, "From another machine: %s/?token=%s\n", other, uiToken)
		}
	}
	fmt.Fprintf(stdout, "New tasks work on %s unless they name another repo. Ctrl-C to stop.\n", o.repo)
	fmt.Fprintf(stdout, "Agents follow the policy in %s; edits take effect when you save.\n", agentPolicy.Path())
	return serveUntilDone(ctx, listener, server.Handler())
}

// localURL is the server as seen from this machine. Listening on every
// address (0.0.0.0, as in a container) is reached on loopback.
func localURL(addr net.Addr) string {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || !tcp.IP.IsUnspecified() {
		return "http://" + addr.String()
	}
	return fmt.Sprintf("http://127.0.0.1:%d", tcp.Port)
}

// localhostURL is the server at localhost, as a page on this machine opens
// it, or "" when it listens on one address that isn't loopback.
func localhostURL(addr net.Addr) string {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || !tcp.IP.IsUnspecified() && !tcp.IP.IsLoopback() {
		return ""
	}
	return fmt.Sprintf("http://localhost:%d", tcp.Port)
}

// networkURLs are the server as other machines on the network reach it,
// when it listens on every address: one for each of this machine's own
// IPv4 addresses, leaving out loopback and the bridges containers use, which
// no other machine reaches.
func networkURLs(addr net.Addr) []string {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || !tcp.IP.IsUnspecified() {
		return nil
	}
	interfaces, _ := net.Interfaces()
	var urls []string
	for _, i := range interfaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || isBridge(i.Name) {
			continue
		}
		addresses, _ := i.Addrs()
		for _, address := range addresses {
			if ip, ok := address.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLinkLocalUnicast() {
				urls = append(urls, fmt.Sprintf("http://%s:%d", ip.IP, tcp.Port))
			}
		}
	}
	return urls
}

func isBridge(name string) bool {
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "cni", "flannel", "vEthernet"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func seedAgents(log *store.Store, state *current.State) error {
	if err := state.Broken(events.AgentsStream); err != nil {
		return err
	}
	roster := state.Roster()
	for _, p := range append(agents.SeedProposals(roster), agents.Upgrades(roster)...) {
		if _, rejection, err := log.Propose(p); err != nil || rejection != nil {
			return fmt.Errorf("seeding the default agents: %v", errors.Join(err, rejectionError(rejection)))
		}
	}
	return nil
}

func serveUntilDone(ctx context.Context, listener net.Listener, handler http.Handler) error {
	// Requests share ctx, so stopping cancels a hook call waiting for you, and
	// it records its own denial before the server exits.
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// The tokens kept in the data folder, next to the UI token.
const (
	hookTokenFile   = "hook-token"
	deskTokenFile   = "desk-token"
	helperTokenFile = "helper-token"
)

// keptToken reads a token from path, or makes and saves one, so whatever
// holds it (the printed link, a running session) keeps working across restarts.
func keptToken(path string) (string, error) {
	if saved, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(saved))) == 32 {
		return strings.TrimSpace(string(saved)), nil
	}
	token := newToken()
	return token, os.WriteFile(path, []byte(token+"\n"), 0o600)
}

func newToken() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// shellQuote quotes a path for the hook command, which engines run in a shell.
// On Windows, engines run hooks with Git Bash, so the path is written with
// forward slashes, which it and Windows both read.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(filepath.ToSlash(path), "'", `'\''`) + "'"
}

func rejectionError(rejection *events.Rejection) error {
	if rejection == nil {
		return nil
	}
	return rejection
}
