package tasksapi

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/waitlist"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
	"gitlab.com/dev.chempa/agentry/internal/platform/schema"
	"gitlab.com/dev.chempa/agentry/internal/platform/worktrees"
	"gitlab.com/dev.chempa/agentry/internal/runtime/coordinator"
)

// Client is what the task, epic and question tools need of agentry mcp,
// which fills it: Agentry is the server, and Repo the repo Claude Code runs
// in, "" outside one. Agentry is a client.Client, not an extensions.API,
// as create_task waits longer for a repo to be cloned. The tools that need
// your decision ask you through extensions.AskOf, as an extension's do.
type Client struct {
	Agentry client.Client
	Repo    string
}

// Task is the part of a task on the board, as GET /api/tasks lists it, that
// the tools and the news read.
type Task struct {
	ID, Title, Repo, Status, CurrentAgent, NeedsYou, Kind, Epic, Parked, Closing, Column string
	Labels, BlockedBy                                                                    []string
	Pushed, DoneByYou                                                                    bool
	Stories, StoriesDone                                                                 int
	Budget, TokensUsed, MaxTurns, Turns, TurnsSinceAnswer                                int // a limit of 0 is none
	Rank                                                                                 float64
	Told                                                                                 *struct{ At, From, Text string }
}

type detail struct {
	Task
	Description, Summary, Coordinator, TokensUnknown string
	Archived                                         bool // read back from its archive: it takes no events
	ArchivedAt                                       time.Time
	Trail                                            []struct{ From, To, Text string }
	Shared                                           []struct {
		Name, By, From string
		Size           int64
	}
}

// question is the part of a question waiting for the user that the tools read.
type question struct {
	waitlist.Question
	Tool, Reason string
	Held         bool // approval: no answer came in time, so the agent was told no
	HeldAt       time.Time
}

// Tools read the board and hand work to Agentry, from Claude Code.
func (c Client) Tools() []extensions.Tool {
	return []extensions.Tool{
		{Name: "status", Description: "What needs the user, what is running and what is planned, for this repo (or all repos). Use it when the user asks how things stand.",
			Input: schema.Object(schema.Props{"all": schema.Boolean("every repo, not only this one")}), Run: c.status},
		{Name: "list_tasks", Description: "Every task of this repo (or all repos), newest first, with its status, epic and what it waits for.",
			Input: schema.Object(schema.Props{"all": schema.Boolean("every repo, not only this one"), "status": schema.Text("only this status: working, needs you, held, done")}), Run: c.listTasks},
		{Name: "show_task", Description: "One task: its description, what it waits for, its summary when done, and its latest steps.",
			Input: schema.Object(schema.Props{"task": schema.Text("its id, such as task-4 (or epic-2 for an epic)")}, "task"), Run: c.showTask},
		{Name: "create_task", Description: "Hand work to Agentry: a task (a story), or an epic of stories. It works in this repo unless repo says otherwise. Show the user the title and description before creating it, unless they said to go ahead.",
			Input: schema.Object(schema.Props{
				"title":       schema.Text("one imperative line under 80 characters"),
				"description": schema.Text("what to change and why, the areas involved, and how to tell it works"),
				"kind":        schema.Enum("story: one change, never split; epic: split into stories; leave it out to let the coordinator decide", "story", "epic"),
				"team":        schema.Text("the team that does it, such as Software or Writing; leave it out to let the coordinators decide"),
				"labels":      schema.Texts("one to three short lowercase words, such as bug or docs"),
				"epic":        schema.Text("for a new story of an epic: the epic's id"),
				"stories":     schema.Texts("for a new epic: ids of tasks there already, to file under it"),
				"repo":        schema.Text("when not this one: the repo's absolute path, a git address to clone (git@host:team/app.git, https://...), or a link to a .zip or .tar.gz of it"),
				"budget":      schema.Integer("the tokens it may use before it asks the user, such as 2000000; -1 for no limit; leave it out for the server's default"),
				"maxTurns":    schema.Integer("the turns it may have without the user's answer before it asks them; -1 for no limit; leave it out for the server's default"),
			}, "title"), Run: c.createTask},
		{Name: "tell_coordinator", Description: "Tell a task's coordinator what the user said: a change of direction, more context, or a request. The turn in progress stops so it reads it at once.",
			Input: schema.Object(schema.Props{"task": schema.Text("its id"), "text": schema.Text("the user's words, with what the coordinator needs to act on them")}, "task", "text"), Run: c.tell},
		{Name: "answer_question", Description: "Send the user's answer to a coordinator's question. Only ever their own answer.",
			Input: schema.Object(schema.Props{"task": schema.Text("its id"), "answer": schema.Text("the user's answer")}, "task", "answer"), Run: c.answer},
		{Name: "close_epic", Description: "Ask the user, directly in Claude Code, whether to close an epic whose stories are all done, whether it is asking or they left it open. Only they can.",
			Input: schema.Object(schema.Props{"epic": schema.Text("the epic's id")}, "epic"), Run: c.closeEpic},
		{Name: "review_approvals", Description: "Ask the user, directly in Claude Code, to approve or deny each action an agent is waiting to take (of one task, or all). Only they can.",
			Input: schema.Object(schema.Props{"task": schema.Text("only this task's")}), Run: c.reviewApprovals},
		{Name: "file_under_epic", Description: "Make a task a story of an epic, move it to another, or with an empty epic take it out of its epic.",
			Input: schema.Object(schema.Props{"task": schema.Text("its id"), "epic": schema.Text(`the epic's id, or "" to take it out`)}, "task", "epic"), Run: c.fileUnder},
		{Name: "set_priority", Description: "Put tasks (such as an epic's stories) in this priority order, first first.",
			Input: schema.Object(schema.Props{"tasks": schema.Texts("their ids, in the order wanted")}, "tasks"), Run: c.setPriority},
		{Name: "start", Description: "Start tasks waiting in the user's Backlog or To do, such as an epic's stories, in the order given.",
			Input: schema.Object(schema.Props{"tasks": schema.Texts("their ids")}, "tasks"), Run: c.start},
	}
}

