// Command agentry is the whole of Agentry in one binary: the server, the
// worker that runs agents on a machine, and the hook the engines call.
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
	{"serve", "run Agentry: the page, the coordinator, the agents", runServe},
	{"worker", "run agents on this machine for the server", notBuilt("worker", "T-022")},
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

// notBuilt stands in for a command until the task that builds it lands.
// It exits 2 so that a hook wired up too early fails closed (proof 01).
func notBuilt(name, task string) func([]string, io.Reader, io.Writer, io.Writer) int {
	return func(_ []string, _ io.Reader, _, stderr io.Writer) int {
		fmt.Fprintf(stderr, "agentry %s: not built yet (%s in PLAN.md)\n", name, task)
		return 2
	}
}
