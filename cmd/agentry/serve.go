package main

import (
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
	"gitlab.com/dev.chempa/agentry/internal/coordinator"
	"gitlab.com/dev.chempa/agentry/internal/desk"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/hooks"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/terminal"
	"gitlab.com/dev.chempa/agentry/internal/turns"
	"gitlab.com/dev.chempa/agentry/internal/watchdog"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

type serveOptions struct {
	address string
	repo    string // the default repo for new tasks
	dataDir string // the log and the worktrees
}

func runServe(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	home, _ := os.UserHomeDir()
	here, _ := os.Getwd()
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var options serveOptions
	flags.StringVar(&options.address, "addr", "127.0.0.1:7800", "where to listen")
	flags.StringVar(&options.repo, "repo", here, "the repo a new task works on when it names none")
	flags.StringVar(&options.dataDir, "data", filepath.Join(home, ".agentry"), "where Agentry keeps its log and the worktrees")
	if flags.Parse(args) != nil {
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, options, stdout); err != nil {
		fmt.Fprintln(stderr, "agentry serve:", err)
		return 1
	}
	return 0
}

func serve(ctx context.Context, o serveOptions, stdout io.Writer) error {
	if err := os.MkdirAll(o.dataDir, 0o700); err != nil {
		return err
	}
	log, err := store.Open(filepath.Join(o.dataDir, "agentry.db"), rules.Check)
	if err != nil {
		return err
	}
	defer log.Close()
	if err := seedAgents(log); err != nil {
		return err
	}
	if err := hooks.DenyOrphanedAsks(log); err != nil {
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
	hookToken := newToken() // new each start: only sessions started by this server have it
	deskToken := newToken() // the same, for the desk's session
	serverURL := "http://" + listener.Addr().String()
	var theme atomic.Value // the page's theme, which the sessions it watches start in
	theme.Store("dark")
	currentTheme := func() string { return theme.Load().(string) }

	runner := turns.NewRunner(log, turns.Config{HookCommand: shellQuote(self) + " hook",
		ServerURL: serverURL, HookToken: hookToken, Theme: currentTheme})
	isRunning := func(task string) bool { return sessions.IsRunning(sessions.SocketFor(task)) }
	go coordinator.NewLoop(log, runner, filepath.Join(o.dataDir, "worktrees")).Run(ctx)
	go watchdog.New(log, watchdog.DefaultLimits, isRunning, runner.Stop).Run(ctx)
	go agentPolicy.Watch(ctx, 2*time.Second, func(loaded *policy.Policy, refused error) { recordPolicy(log, agentPolicy, refused) })

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
		hub.Serve(w, r, desk.Name)
	})
	defer desk.Stop()

	server := web.NewServer(log, web.Config{UIToken: uiToken, DeskToken: deskToken, DefaultRepo: o.repo, Policy: agentPolicy, Desk: deskTerminal,
		SetTheme: func(t string) { theme.Store(t) }},
		hooks.HTTP(hooks.NewHandler(log, agentPolicy.Decide), hookToken), hub.Handler())
	fmt.Fprintf(stdout, "Agentry is running. Open http://%s/?token=%s\n", listener.Addr(), uiToken)
	fmt.Fprintf(stdout, "New tasks work on %s unless they name another repo. Ctrl-C to stop.\n", o.repo)
	fmt.Fprintf(stdout, "Agents follow the policy in %s; edits take effect when you save.\n", agentPolicy.Path())
	return serveUntilDone(ctx, listener, server.Handler())
}

func seedAgents(log *store.Store) error {
	stream, err := log.Stream(events.AgentsStream)
	if err != nil {
		return err
	}
	roster := agents.FromEvents(stream)
	for _, p := range append(agents.SeedProposals(roster), agents.ProfileUpgrades(roster)...) {
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

// keptToken reads the UI token from path, or makes and saves one, so the
// printed link keeps working across restarts.
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
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func rejectionError(rejection *events.Rejection) error {
	if rejection == nil {
		return nil
	}
	return rejection
}