// ---- reading -------------------------------------------------------------------

func (c Client) status(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ All bool }
	json.Unmarshal(arguments, &a)
	all, waiting, err := c.board()
	if err != nil {
		return "", err
	}
	shown, idOf := c.inScope(all, a.All), ShownIDs(all)
	var b strings.Builder
	fmt.Fprintf(&b, "Agentry board for %s (%s)\n", c.scope(a.All), c.Agentry.PageURL("/tasks"))
	// What needs you is everywhere: it waits for you whichever repo it is in.
	repoOf := map[string]string{}
	for _, t := range all {
		repoOf[t.ID] = t.Repo
	}
	section(&b, "Needs you", lines(waiting, func(q question) (string, bool) {
		if repo := repoOf[q.Task]; c.Repo != "" && repo != c.Repo {
			return waitlist.Line(q.Question) + " [in " + repo + "]", true
		}
		return waitlist.Line(q.Question), true
	}))
	section(&b, "Running", lines(shown, func(t Task) (string, bool) {
		if t.Status != "working" || t.Kind == "epic" {
			return "", false
		}
		what := "the coordinator is next"
		if t.CurrentAgent != "" {
			what = t.CurrentAgent + " is working"
		}
		if t.Column != "" {
			what += " (" + tasks.ColumnName(t.Column) + ")"
		}
		if len(t.BlockedBy) > 0 {
			what = "waits for " + strings.Join(IDsAs(t.BlockedBy, idOf), ", ")
		}
		return t.Shown() + " " + t.Title + ": " + what + " · " + usageOf(t), true
	}))
	section(&b, "Epics", lines(shown, func(t Task) (string, bool) {
		if t.Closing != "" && t.Status == "working" {
			return fmt.Sprintf("%s %s: all %d stories done, open until the user closes it (close_epic) or adds a story", t.Shown(), t.Title, t.Stories), t.Kind == "epic"
		}
		return fmt.Sprintf("%s %s: %d of %d stories done", t.Shown(), t.Title, t.StoriesDone, t.Stories), t.Kind == "epic" && t.Status != "done"
	}))
	section(&b, "Not started (in the Backlog or To do)", lines(shown, func(t Task) (string, bool) {
		line := t.Shown() + " " + t.Title
		if t.Epic != "" {
			line += " (story of " + tasks.ShownEpic(t.Epic) + ")"
		}
		return line, t.Status == "held"
	}))
	return b.String(), nil
}

