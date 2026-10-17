package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/workerlink"
)

// workerConfig is what `agentry worker join` keeps, and the worker runs with.
type workerConfig struct {
	Server     string `json:"server"`
	Name       string `json:"name"`
	Credential string `json:"credential"`
}

// workerCommand is one of `agentry worker`'s subcommands. run gets the
// worker's folder and the arguments after the subcommand's flags.
type workerCommand struct {
	name, args, summary string
	run                 func(w *workerSetup, args []string, stdout io.Writer) error
	flags               func(flags *flag.FlagSet, w *workerSetup)
}

// workerSetup is what every subcommand works with: the worker's folder,
// and the flags a subcommand takes.
type workerSetup struct {
	dir    string
	name   string // join -name
	follow bool   // logs -f
	purge  bool   // leave -purge
}

var workerCommands = []workerCommand{
	{name: "join", args: "[-name n] <server> <code>", summary: "join a server with a code from its Workers page",
		run: runJoin, flags: func(f *flag.FlagSet, w *workerSetup) {
			f.StringVar(&w.name, "name", "", "what to call this machine (default: its host name)")
		}},
	{name: "start", summary: "run in the background, started at login and kept running", run: runStart},
	{name: "stop", summary: "stop running in the background", run: runStop},
	{name: "restart", summary: "stop, then start", run: runStart},
	{name: "status", summary: "the server, the service, the link, and what agents will find here", run: runStatus},
	{name: "logs", args: "[-f]", summary: "the worker's log; -f follows it", run: runLogs,
		flags: func(f *flag.FlagSet, w *workerSetup) { f.BoolVar(&w.follow, "f", false, "follow the log") }},
	{name: "update", summary: "get the server's agentry now, and restart (it also does this by itself)", run: runUpdate},
	{name: "leave", args: "[-purge]", summary: "take this machine off the server and stop; -purge deletes its repos and worktrees too", run: runLeave,
		flags: func(f *flag.FlagSet, w *workerSetup) {
			f.BoolVar(&w.purge, "purge", false, "delete the worker's repos and worktrees")
		}},
	{name: "run", summary: "run in the foreground, until stopped (what the background service runs)", run: runForeground},
}

// runWorker is `agentry worker <command>`: everything a machine needs to
// run agents for a server. With no command it runs, as it always has.
func runWorker(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	name := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	if name == "help" {
		printWorkerUsage(stdout)
		return 0
	}
	i := slices.IndexFunc(workerCommands, func(c workerCommand) bool { return c.name == name })
	if i < 0 {
		fmt.Fprintf(stderr, "agentry worker: unknown command %q\n\n", name)
		printWorkerUsage(stderr)
		return 2
	}
	command := workerCommands[i]

	home, _ := os.UserHomeDir()
	setup := &workerSetup{}
	flags := flag.NewFlagSet("worker "+name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&setup.dir, "dir", filepath.Join(home, ".agentry-worker"), "where the worker keeps its credential, repos, worktrees and log")
	if command.flags != nil {
		command.flags(flags, setup)
	}
	wantArgs := strings.Count(command.args, "<")
	if flags.Parse(args) != nil || flags.NArg() != wantArgs {
		fmt.Fprintf(stderr, "usage: agentry worker %s %s\n", name, command.args)
		return 2
	}
	if err := command.run(setup, flags.Args(), stdout); err != nil {
		fmt.Fprintf(stderr, "agentry worker %s: %v\n", name, err)
		return 1
	}
	return 0
}

func printWorkerUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: agentry worker <command> [-dir folder]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range workerCommands {
		fmt.Fprintf(w, "  %-34s %s\n", strings.TrimSpace(c.name+" "+c.args), c.summary)
	}
}

// runJoin joins the server with the code, unless this machine has joined
// it already and the server still takes its credential: then the code is
// left unused, so the install command can be run again to update a worker.
func runJoin(w *workerSetup, args []string, stdout io.Writer) error {
	server := strings.TrimSuffix(args[0], "/")
	if config, err := loadWorker(w.dir); err == nil && config.Server == server && stillJoined(config) {
		fmt.Fprintf(stdout, "%s has joined %s already.\n", config.Name, server)
		return nil
	}
	return joinServer(w.dir, server, args[1], w.name, stdout)
}

// stillJoined asks the server whether it takes the credential.
func stillJoined(config workerConfig) bool {
	request, _ := http.NewRequest(http.MethodHead, config.Server+workerlink.UpdatePath, nil)
	request.Header.Set("Authorization", "Bearer "+config.Credential)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return true // can't tell: keep what it has rather than spend the code
	}
	response.Body.Close()
	return response.StatusCode != http.StatusUnauthorized
}

func runForeground(w *workerSetup, _ []string, stdout io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runAsWorker(ctx, w.dir, stdout)
}

// loadWorker reads what `agentry worker join` kept.
func loadWorker(dir string) (workerConfig, error) {
	var config workerConfig
	saved, err := os.ReadFile(filepath.Join(dir, "worker.json"))
	if errors.Is(err, os.ErrNotExist) {
		return config, errors.New("this machine hasn't joined a server yet; run the command from the Workers page")
	}
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(saved, &config); err != nil {
		return config, fmt.Errorf("reading %s: %w", filepath.Join(dir, "worker.json"), err)
	}
	return config, nil
}

func joinServer(dir, server, code, name string, stdout io.Writer) error {
	server = strings.TrimSuffix(server, "/")
	body, _ := json.Marshal(map[string]any{"code": code, "name": name, "machine": machines.Describe()})
	response, err := http.Post(server+"/api/worker/join", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reason, _ := io.ReadAll(response.Body)
		return fmt.Errorf("%s: %s", response.Status, strings.TrimSpace(string(reason)))
	}
	var joined workerConfig
	if err := json.NewDecoder(response.Body).Decode(&joined); err != nil {
		return err
	}
	joined.Server = server
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	saved, _ := json.MarshalIndent(joined, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "worker.json"), saved, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Joined %s as %s. Run `agentry worker start` to start taking work.\n", server, joined.Name)
	return nil
}

func runAsWorker(ctx context.Context, dir string, out io.Writer) error {
	config, err := loadWorker(dir)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	logger := log.New(out, "", log.LstdFlags)
	logger.Printf("%s, working for %s", config.Name, config.Server)
	worker := &workerlink.Worker{
		Server: config.Server, Credential: config.Credential, About: machines.Describe(), Log: logger.Printf, SelfUpdate: true,
		OnLink: func(up bool, err error) { saveLinkState(dir, up, err) },
		Machine: &machines.Local{Label: config.Name, ServerURL: config.Server, HookCommand: shellQuote(self) + " hook",
			WorktreeRoot: filepath.Join(dir, "worktrees"),
			Hub: &machines.Hub{URL: workerlink.HubURL(config.Server), Credential: config.Credential,
				MirrorRoot: filepath.Join(dir, "repos")}},
	}
	err = worker.Run(ctx)
	if errors.Is(err, workerlink.ErrUpdated) {
		logger.Print("agentry was updated to the server's; restarting")
		return syscall.Exec(self, os.Args, os.Environ()) // the same process, the new program
	}
	return err
}
