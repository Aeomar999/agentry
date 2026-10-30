// Package workersapi is the Workers page's HTTP API: the machines, besides
// this one, that agents run on, one of them in detail, a join code and the
// command that uses it, and taking a worker off. It is also what a joined
// worker calls with its credential: its link, a task's repo, updating
// itself and leaving. Web mounts its Routes as it mounts an extension's.
package workersapi

import (
	"context"
	"net/http"
	"slices"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

// State is what the routes read: every task, for the machine its work is
// on, and one, for its repo.
type State interface {
	Tasks() []*tasks.Task
	Task(id string) (*tasks.Task, error)
}

// API holds what the workers' routes need: the log to write join codes and
// removals to and read the workers' history from, the state whose tasks are
// on each machine, the workers and the agents as web reads them, this
// machine, a machine's details read from it, the way to cut a removed
// worker's link, web's install commands for the address the request came
// in on, and for a joined worker the hub's link and streams and web's
// agentry download.
type API struct {
	Log             extensions.Proposer
	Events          extensions.EventLog
	State           State
	Workers         func() (workers.Roster, error)
	Roster          func() (agents.Roster, error)
	Local           workers.Machine                                                // what this machine is, as a worker would say it
	About           func(ctx context.Context, name string) (machines.About, error) // nil when there is no fleet
	Disconnect      func(name string)                                              // nil when no worker can connect
	InstallCommands func(r *http.Request, path, code string) map[string]any

	// The hub's ends of a worker's link: all nil when no worker can connect,
	// and then no route a worker calls is mounted.
	Connect     func(w http.ResponseWriter, r *http.Request, worker workers.Worker)
	Stream      func(w http.ResponseWriter, r *http.Request, worker workers.Worker, id string)
	ServeBinary http.HandlerFunc // agentry for ?os= and ?arch=, with its hash as its ETag
}

// Routes are the workers' endpoints, for the Workers page, and the ones a
// joined worker calls when it can connect.
func (a API) Routes() []extensions.Route {
	routes := []extensions.Route{
		{Pattern: "GET /api/workers", Auth: extensions.UI, Handler: a.listWorkers},
		{Pattern: "POST /api/workers/invite", Auth: extensions.UI, Handler: a.invite},
		{Pattern: "GET /api/workers/{name}", Auth: extensions.UI, Handler: a.showWorker},
		{Pattern: "DELETE /api/workers/{name}", Auth: extensions.UI, Handler: a.removeWorker},
	}
	if a.Connect == nil {
		return routes
	}
	return append(routes,
		extensions.Route{Pattern: "GET " + workerlink.ConnectPath, Auth: extensions.Worker, Handler: a.connect},
		extensions.Route{Pattern: "GET " + workerlink.StreamPath + "{id}", Auth: extensions.Worker, Handler: a.stream},
		extensions.Route{Pattern: "GET " + workerlink.GitPath + "{task}/", Auth: extensions.Worker, Handler: a.gitFetch},
		extensions.Route{Pattern: "POST " + workerlink.GitPath + "{task}/git-upload-pack", Auth: extensions.Worker, Handler: a.gitFetch},
		extensions.Route{Pattern: "POST " + workerlink.GitPath + "{task}/bundle", Auth: extensions.Worker, Handler: a.receiveBundle},
		extensions.Route{Pattern: "GET " + workerlink.UpdatePath, Auth: extensions.Worker, Handler: a.update},
		extensions.Route{Pattern: "POST " + workerlink.LeavePath, Auth: extensions.Worker, Handler: a.leave},
	)
}

// Workers: the machines, besides this one, that agents run on. You make a
// join code on the page; the page shows one command that installs agentry
// on the other machine, joins it with the code, and starts it.

// workerView is a worker as the page shows it, with the tasks whose work is on it.
type workerView struct {
	workers.Worker
	Tasks []string `json:"tasks"`
}

func (a API) listWorkers(w http.ResponseWriter, r *http.Request) {
	roster, err := a.Workers()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	on := TasksByMachine(a.State)
	list := []workerView{}
	for _, worker := range roster.Workers {
		worker.Engines, worker.Missing = listOf(worker.Engines), listOf(worker.Missing)
		list = append(list, workerView{Worker: worker, Tasks: listOf(on[worker.Name])})
	}
	local := a.Local
	local.Engines, local.Missing = listOf(local.Engines), listOf(local.Missing)
	httpapi.Reply(w, map[string]any{"workers": list, "local": map[string]any{"tasks": listOf(on[workers.Local]), "machine": local}})
}

// workerDetail is one machine as the Workers page's side panel shows it.
type workerDetail struct {
	Worker     workerView      `json:"worker"`
	About      *machines.About `json:"about"`      // read from the machine now; nil if it couldn't be
	AboutError string          `json:"aboutError"` // why not
	Agents     []string        `json:"agents"`     // the agents that may run on it
	History    []historyEntry  `json:"history"`    // when it joined, came online and went offline, newest first
}

type historyEntry struct {
	At   time.Time `json:"at"`
	What string    `json:"what"` // joined, online, offline
}

// showWorker is one machine in detail: workers.Local, or a worker's name.
func (a API) showWorker(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	roster, err := a.Workers()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	var detail workerDetail
	if name == workers.Local {
		local := a.Local
		detail.Worker = workerView{Worker: workers.Worker{Name: workers.Local, Host: local.Host, OS: local.OS, Arch: local.Arch,
			Online: true, Version: local.Version, Engines: listOf(local.Engines), Missing: listOf(local.Missing)}}
	} else if worker, ok := roster.Get(name); ok {
		worker.Engines, worker.Missing = listOf(worker.Engines), listOf(worker.Missing)
		detail.Worker = workerView{Worker: worker}
	} else {
		http.Error(w, "no worker called "+name, http.StatusNotFound)
		return
	}
	detail.Worker.Tasks = listOf(TasksByMachine(a.State)[name])

	if a.About != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if about, err := a.About(ctx, name); err != nil {
			detail.AboutError = err.Error()
		} else {
			detail.About = &about
		}
	}

	agentRoster, _ := a.Roster()
	detail.Agents = []string{}
	for _, agent := range agentRoster {
		if slices.Contains(agent.Workers, name) || name == workers.Local && len(agent.Workers) == 0 {
			detail.Agents = append(detail.Agents, agent.Name)
		}
	}

	detail.History = []historyEntry{}
	stream, _ := a.Events.Stream(events.WorkersStream)
	for i := len(stream) - 1; i >= 0 && len(detail.History) < 20; i-- {
		e := stream[i]
		what := map[events.Type]string{events.WorkerJoined: "joined", events.WorkerConnected: "online",
			events.WorkerDisconnected: "offline"}[e.Type]
		if what != "" && e.Text("name") == name {
			detail.History = append(detail.History, historyEntry{At: e.At, What: what})
		}
		if e.Type == events.WorkerJoined && e.Text("name") == name {
			break // what came before was an earlier machine by this name
		}
	}
	httpapi.Reply(w, detail)
}