func (c Client) listTasks(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct {
		All    bool
		Status string
	}
	json.Unmarshal(arguments, &a)
	all, _, err := c.board()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, t := range c.inScope(all, a.All) {
		if a.Status != "" && t.Status != a.Status {
			continue
		}
		status := t.Status
		if t.Column != "" && t.Column != "done" { // done says it already
			status += " · " + tasks.ColumnName(t.Column)
		}
		fmt.Fprintf(&b, "%s [%s] %s", t.Shown(), status, t.Title)
		switch {
		case t.Kind == "epic":
			fmt.Fprintf(&b, " (epic, %d of %d stories done)", t.StoriesDone, t.Stories)
		case t.Epic != "":
			fmt.Fprintf(&b, " (story of %s)", tasks.ShownEpic(t.Epic))
		}
		if t.Kind != "epic" { // an epic's work is its stories'
			fmt.Fprintf(&b, " · %s", usageOf(t))
		}
		if len(t.Labels) > 0 {
			fmt.Fprintf(&b, " {%s}", strings.Join(t.Labels, ", "))
		}
		if t.NeedsYou != "" {
			fmt.Fprintf(&b, "\n    needs you: %s", tasks.OneLine(t.NeedsYou, 200))
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "No tasks for " + c.scope(a.All) + ".", nil
	}
	return b.String(), nil
}

func (c Client) showTask(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task string }
	json.Unmarshal(arguments, &a)
	a.Task = tasks.StoredID(a.Task)
	var t detail
	if err := c.Agentry.Call("GET", "/api/tasks/"+a.Task, nil, &t); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\nStatus: %s · %s · coordinator %s · repo %s\nOn the page: %s\n", t.Shown(), t.Title, statusOf(t.Task), columnOf(t.Task), t.Coordinator, t.Repo,
		c.Agentry.PageURL("/tasks/"+t.ID))
	if t.Status == "done" && t.DoneByYou {
		b.WriteString("Moved to Done by you.\n")
	}
	if t.Archived {
		fmt.Fprintf(&b, "Archived on %s: it can be read, not changed.\n", t.ArchivedAt.Format(time.DateOnly))
	}
	if t.Kind == "epic" {
		fmt.Fprintf(&b, "An epic: %d of %d stories done.\n", t.StoriesDone, t.Stories)
	} else if t.Epic != "" {
		fmt.Fprintf(&b, "A story of %s.\n", tasks.ShownEpic(t.Epic))
	}
	fmt.Fprintf(&b, "Budget: %s\n", usageOf(t.Task))
	if t.TokensUnknown != "" {
		fmt.Fprintf(&b, "%s\n", t.TokensUnknown)
	}
	if t.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", t.Description)
	}
	if t.NeedsYou != "" {
		fmt.Fprintf(&b, "\nWaiting for the user: %s\n", t.NeedsYou)
	}
	if t.Summary != "" {
		fmt.Fprintf(&b, "\nSummary: %s\n", t.Summary)
	}
	if len(t.Shared) > 0 {
		b.WriteString("\nShared files (get_file downloads one):\n")
		for _, f := range t.Shared {
			fmt.Fprintf(&b, "- %s (%d bytes, %s)\n", f.Name, f.Size, firstOf(f.From, f.By))
		}
	}
	if len(t.Trail) > 0 {
		b.WriteString("\nLatest steps:\n")
		for _, step := range t.Trail[max(0, len(t.Trail)-8):] {
			fmt.Fprintf(&b, "- %s → %s: %s\n", step.From, step.To, tasks.OneLine(step.Text, 300))
		}
	}
	return b.String(), nil
}

// statusOf is the task's status, with a done task whose work an agent
// pushed said to be shipped.
func statusOf(t Task) string {
	if t.Status == "done" && t.Pushed {
		return "shipped"
	}
	return t.Status
}

// columnOf is where the task shows on the board: "on the board: Shipping".
func columnOf(t Task) string {
	return "on the board: " + tasks.ColumnName(t.Column)
}

// usageOf is what the task has used of its budget and its turns, shown
// with no limit too: "1.2M tokens (no limit) · 14 turns (no limit)".
func usageOf(t Task) string { return budgetOf(t) + " · " + turnsOf(t) }

// budgetOf is what the task has used of its budget: "1.2M of 5M tokens".
func budgetOf(t Task) string {
	if t.Budget == 0 {
		return coordinator.Tokens(t.TokensUsed) + " tokens (no limit)"
	}
	return coordinator.Tokens(t.TokensUsed) + " of " + coordinator.Tokens(t.Budget) + " tokens"
}

