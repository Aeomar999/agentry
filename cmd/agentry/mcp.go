package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/mcp"
)

// `agentry mcp` is Agentry for Claude Code: an MCP server on stdio with
// Agentry's tools, for the repo Claude Code runs in. It reaches the server
// this machine was connected to (`agentry mcp connect`, which the install
// command from the Integrations page runs), or else the one running here;
// --server and --token (or AGENTRY_URL and AGENTRY_TOKEN) name another.
// `agentry mcp install` adds it to Claude Code for every repo, with the
// status line.

const mcpUsage = `Usage:
  agentry mcp [--server URL --token TOKEN] [--data DIR] [--quiet]
                 run as Claude Code's MCP server (Claude Code starts it)
  agentry mcp install [--server URL --token TOKEN]
                 add it to Claude Code for every repo, and the status line
  agentry mcp connect SERVER CODE
                 connect this machine with a code from the Integrations page`

func runMCP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "connect" {
		return connectMachine(args[1:], stdout, stderr)
	}
	install := len(args) > 0 && args[0] == "install"
	if install {
		args = args[1:]
	}
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	server := flags.String("server", os.Getenv("AGENTRY_URL"), "")
	token := flags.String("token", os.Getenv("AGENTRY_TOKEN"), "")
	dataDir := flags.String("data", defaultDataDir(), "")
	quiet := flags.Bool("quiet", false, "") // no notifications on this computer
	if err := flags.Parse(args); err != nil {
		fmt.Fprintf(stderr, "%v\n%s\n", err, mcpUsage)
		return 2
	}
	agentry, err := connect(*server, *token, *dataDir)
	if err != nil {
		fmt.Fprintln(stderr, "agentry mcp:", err)
		return 1
	}
	if install {
		return installMCP(*server, *token, stdout, stderr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dir, _ := os.Getwd()
	s := mcp.New(agentry, dir, machines.AgentryVersion())
	if !*quiet {
		go s.Watch(ctx)
	}
	if err := s.Serve(ctx, stdin, stdout); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, "agentry mcp:", err)
		return 1
	}
	return 0
}

// connect is the server given, the one this machine was connected to, or
// the one running on it.
func connect(server, token, dataDir string) (client.Client, error) {
	if server != "" || token != "" {
		if server == "" || token == "" {
			return client.Client{}, errors.New("give both --server and --token (the page's token, from its link)")
		}
		return client.Client{Where: client.Where{URL: server}, Token: token}, nil
	}
	if connected, ok := client.Connected(); ok {
		return connected, nil
	}
	return client.Local(dataDir)
}

// connectMachine swaps a code from the Integrations page for this machine's
// credential, and keeps it.
func connectMachine(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, mcpUsage)
		return 2
	}
	server := strings.TrimSuffix(args[0], "/")
	host, _ := os.Hostname()
	var made struct{ Credential, Page string }
	request := map[string]string{"code": args[1], "host": host, "user": os.Getenv("USER"), "os": runtime.GOOS}
	if err := (client.Client{Where: client.Where{URL: server}}).Call("POST", "/api/integrations/connect", request, &made); err != nil {
		fmt.Fprintln(stderr, "agentry mcp connect:", err)
		return 1
	}
	if err := client.SaveConnection(client.Connection{URL: server, Credential: made.Credential}); err != nil {
		fmt.Fprintln(stderr, "agentry mcp connect:", err)
		return 1
	}
	fmt.Fprintf(stdout, "Connected to Agentry at %s as %s. The Integrations page lists it, and can revoke it.\n", server, host)
	return 0
}

func defaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agentry")
}

