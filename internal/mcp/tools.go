package mcp

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/coordinator"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// A tool is one thing Claude can do with Agentry.
type tool struct {
	name, description string
	input             map[string]any // JSON Schema of its arguments
	run               func(ctx context.Context, arguments json.RawMessage) (string, error)
}

// The parts of the API's tasks and questions the tools read.
type task struct {
	ID, Title, Repo, Status, CurrentAgent, NeedsYou, Kind, Epic, Parked, Closing, Column string
	Labels, BlockedBy                                                                    []string
	Stories, StoriesDone                                                                 int
	Rank                                                                                 float64
	Told                                                                                 *struct{ At, From, Text string }
}

type detail struct {
	task
	Description, Summary, Coordinator              string
	Budget, TokensUsed, MaxTurns, TurnsSinceAnswer int // a limit of 0 is none
	Trail                                          []struct{ From, To, Text string }
	Shared                                         []struct {
		Name, By, From string
		Size           int64
	}
}

type question struct {
	Kind, Task, TaskTitle, Agent, Text, Action, Tool, Reason string
	Options                                                  []string
}

func (s *Server) tools() []tool {
	return []tool{
		{"status", "What needs the user, what is running and what is planned, for this repo (or all repos). Use it when the user asks how things stand.",
			object(props{"all": boolean("every repo, not only this one")}), s.status},
		{"list_tasks", "Every task of this repo (or all repos), newest first, with its status, epic and what it waits for.",
			object(props{"all": boolean("every repo, not only this one"), "status": text("only this status: working, needs you, held, done")}), s.listTasks},
		{"show_task", "One task: its description, what it waits for, its summary when done, and its latest steps.",
			object(props{"task": text("its id, such as task-4")}, "task"), s.showTask},
		{"create_task", "Hand work to Agentry: a task (a story), or an epic of stories. It works in this repo unless repo says otherwise. Show the user the title and description before creating it, unless they said to go ahead.",
			object(props{
				"title":       text("one imperative line under 80 characters"),
				"description": text("what to change and why, the areas involved, and how to tell it works"),
				"kind":        enum("story: one change, never split; epic: split into stories; leave it out to let the coordinator decide", "story", "epic"),
				"team":        text("the team that does it, such as Software or Writing; leave it out to let the coordinators decide"),
				"labels":      texts("one to three short lowercase words, such as bug or docs"),
				"epic":        text("for a new story of an epic: the epic's id"),
				"stories":     texts("for a new epic: ids of tasks there already, to file under it"),
				"repo":        text("when not this one: the repo's absolute path, a git address to clone (git@host:team/app.git, https://...), or a link to a .zip or .tar.gz of it"),
				"budget":      integer("the tokens it may use before it asks the user, such as 2000000; -1 for no limit; leave it out for the server's default"),
				"maxTurns":    integer("the turns it may have without the user's answer before it asks them; -1 for no limit; leave it out for the server's default"),
			}, "title"), s.createTask},
		{"tell_coordinator", "Tell a task's coordinator what the user said: a change of direction, more context, or a request. The turn in progress stops so it reads it at once.",
			object(props{"task": text("its id"), "text": text("the user's words, with what the coordinator needs to act on them")}, "task", "text"), s.tell},
		{"answer_question", "Send the user's answer to a coordinator's question. Only ever their own answer.",
			object(props{"task": text("its id"), "answer": text("the user's answer")}, "task", "answer"), s.answer},
		{"close_epic", "Ask the user, directly in Claude Code, whether to close an epic whose stories are all done, whether it is asking or they left it open. Only they can.",
			object(props{"epic": text("the epic's id")}, "epic"), s.closeEpic},
		{"review_approvals", "Ask the user, directly in Claude Code, to approve or deny each action an agent is waiting to take (of one task, or all). Only they can.",
			object(props{"task": text("only this task's")}), s.reviewApprovals},
		{"file_under_epic", "Make a task a story of an epic, move it to another, or with an empty epic take it out of its epic.",
			object(props{"task": text("its id"), "epic": text(`the epic's id, or "" to take it out`)}, "task", "epic"), s.fileUnder},
		{"set_priority", "Put tasks (such as an epic's stories) in this priority order, first first.",
			object(props{"tasks": texts("their ids, in the order wanted")}, "tasks"), s.setPriority},
		{"start", "Start tasks waiting in the user's Backlog or To do, such as an epic's stories, in the order given.",
			object(props{"tasks": texts("their ids")}, "tasks"), s.start},
		{"attach_file", "Share a file on this machine with a task: its agents find it in .agentry/files, on whichever machine works on it. For data, documents or builds that aren't in git.",
			object(props{"task": text("its id"), "path": text("the file's path here"), "as": text("optional: the name to share it by, such as data/sales.csv")}, "task", "path"), s.attachFile},
		{"get_file", "Download one of a task's shared files to this machine.",
			object(props{"task": text("its id"), "name": text("the shared file's name"), "to": text("optional: where to save it; this folder if left out")}, "task", "name"), s.getFile},
		{"list_ideas", "The features the Visionaries suggest for this repo (or all repos), newest first: open ones unless state says otherwise. Relay them to the user; the user decides on each.",
			object(props{"all": boolean("every repo, not only this one"), "state": enum("which ideas: open (the default), accepted, dismissed or all", "open", "accepted", "dismissed", "all")}), s.listIdeas},
		{"accept_idea", "Make an idea a task in the user's Backlog. Only with the user's own decision, never on your own judgement: Claude Code asks the user to confirm when it can, and otherwise acts on these arguments.",
			object(props{"id": text("the idea's id, such as idea-3"), "kind": enum("task (one change) or epic; leave it out for what the idea suggests", "task", "epic"),
				"note": text("optional: what the user said, for the task's coordinator")}, "id"), s.acceptIdea},
		{"dismiss_idea", "Dismiss an idea. Only with the user's own decision, never on your own judgement: Claude Code asks the user to confirm when it can, and otherwise acts on these arguments.",
			object(props{"id": text("the idea's id"), "note": text("optional: the user's reason, so the Visionaries don't suggest it again")}, "id"), s.dismissIdea},
	}
}