// turnsOf is how many turns the task has had since the user last answered,
// against its limit: "18 of 24 turns"; with no limit, all its turns.
func turnsOf(t Task) string {
	if t.MaxTurns == 0 {
		return fmt.Sprintf("%d turns (no limit)", t.Turns)
	}
	return fmt.Sprintf("%d of %d turns", t.TurnsSinceAnswer, t.MaxTurns)
}

// ---- handing over work ---------------------------------------------------------

func (c Client) createTask(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct {
		Title, Description, Kind, Team, Epic, Repo string
		Labels, Stories                            []string
		Budget, MaxTurns                           int // 0, left out, takes the server's default; -1 is no limit
	}
	if err := json.Unmarshal(arguments, &a); err != nil {
		return "", err
	}
	if a.Repo == "" {
		a.Repo = c.Repo
	}
	a.Epic, a.Stories = tasks.StoredID(a.Epic), IDsAs(a.Stories, tasks.StoredID)
	if a.Epic != "" {
		a.Repo = "" // a story works where its epic does
	}
	var created struct {
		Stream   string
		NotFiled []string
	}
	agentry := c.Agentry
	agentry.Timeout = worktrees.CloneTimeout + time.Minute // a repo named by its address is cloned before the task is made
	err := agentry.Call("POST", "/api/tasks", map[string]any{"title": a.Title, "description": a.Description, "kind": a.Kind,
		"team": a.Team, "labels": a.Labels, "epic": a.Epic, "stories": a.Stories, "repo": a.Repo,
		"budget": a.Budget, "maxTurns": a.MaxTurns}, &created)
	if err != nil {
		return "", err
	}
	text := fmt.Sprintf("Created %s: %s\n%s", tasks.ShownID(created.Stream, tasks.Kind(a.Kind)), a.Title, c.Agentry.PageURL("/tasks/"+created.Stream))
	switch {
	case a.Epic != "":
		text += "\nIt waits in the user's Backlog until they start it."
	case a.Kind == "epic":
		text += "\nIts coordinator splits it into stories next; they wait in the user's Backlog until started."
	default:
		text += "\nIts coordinator reads it next."
	}
	for _, refused := range created.NotFiled {
		text += "\nNot filed: " + refused
	}
	return text, nil
}

func (c Client) tell(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task, Text string }
	json.Unmarshal(arguments, &a)
	a.Task = tasks.StoredID(a.Task)
	if err := c.Agentry.Call("POST", "/api/tasks/"+a.Task+"/message", map[string]string{"text": a.Text}, nil); err != nil {
		return "", err
	}
	return "Sent to " + c.ShownNow()(a.Task) + "'s coordinator. It reads it now; the task's chat shows it.", nil
}

func (c Client) answer(ctx context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task, Answer string }
	json.Unmarshal(arguments, &a)
	// Closing is the user's to approve, asked or not: an epic left open has
	// no question waiting, yet this answer would still close it.
	if closes(a.Answer) {
		return c.closeEpic(ctx, json.RawMessage(fmt.Sprintf(`{"epic": %q}`, a.Task)))
	}
	a.Task = tasks.StoredID(a.Task)
	if q, ok := c.questionOf(a.Task); ok && copies(q, a.Answer) {
		return c.approveCopy(ctx, q)
	}
	if err := c.Agentry.Call("POST", "/api/tasks/"+a.Task+"/answer", map[string]string{"answer": a.Answer}, nil); err != nil {
		return "", err
	}
	return "Answered " + c.ShownNow()(a.Task) + ". Its coordinator carries on.", nil
}

func (c Client) fileUnder(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task, Epic string }
	json.Unmarshal(arguments, &a)
	a.Task, a.Epic = tasks.StoredID(a.Task), tasks.StoredID(a.Epic)
	if err := c.Agentry.Call("PUT", "/api/tasks/"+a.Task+"/epic", map[string]string{"epic": a.Epic}, nil); err != nil {
		return "", err
	}
	if a.Epic == "" {
		return a.Task + " is out of its epic.", nil
	}
	return a.Task + " is a story of " + tasks.ShownEpic(a.Epic) + ".", nil
}