// TasksByMachine is every unfinished task with a worktree, by the machine it is on.
func TasksByMachine(state State) map[string][]string {
	on := map[string][]string{}
	for _, task := range state.Tasks() {
		if !task.IsDone && task.Worktree != nil {
			on[task.Worktree.Worker] = append(on[task.Worktree.Worker], task.ID)
		}
	}
	return on
}

// invite makes a join code, and the command that uses it.
func (a API) invite(w http.ResponseWriter, r *http.Request) {
	code, p := workers.Invite(httpapi.You, time.Now())
	if _, rejection, err := a.Log.Propose(p); err != nil || rejection != nil {
		httpapi.Failed(w, firstError(err, rejection))
		return
	}
	answer := a.InstallCommands(r, "/worker/install", code)
	answer["code"], answer["expires"] = code, time.Now().Add(workers.InviteLifetime)
	httpapi.Reply(w, answer)
}

func (a API) removeWorker(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	httpapi.Propose(w, a.Log, workers.Removal(httpapi.You, name))
	if a.Disconnect != nil {
		a.Disconnect(name)
	}
}

// firstError is why a proposal failed; it mirrors internal/web/workers_api.go.
func firstError(err error, rejection *events.Rejection) error {
	if err != nil {
		return err
	}
	return rejection
}

// listOf makes nil an empty list, so the page reads [], never null; it
// mirrors internal/web/views.go.
func listOf(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