// ---- reading -------------------------------------------------------------------

func (s *Server) status(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ All bool }
	json.Unmarshal(arguments, &a)
	all, waiting, err := s.board()
	if err != nil {
		return "", err
	}
	shown := s.inScope(all, a.All)
	var b strings.Builder
	fmt.Fprintf(&b, "Agentry board for %s (%s)\n", s.scope(a.All), s.agentry.PageURL("/tasks"))
	// What needs you is everywhere: it waits for you whichever repo it is in.
	repoOf := map[string]string{}
	for _, t := range all {
		repoOf[t.ID] = t.Repo
	}
	section(&b, "Needs you", lines(waiting, func(q question) (string, bool) {
		if repo := repoOf[q.Task]; s.repo != "" && repo != s.repo {
			return describe(q) + " [in " + repo + "]", true
		}
		return describe(q), true
	}))
	section(&b, "Running", lines(shown, func(t task) (string, bool) {
		if t.Status != "working" || t.Kind == "epic" {
			return "", false
		}
		what := "the coordinator is next"
		if t.CurrentAgent != "" {
			what = t.CurrentAgent + " is working"
		}
		if len(t.BlockedBy) > 0 {
			what = "waits for " + strings.Join(t.BlockedBy, ", ")
		}
		return t.ID + " " + t.Title + ": " + what, true
	}))
	section(&b, "Epics", lines(shown, func(t task) (string, bool) {
		if t.Closing != "" && t.Status == "working" {
			return fmt.Sprintf("%s %s: all %d stories done, open until the user closes it (close_epic) or adds a story", t.ID, t.Title, t.Stories), t.Kind == "epic"
		}
		return fmt.Sprintf("%s %s: %d of %d stories done", t.ID, t.Title, t.StoriesDone, t.Stories), t.Kind == "epic" && t.Status != "done"
	}))
	section(&b, "Not started (in the Backlog or To do)", lines(shown, func(t task) (string, bool) {
		line := t.ID + " " + t.Title
		if t.Epic != "" {
			line += " (story of " + t.Epic + ")"
		}
		return line, t.Status == "held"
	}))
	return b.String(), nil
}