// setPriority gives the tasks their own ranks back, in the order wanted, so
// they move only among themselves.
func (c Client) setPriority(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Tasks []string }
	json.Unmarshal(arguments, &a)
	a.Tasks = IDsAs(a.Tasks, tasks.StoredID)
	all, _, err := c.board()
	if err != nil {
		return "", err
	}
	idOf := ShownIDs(all)
	var ranks []float64
	for _, id := range a.Tasks {
		i := slices.IndexFunc(all, func(t Task) bool { return t.ID == id })
		if i < 0 {
			return "", fmt.Errorf("no task %s", id)
		}
		ranks = append(ranks, all[i].Rank)
	}
	sort.Float64s(ranks)
	for i, id := range a.Tasks {
		if err := c.Agentry.Call("PUT", "/api/tasks/"+id+"/rank", map[string]float64{"rank": ranks[i]}, nil); err != nil {
			return "", fmt.Errorf("%s: %w", idOf(id), err)
		}
	}
	return "In this order now: " + strings.Join(IDsAs(a.Tasks, idOf), ", ") + ".", nil
}

func (c Client) start(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Tasks []string }
	json.Unmarshal(arguments, &a)
	a.Tasks = IDsAs(a.Tasks, tasks.StoredID)
	idOf := c.ShownNow()
	for _, id := range a.Tasks {
		if err := c.Agentry.Call("POST", "/api/tasks/"+id+"/direct", map[string]string{"to": "in-progress"}, nil); err != nil {
			return "", fmt.Errorf("%s: %w", idOf(id), err)
		}
	}
	return "Started " + strings.Join(IDsAs(a.Tasks, idOf), ", ") + ". They are on the Tasks board now.", nil
}

// ---- the board -----------------------------------------------------------------

// board is every task, and everything waiting for you.
func (c Client) board() ([]Task, []question, error) {
	var all []Task
	if err := c.Agentry.Call("GET", "/api/tasks", nil, &all); err != nil {
		return nil, nil, err
	}
	var waiting []question
	if err := c.Agentry.Call("GET", "/api/questions", nil, &waiting); err != nil {
		return nil, nil, err
	}
	return all, waiting, nil
}

func (c Client) questionOf(id string) (question, bool) {
	_, waiting, err := c.board()
	if err != nil {
		return question{}, false
	}
	for _, q := range waiting {
		if q.Task == id && q.Kind != "approval" {
			return q, true
		}
	}
	return question{}, false
}

// inScope are the tasks of this repo, or all of them.
func (c Client) inScope(all []Task, everywhere bool) []Task {
	if everywhere || c.Repo == "" {
		return all
	}
	var mine []Task
	for _, t := range all {
		if t.Repo == c.Repo {
			mine = append(mine, t)
		}
	}
	return mine
}

func (c Client) scope(everywhere bool) string {
	if everywhere || c.Repo == "" {
		return "every repo"
	}
	return c.Repo
}

// ---- small helpers -------------------------------------------------------------

func section(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:\n", title)
	for _, item := range items {
		b.WriteString("- " + item + "\n")
	}
}

func lines[T any](items []T, line func(T) (string, bool)) []string {
	var out []string
	for _, item := range items {
		if text, ok := line(item); ok {
			out = append(out, text)
		}
	}
	return out
}

// Shown is the task's id as you read it: epic-N for an epic.
func (t Task) Shown() string { return tasks.ShownID(t.ID, tasks.Kind(t.Kind)) }

// shown is the id of the task q waits on, as you read it.
func (q question) shown() string { return tasks.ShownID(q.Task, tasks.Kind(q.TaskKind)) }

// ShownIDs gives each task's id as you read it, by its kind on the board
// all; an id not on it stays as it is.
func ShownIDs(all []Task) func(id string) string {
	kinds := map[string]tasks.Kind{}
	for _, t := range all {
		kinds[t.ID] = tasks.Kind(t.Kind)
	}
	return func(id string) string { return tasks.ShownID(id, kinds[id]) }
}

// ShownNow is shownIDs of the board now: when it can't be read, every id
// stays as it is.
func (c Client) ShownNow() func(id string) string {
	var all []Task
	c.Agentry.Call("GET", "/api/tasks", nil, &all)
	return ShownIDs(all)
}

// IDsAs are the ids, each as id gives it, such as tasks.StoredID.
func IDsAs(ids []string, id func(string) string) []string {
	var out []string
	for _, each := range ids {
		out = append(out, id(each))
	}
	return out
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