// installMCP adds this binary to Claude Code as the "agentry" MCP server
// for every repo, and sets the status line unless you have one.
func installMCP(server, token string, stdout, stderr io.Writer) int {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "agentry mcp install:", err)
		return 1
	}
	add := []string{"mcp", "add", "--scope", "user", "agentry", "--", self, "mcp"}
	if server != "" {
		add = append(add, "--server", server, "--token", token)
	}
	exec.Command("claude", "mcp", "remove", "--scope", "user", "agentry").Run() // so it can be installed again
	if out, err := exec.Command("claude", add...).CombinedOutput(); err != nil {
		fmt.Fprintf(stderr, "agentry mcp install: claude mcp add failed: %v\n%s", err, out)
		return 1
	}
	fmt.Fprintln(stdout, "Added Agentry to Claude Code for every repo. Start `claude` in a repo and ask it for your board, or type /mcp__agentry__board.")

	settings := filepath.Join(filepath.Dir(defaultDataDir()), ".claude", "settings.json")
	if err := addHooks(settings, shellQuote(self)+" claude-hook"); err != nil {
		fmt.Fprintf(stderr, "Claude Code's hooks weren't set (%v), so Claude hears what waits for you only when it calls Agentry.\n", err)
	} else {
		fmt.Fprintln(stdout, "Set Claude Code's hooks, so Claude tells you when something needs you, even between your messages.")
	}
	statusLine := map[string]any{"type": "command", "command": shellQuote(self) + " statusline"}
	switch added, err := addStatusLine(settings, statusLine); {
	case err != nil:
		fmt.Fprintf(stderr, "The status line wasn't set (%v). Add this to %s:\n  \"statusLine\": {\"type\": \"command\", \"command\": %q}\n", err, settings, statusLine["command"])
	case added:
		fmt.Fprintln(stdout, "Set Claude Code's status line to show what needs you and what runs.")
	default:
		fmt.Fprintf(stdout, "You have a status line already, so it was left as it is. To show Agentry's, run %s statusline from it.\n", self)
	}
	return 0
}

// addHooks adds Agentry's hooks to Claude Code's settings, once, keeping
// every other hook: what waits for you when a session starts and with each
// message, and after each reply a waiter that wakes Claude when something
// new waits (asyncRewake: exit 2 wakes it).
func addHooks(path, command string) error {
	settings := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("%s isn't JSON I can read", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	add := func(event string, group map[string]any) {
		groups, _ := hooks[event].([]any)
		kept := []any{}
		for _, g := range groups { // an earlier install's goes: this one replaces it
			if data, _ := json.Marshal(g); !strings.Contains(string(data), "claude-hook") {
				kept = append(kept, g)
			}
		}
		hooks[event] = append(kept, group)
	}
	say := map[string]any{"type": "command", "command": command + " context", "timeout": 5}
	add("SessionStart", map[string]any{"hooks": []any{say}})
	add("UserPromptSubmit", map[string]any{"hooks": []any{say}})
	add("Stop", map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command + " rewake", "async": true, "asyncRewake": true,
		"timeout": int(waiterLifetime / time.Second)}}}) // it waits for hours, by design
	settings["hooks"] = hooks
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// addStatusLine sets statusLine in Claude Code's settings if there is none,
// keeping everything else in the file.
func addStatusLine(path string, statusLine map[string]any) (bool, error) {
	settings := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, fmt.Errorf("%s isn't JSON I can read", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if existing, has := settings["statusLine"]; has {
		data, _ := json.Marshal(existing)
		return strings.Contains(string(data), "agentry"), nil // Agentry's is there already, or yours is kept
	}
	settings["statusLine"] = statusLine
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(data, '\n'), 0o644)
}

// `agentry statusline` is the line Claude Code shows under its prompt: what
// needs you, what runs. It prints nothing when no server answers, so a
// stopped Agentry doesn't clutter it.
func runStatusLine(args []string, stdin io.Reader, stdout, _ io.Writer) int {
	io.Copy(io.Discard, stdin) // Claude Code sends its session as JSON; the line is the same for every repo
	agentry, err := connect(os.Getenv("AGENTRY_URL"), os.Getenv("AGENTRY_TOKEN"), defaultDataDir())
	if err != nil {
		return 0
	}
	agentry.Timeout = 2 * time.Second
	var tasks []struct{ Status, CurrentAgent, Kind string }
	var waiting []json.RawMessage
	if agentry.Call("GET", "/api/tasks", nil, &tasks) != nil || agentry.Call("GET", "/api/questions", nil, &waiting) != nil {
		return 0
	}
	running := 0
	for _, t := range tasks {
		if t.Status == "working" && t.CurrentAgent != "" && t.Kind != "epic" {
			running++
		}
	}
	parts := []string{"agentry"}
	switch {
	case len(waiting) == 1:
		parts = append(parts, "\x1b[33m1 needs you\x1b[0m") // yellow, as on the page
	case len(waiting) > 1:
		parts = append(parts, fmt.Sprintf("\x1b[33m%d need you\x1b[0m", len(waiting)))
	}
	parts = append(parts, fmt.Sprintf("%d running", running))
	fmt.Fprintln(stdout, strings.Join(parts, " · "))
	return 0
}