func (s *Server) listTasks(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct {
		All    bool
		Status string
	}
	json.Unmarshal(arguments, &a)
	all, _, err := s.board()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, t := range s.inScope(all, a.All) {
		if a.Status != "" && t.Status != a.Status {
			continue
		}
		fmt.Fprintf(&b, "%s [%s] %s", t.ID, t.Status, t.Title)
		switch {
		case t.Kind == "epic":
			fmt.Fprintf(&b, " (epic, %d of %d stories done)", t.StoriesDone, t.Stories)
		case t.Epic != "":
			fmt.Fprintf(&b, " (story of %s)", t.Epic)
		}
		if len(t.Labels) > 0 {
			fmt.Fprintf(&b, " {%s}", strings.Join(t.Labels, ", "))
		}
		if t.NeedsYou != "" {
			fmt.Fprintf(&b, "\n    needs you: %s", oneLine(t.NeedsYou, 200))
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "No tasks for " + s.scope(a.All) + ".", nil
	}
	return b.String(), nil
}

func (s *Server) showTask(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task string }
	json.Unmarshal(arguments, &a)
	var t detail
	if err := s.agentry.Call("GET", "/api/tasks/"+a.Task, nil, &t); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\nStatus: %s · %s · coordinator %s · repo %s\nOn the page: %s\n", t.ID, t.Title, t.Status, columnOf(t.task), t.Coordinator, t.Repo,
		s.agentry.PageURL("/tasks/"+t.ID))
	if t.Kind == "epic" {
		fmt.Fprintf(&b, "An epic: %d of %d stories done.\n", t.StoriesDone, t.Stories)
	} else if t.Epic != "" {
		fmt.Fprintf(&b, "A story of %s.\n", t.Epic)
	}
	fmt.Fprintf(&b, "Budget: %s · %s\n", budgetOf(t), turnsOf(t))
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
			fmt.Fprintf(&b, "- %s → %s: %s\n", step.From, step.To, oneLine(step.Text, 300))
		}
	}
	return b.String(), nil
}

// columnOf is where the task shows on the board: "on the board: Shipping".
func columnOf(t task) string {
	names := map[string]string{"backlog": "Backlog", "todo": "To do", "in-progress": "In progress", "review": "Review", "shipping": "Shipping", "done": "Done"}
	return "on the board: " + cmp.Or(names[t.Column], t.Column)
}

// budgetOf is what the task has used of its budget: "1.2M of 5M tokens".
func budgetOf(t detail) string {
	if t.Budget == 0 {
		return coordinator.Tokens(t.TokensUsed) + " tokens, no limit"
	}
	return coordinator.Tokens(t.TokensUsed) + " of " + coordinator.Tokens(t.Budget) + " tokens"
}

// turnsOf is how many turns the task has had since the user last answered,
// against its limit: "18 of 24 turns".
func turnsOf(t detail) string {
	if t.MaxTurns == 0 {
		return fmt.Sprintf("%d turns, no limit", t.TurnsSinceAnswer)
	}
	return fmt.Sprintf("%d of %d turns", t.TurnsSinceAnswer, t.MaxTurns)
}

// ---- handing over work ---------------------------------------------------------

