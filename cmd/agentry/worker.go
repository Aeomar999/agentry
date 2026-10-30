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
	"strconv"
	"strings"
	"syscall"

	"gitlab.com/dev.chempa/agentry/internal/domain/workers/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/platform/pinned"
	"gitlab.com/dev.chempa/agentry/internal/platform/procs"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

// workerConfig is what `agentry worker join` keeps, and the worker runs with.
// CA is the server's own CA, PEM encoded, when it is served over https with
// one: the worker trusts it alone for the server.
type workerConfig struct {
	Server     string `json:"server"`
	Name       string `json:"name"`
	Credential string `json:"credential"`
	CA         string `json:"ca,omitempty"`
}

// client calls the server, trusting only its CA when it has one.
func (c workerConfig) client() *http.Client { return pinned.Client([]byte(c.CA), 0) }

// link is the worker's end of the link to the server, with no machine: for
// asking about updates.
func (c workerConfig) link() *workerlink.Worker {
	return &workerlink.Worker{Server: c.Server, Credential: c.Credential, HTTP: c.client()}
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
	dir           string
	name          string // join -name
	caFingerprint string // join -ca-fingerprint
	follow        bool   // logs -f
	purge         bool   // leave -purge
}

var workerCommands = []workerCommand{
	{name: "join", args: "[-name n] [-ca-fingerprint f] <server> <code>", summary: "join a server with a code from its Workers page",
		run: runJoin, flags: func(f *flag.FlagSet, w *workerSetup) {
			f.StringVar(&w.name, "name", "", "what to call this machine (default: its host name)")
			f.StringVar(&w.caFingerprint, "ca-fingerprint", "", "the SHA-256 of the server's own CA, from `agentry ca`, to trust it over https")
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
// Run again with -ca-fingerprint, it moves a worker that joined over plain
// HTTP to the server's https, keeping the CA; without it, a worker keeps
// the CA it has.
func runJoin(w *workerSetup, args []string, stdout io.Writer) error {
	given := strings.TrimSuffix(args[0], "/")
	config, err := loadWorker(w.dir)
	joined := err == nil
	var target trustedServer
	if joined && config.Server == given && config.CA != "" && w.caFingerprint == "" {
		target = trustedServer{server: given, ca: []byte(config.CA)}
	} else if target, err = trustServer(given, w.caFingerprint, trustTimeout); err != nil {
		return err
	}
	if joined && sameServer(config.Server, given, target) {
		config.Server, config.CA = target.server, string(target.ca)
		if still, err := stillJoined(config); err != nil {
			return err
		} else if still {
			if err := saveWorker(w.dir, config); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "%s has joined %s already.\n", config.Name, config.Server)
			return nil
		}
	}
	return joinServer(w.dir, target, args[1], w.name, stdout)
}

// sameServer is whether a worker that joined joinedAt has joined the server
// given, which is reached at target: the same address, or, once the CA is
// checked, the https of the plain HTTP address it joined at.
func sameServer(joinedAt, given string, target trustedServer) bool {
	if joinedAt == given || joinedAt == target.server {
		return true
	}
	secure, err := httpsOf(joinedAt)
	return target.ca != nil && err == nil && secure == target.server
}

// stillJoined asks the server whether it takes the credential. A server
// whose certificate the kept CA doesn't vouch for is an error: its CA was
// regenerated, and only a new fingerprint can say to trust the new one.
func stillJoined(config workerConfig) (bool, error) {
	request, _ := http.NewRequest(http.MethodHead, config.Server+workerlink.UpdatePath, nil)
	request.Header.Set("Authorization", "Bearer "+config.Credential)
	response, err := config.client().Do(request)
	if pinned.Untrusted(err) {
		return false, fmt.Errorf("%s's CA isn't the one kept when this worker joined: it was regenerated, or this is another server. "+
			"Join again with -ca-fingerprint from `agentry ca` on the server", config.Server)
	}
	if err != nil {
		return true, nil // can't tell: keep what it has rather than spend the code
	}
	response.Body.Close()
	return response.StatusCode != http.StatusUnauthorized, nil
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

func joinServer(dir string, target trustedServer, code, name string, stdout io.Writer) error {
	server := target.server
	body, _ := json.Marshal(map[string]any{"code": code, "name": name, "machine": machines.Describe()})
	response, err := pinned.Client(target.ca, 0).Post(server+"/api/worker/join", "application/json", bytes.NewReader(body))
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
	joined.Server, joined.CA = server, string(target.ca)
	if err := saveWorker(dir, joined); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Joined %s as %s. Run `agentry worker start` to start taking work.\n", server, joined.Name)
	return nil
}

func saveWorker(dir string, config workerConfig) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	saved, _ := json.MarshalIndent(config, "", "  ")
	return os.WriteFile(filepath.Join(dir, "worker.json"), saved, 0o600)
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
	dropParentSession(logger.Writer())
	worker, err := workerlink.Setup{Server: config.Server, Name: config.Name, Credential: config.Credential, CA: config.CA,
		Dir: dir, HookCommand: shellQuote(self) + " hook"}.Worker()
	if err != nil {
		return err
	}
	worker.Log, worker.SelfUpdate = logger.Printf, true
	worker.OnLink = func(up bool, err error) { saveLinkState(dir, up, err) }
	err = worker.Run(ctx)
	if errors.Is(err, workerlink.ErrUpdated) {
		logger.Print("agentry was updated to the server's; restarting")
		pid, err := procs.Restart(self) // on Unix it returns only if it fails
		if err == nil && pid != 0 {
			// Windows: a new process carries on, so the service follows it.
			if _, statErr := os.Stat(filepath.Join(dir, "worker.pid")); statErr == nil {
				os.WriteFile(filepath.Join(dir, "worker.pid"), []byte(strconv.Itoa(pid)), 0o644)
			}
		}
		return err
	}
	return err
}
