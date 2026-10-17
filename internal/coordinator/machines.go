package coordinator

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// A coordinator may choose the machine an agent works on, among the ones
// that agent may run on: Agentry moves the task's work there before the
// turn. Left to Agentry, a turn runs where the work is, if the agent may run
// there, or else on the first of the agent's machines that is online.

// machine is one machine agents can run on, as the coordinator's brief
// shows it.
type machine struct {
	name    string // what a decision calls it: "local" for this one, or a worker's name
	host    string
	os      string
	online  bool
	engines []string
}

// machines are this one and every worker that has joined.
func (l *Loop) machines() []machine {
	host, _ := os.Hostname()
	list := []machine{{name: workers.Local, host: host, os: "this machine, where Agentry runs", online: true}}
	stream, err := l.store.Stream(events.WorkersStream)
	if err != nil {
		return list
	}
	for _, w := range workers.FromEvents(stream).Workers {
		list = append(list, machine{name: w.Name, host: w.Host, os: w.OS + "/" + w.Arch, online: w.Online, engines: w.Engines})
	}
	return list
}

// machineFor is the machine a decision named, by its name or its host's,
// if the agent may run there; refused says why not.
func (l *Loop) machineFor(agent agents.Agent, wanted string) (name, refused string) {
	wanted = strings.TrimSpace(wanted)
	if wanted == "" {
		return "", ""
	}
	all := l.machines()
	i := slices.IndexFunc(all, func(m machine) bool { return strings.EqualFold(m.name, wanted) || strings.EqualFold(m.host, wanted) })
	if i < 0 {
		var names []string
		for _, m := range all {
			names = append(names, m.name)
		}
		return "", "there is no machine called " + wanted + "; the machines are " + strings.Join(names, ", ")
	}
	name = all[i].name
	if allowed := mayRunOn(agent); !slices.Contains(allowed, name) {
		return "", fmt.Sprintf("%s may not run on %s; it runs on %s. Choose one of those, or ask the user to let it run on %s",
			agent.Name, name, strings.Join(allowed, ", "), name)
	}
	return name, ""
}

// mayRunOn are the machines the agent may run on: this one when it names none.
func mayRunOn(agent agents.Agent) []string {
	if len(agent.Workers) == 0 {
		return []string{workers.Local}
	}
	return agent.Workers
}

// machinesText is the brief's list of machines: which are online, what they
// have, which of the team's agents may run on each, and where the work is.
func machinesText(task *tasks.Task, list []machine, team []agents.Agent) string {
	if len(list) == 1 {
		return "" // only this machine: nothing to choose
	}
	var b strings.Builder
	for _, m := range list {
		state := "online"
		if !m.online {
			state = "offline"
		}
		var who []string
		for _, agent := range team {
			if slices.Contains(mayRunOn(agent), m.name) {
				who = append(who, agent.Name)
			}
		}
		fmt.Fprintf(&b, "- %s (host %s, %s), %s", m.name, m.host, m.os, state)
		if len(m.engines) > 0 {
			fmt.Fprintf(&b, ", has %s", strings.Join(m.engines, ", "))
		}
		if len(who) > 0 {
			fmt.Fprintf(&b, "; may run %s", strings.Join(who, ", "))
		}
		b.WriteString("\n")
	}
	where := workers.Local
	if task.Worktree != nil && task.Worktree.Worker != "" {
		where = task.Worktree.Worker
	}
	fmt.Fprintf(&b, "\nThe task's work is on %s now. Unless you choose, each turn runs where the work is, if the agent may run "+
		"there, or else on the first of the agent's machines that is online. To choose, add \"machine\" to an assign, with a "+
		"machine above that the agent may run on: Agentry moves the work there first, with everything committed on its branch. "+
		"Choose when the work needs what only one machine has (an operating system, a tool, a device, a service it reaches), "+
		"and keep a task's work on one machine unless it must move. Ask an agent to check what a machine has when you aren't sure.", where)
	return b.String()
}