func (s *Server) createTask(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct {
		Title, Description, Kind, Team, Epic, Repo string
		Labels, Stories                            []string
		Budget, MaxTurns                           int // 0, left out, takes the server's default; -1 is no limit
	}
	if err := json.Unmarshal(arguments, &a); err != nil {
		return "", err
	}
	if a.Repo == "" {
		a.Repo = s.repo
	}
	if a.Epic != "" {
		a.Repo = "" // a story works where its epic does
	}
	var created struct {
		Stream   string
		NotFiled []string
	}
	agentry := s.agentry
	agentry.Timeout = worktrees.CloneTimeout + time.Minute // a repo named by its address is cloned before the task is made
	err := agentry.Call("POST", "/api/tasks", map[string]any{"title": a.Title, "description": a.Description, "kind": a.Kind,
		"team": a.Team, "labels": a.Labels, "epic": a.Epic, "stories": a.Stories, "repo": a.Repo,
		"budget": a.Budget, "maxTurns": a.MaxTurns}, &created)
	if err != nil {
		return "", err
	}
	text := fmt.Sprintf("Created %s: %s\n%s", created.Stream, a.Title, s.agentry.PageURL("/tasks/"+created.Stream))
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

func (s *Server) tell(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task, Text string }
	json.Unmarshal(arguments, &a)
	if err := s.agentry.Call("POST", "/api/tasks/"+a.Task+"/message", map[string]string{"text": a.Text}, nil); err != nil {
		return "", err
	}
	return "Sent to " + a.Task + "'s coordinator. It reads it now; the task's chat shows it.", nil
}

func (s *Server) answer(ctx context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task, Answer string }
	json.Unmarshal(arguments, &a)
	// Closing is the user's to approve, asked or not: an epic left open has
	// no question waiting, yet this answer would still close it.
	if closes(a.Answer) {
		return s.closeEpic(ctx, json.RawMessage(fmt.Sprintf(`{"epic": %q}`, a.Task)))
	}
	if q, ok := s.questionOf(a.Task); ok && copies(q, a.Answer) {
		return s.approveCopy(ctx, q)
	}
	if err := s.agentry.Call("POST", "/api/tasks/"+a.Task+"/answer", map[string]string{"answer": a.Answer}, nil); err != nil {
		return "", err
	}
	return "Answered " + a.Task + ". Its coordinator carries on.", nil
}

func (s *Server) fileUnder(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task, Epic string }
	json.Unmarshal(arguments, &a)
	if err := s.agentry.Call("PUT", "/api/tasks/"+a.Task+"/epic", map[string]string{"epic": a.Epic}, nil); err != nil {
		return "", err
	}
	if a.Epic == "" {
		return a.Task + " is out of its epic.", nil
	}
	return a.Task + " is a story of " + a.Epic + ".", nil
}

// setPriority gives the tasks their own ranks back, in the order wanted, so
// they move only among themselves.
func (s *Server) setPriority(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Tasks []string }
	json.Unmarshal(arguments, &a)
	all, _, err := s.board()
	if err != nil {
		return "", err
	}
	var ranks []float64
	for _, id := range a.Tasks {
		i := slices.IndexFunc(all, func(t task) bool { return t.ID == id })
		if i < 0 {
			return "", fmt.Errorf("no task %s", id)
		}
		ranks = append(ranks, all[i].Rank)
	}
	sort.Float64s(ranks)
	for i, id := range a.Tasks {
		if err := s.agentry.Call("PUT", "/api/tasks/"+id+"/rank", map[string]float64{"rank": ranks[i]}, nil); err != nil {
			return "", fmt.Errorf("%s: %w", id, err)
		}
	}
	return "In this order now: " + strings.Join(a.Tasks, ", ") + ".", nil
}

func (s *Server) start(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Tasks []string }
	json.Unmarshal(arguments, &a)
	for _, id := range a.Tasks {
		if err := s.agentry.Call("POST", "/api/tasks/"+id+"/direct", map[string]string{"to": "in-progress"}, nil); err != nil {
			return "", fmt.Errorf("%s: %w", id, err)
		}
	}
	return "Started " + strings.Join(a.Tasks, ", ") + ". They are on the Tasks board now.", nil
}

// ---- what only the user decides -----------------------------------------------

