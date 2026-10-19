// Command agentry is the whole of Agentry in one binary: the server, the
// worker that runs agents on a machine, the task command the desk uses, and
// the hook the engines call.
package main

import (
	"fmt"
	"io"
	"os"
)

// command is one subcommand. run gets the arguments after its name.
type command struct {
	name    string
	summary string
	run     func(args []string, stdin io.Reader, stdout, stderr io.Writer) int
}

var commands = []command{
	{"open", "start Agentry here in the background if it isn't running, and open it in your browser", runOpen},
	{"stop", "stop the Agentry that open started", runStopServer},
	{"ca", "show the fingerprint of Agentry's own HTTPS CA, or replace it with -regenerate", runCA},
	{"serve", "run Agentry: the page, the coordinator, the agents", runServe},
	{"worker", "run agents on this machine for a server; join it first", runWorker},
	{"task", "create, list, show or answer tasks; used by the desk", runTask},
	{"send", "send a task's coordinator what you asked; used by Claude in a task's Workspace", runSend},
	{"mcp", "Agentry's tools for Claude Code in any repo; `agentry mcp install` adds them", runMCP},
	{"statusline", "the line Claude Code shows: what needs you, what runs", runStatusLine},
	{"wait", "wait until something new needs you or a task finishes, then say what", runWait},
	{"claude-hook", "called by Claude Code's hooks, so it hears from Agentry; not run by hand", runClaudeHook},
	{"hook", "called by an engine's hooks; not run by hand", runHook},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printUsage(stdout)
		return 0
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(args[1:], stdin, stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "agentry: unknown command %q\n\n", args[0])
	printUsage(stderr)
	return 2
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: agentry <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-8s %s\n", c.name, c.summary)
	}
}