// closeEpic asks you, in Claude Code, whether to close the epic: one that
// asks to be closed, or one you left open with every story done.
func (s *Server) closeEpic(ctx context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Epic string }
	json.Unmarshal(arguments, &a)
	title, text, ok := s.closable(a.Epic)
	if !ok {
		return "", fmt.Errorf("%s isn't ready to close: not every story of it is done", a.Epic)
	}
	if !s.canElicit {
		return "Closing an epic is the user's to approve. Give them this link: " + s.agentry.PageURL("/tasks/"+a.Epic), nil
	}
	answer, ok, err := s.elicit(ctx, "Close the epic "+a.Epic+", "+title+"?\n\n"+text, object(props{
		"decision": enum("Close it, or keep it open to tell its coordinator what else it needs", "close", "keep open"),
	}, "decision"))
	if err != nil {
		return "", err
	}
	if !ok || answer["decision"] != "close" {
		return "The user kept " + a.Epic + " open. Ask them what else it needs, and send it with answer_question.", nil
	}
	if err := s.agentry.Call("POST", "/api/tasks/"+a.Epic+"/answer", map[string]string{"answer": closeEpic}, nil); err != nil {
		return "", err
	}
	return "The user closed " + a.Epic + ".", nil
}

// closable gives the epic's title and what to show you before closing it,
// if it can close: it asks to, or every story is done and you left it open.
func (s *Server) closable(id string) (title, text string, ok bool) {
	if q, asking := s.questionOf(id); asking && slices.Contains(q.Options, closeEpic) {
		return q.TaskTitle, q.Text, true
	}
	var epic task
	if err := s.agentry.Call("GET", "/api/tasks/"+id, nil, &epic); err != nil || epic.Closing == "" {
		return "", "", false
	}
	return epic.Title, "Every story of it is done. Its summary:\n\n" + epic.Closing, true
}

// reviewApprovals asks you, in Claude Code, about each waiting action.
func (s *Server) reviewApprovals(ctx context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task string }
	json.Unmarshal(arguments, &a)
	_, waiting, err := s.board()
	if err != nil {
		return "", err
	}
	var approvals []question
	for _, q := range waiting {
		if q.Kind == "approval" && (a.Task == "" || q.Task == a.Task) {
			approvals = append(approvals, q)
		}
	}
	if len(approvals) == 0 {
		return "No action is waiting for approval.", nil
	}
	if !s.canElicit {
		return fmt.Sprintf("%d actions wait for the user's approval, which is theirs to give. Give them this link: %s", len(approvals),
			s.agentry.PageURL("/tasks?status=needs-you")), nil
	}
	var done []string
	for _, q := range approvals {
		answer, ok, err := s.elicit(ctx, fmt.Sprintf("%s, on %s (%s), asks to run:\n\n%s: %s\n\nWhy it asks: %s", q.Agent, q.Task, q.TaskTitle,
			q.Tool, q.Text, q.Reason), object(props{
			"decision": enum("Let it run, or refuse it", "approve", "deny"),
			"reason":   text("Optional: why, for the agent"),
		}, "decision"))
		if err != nil {
			return "", err
		}
		if !ok {
			done = append(done, q.Task+" "+q.Action+": left for later")
			break
		}
		verb, past := "approve", "approved"
		if answer["decision"] == "deny" {
			verb, past = "deny", "denied"
		}
		reason, _ := answer["reason"].(string)
		if err := s.agentry.Call("POST", "/api/tasks/"+q.Task+"/actions/"+q.Action+"/"+verb, map[string]string{"reason": reason}, nil); err != nil {
			done = append(done, q.Task+" "+q.Action+": "+err.Error())
			continue
		}
		done = append(done, q.Task+" "+q.Action+": "+past+" by the user")
	}
	return strings.Join(done, "\n"), nil
}

// closeEpic is the answer that approves closing an epic (coordinator.CloseEpic).
const closeEpic = "Close the epic"

// copyFile is the answer that lets a file be copied from outside a
// machine's worktrees (coordinator.CopyFile).
const copyFile = "Copy it"

func copies(q question, answer string) bool {
	return slices.Contains(q.Options, copyFile) && strings.EqualFold(strings.TrimSpace(answer), copyFile)
}

// approveCopy asks you, in Claude Code, whether to let the file be copied.
func (s *Server) approveCopy(ctx context.Context, q question) (string, error) {
	if !s.canElicit {
		return "Copying a file from outside a machine's worktrees is the user's to allow. Give them this link: " + s.agentry.PageURL("/tasks/"+q.Task), nil
	}
	answer, ok, err := s.elicit(ctx, q.Text, object(props{"decision": enum("Copy it into the task's shared files, or not", "copy", "don't copy")}, "decision"))
	if err != nil {
		return "", err
	}
	if !ok || answer["decision"] != "copy" {
		return "The user didn't allow it. Send their reason, if they give one, with answer_question.", nil
	}
	if err := s.agentry.Call("POST", "/api/tasks/"+q.Task+"/answer", map[string]string{"answer": copyFile}, nil); err != nil {
		return "", err
	}
	return "The user allowed it: the file is in " + q.Task + "'s shared files.", nil
}

func closes(answer string) bool {
	return strings.EqualFold(strings.TrimSpace(answer), closeEpic)
}

// ---- the board -----------------------------------------------------------------

// board is every task, and everything waiting for you.
func (s *Server) board() ([]task, []question, error) {
	var all []task
	if err := s.agentry.Call("GET", "/api/tasks", nil, &all); err != nil {
		return nil, nil, err
	}
	var waiting []question
	if err := s.agentry.Call("GET", "/api/questions", nil, &waiting); err != nil {
		return nil, nil, err
	}
	return all, waiting, nil
}

func (s *Server) questionOf(id string) (question, bool) {
	_, waiting, err := s.board()
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
func (s *Server) inScope(all []task, everywhere bool) []task {
	if everywhere || s.repo == "" {
		return all
	}
	var mine []task
	for _, t := range all {
		if t.Repo == s.repo {
			mine = append(mine, t)
		}
	}
	return mine
}

func (s *Server) scope(everywhere bool) string {
	if everywhere || s.repo == "" {
		return "every repo"
	}
	return s.repo
}

// waitingNote is what waits for you anywhere, for the end of every reply.
func (s *Server) waitingNote() string {
	_, waiting, err := s.board()
	if err != nil {
		return ""
	}
	notes := lines(waiting, func(q question) (string, bool) { return describe(q), true })
	if ideas, ok := openIdeas(s.agentry, s.repo); ok {
		notes = append(notes, ideas.Line)
	}
	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\nWaiting for the user (%d):", len(notes))
	for _, note := range notes[:min(5, len(notes))] {
		b.WriteString("\n- " + note)
	}
	return b.String()
}

func describe(q question) string {
	switch q.Kind {
	case "approval":
		return fmt.Sprintf("%s: %s asks to run %s (review_approvals)", q.Task, q.Agent, oneLine(q.Text, 120))
	case "waiting":
		return fmt.Sprintf("%s: %s is stuck on a prompt in its terminal: %s", q.Task, q.Agent, oneLine(q.Text, 120))
	}
	if slices.Contains(q.Options, closeEpic) {
		return fmt.Sprintf("%s: the epic %s is ready to close (close_epic)", q.Task, q.TaskTitle)
	}
	if slices.Contains(q.Options, copyFile) {
		return fmt.Sprintf("%s: %s (answer_question with %q asks the user)", q.Task, oneLine(q.Text, 200), copyFile)
	}
	asker := q.Agent
	if asker == "coordinator-loop" {
		asker = "Agentry" // its own stops: a budget, a limit of turns
	}
	return fmt.Sprintf("%s: %s asks: %s", q.Task, asker, oneLine(q.Text, 160))
}

// ---- small helpers -------------------------------------------------------------

type props map[string]any

func object(properties props, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func text(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}
func texts(description string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
}
func integer(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}
func boolean(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}
func enum(description string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": description}
}

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

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func oneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) > limit {
		text = string([]rune(text)[:limit-1]) + "…"
	}
	return text
}
