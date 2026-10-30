package ideas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
	"gitlab.com/dev.chempa/agentry/internal/platform/schema"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Ideas' MCP tools, run in agentry mcp over the HTTP API (routes.go). Claude
// only relays ideas: you accept an idea, which makes it a task in your
// Backlog, or dismiss it.

// instructions are ideas' line in agentry mcp's instructions to Claude.
const instructions = `- Ideas: the Visionaries suggest features as ideas (list_ideas). Relay them to the user; the user accepts or dismisses each (accept_idea, dismiss_idea). Never accept or dismiss an idea on your own. When accept_idea says an idea looks like an open task, show the user that task; only they choose to make another (anyway).`

// session is the Agentry server and the repo a Claude Code session is in
// ("" outside one), which the tools act on.
type session struct {
	agentry extensions.API
	repo    string
}

// tools are ideas' MCP tools for a session in repo.
func tools(agentry extensions.API, repo string) []extensions.Tool {
	s := session{agentry: agentry, repo: repo}
	return []extensions.Tool{
		{Name: "list_ideas", Description: "The features the Visionaries suggest for this repo (or all repos), newest first: open ones unless state says otherwise. Relay them to the user; the user decides on each.",
			Input: schema.Object(schema.Props{"all": schema.Boolean("every repo, not only this one"), "state": schema.Enum("which ideas: open (the default), accepted, dismissed or all", "open", "accepted", "dismissed", "all")}), Run: s.listIdeas},
		{Name: "accept_idea", Description: "Make an idea a task in the user's Backlog, or with start, a task that starts now. An idea that looks like an open task makes none until the user sees that task and chooses to make another (anyway). Only with the user's own decision, never on your own judgement: Claude Code asks the user to confirm when it can, and otherwise acts on these arguments.",
			Input: schema.Object(schema.Props{"id": schema.Text("the idea's id, such as idea-3"), "kind": schema.Enum("task (one change) or epic; leave it out for what the idea suggests", "task", "epic"),
				"note":   schema.Text("optional: what the user said, for the task's coordinator"),
				"start":  schema.Boolean("start the task now instead of leaving it in the Backlog; it spends tokens straight away"),
				"anyway": schema.Boolean("make the task even though the idea looks like an open task; only once the user has seen that task and chosen to")}, "id"), Run: s.acceptIdea},
		{Name: "dismiss_idea", Description: "Dismiss an idea. Only with the user's own decision, never on your own judgement: Claude Code asks the user to confirm when it can, and otherwise acts on these arguments.",
			Input: schema.Object(schema.Props{"id": schema.Text("the idea's id"), "note": schema.Text("optional: the user's reason, so the Visionaries don't suggest it again")}, "id"), Run: s.dismissIdea},
	}
}

// listed is an idea as GET /api/ideas answers it: the parts the tools read.
type listed struct {
	ID, Title, Problem, Proposal, Size, Task, Repo, Kind, State, Note, Became, BecameKind string
	About, AboutTitle                                                                     string // the epic whose close started its session
	BackedBy, DoubtedBy                                                                   []string
	AutoAccepted                                                                          bool // Agentry accepted it for you
}

func (s session) listIdeas(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct {
		All   bool
		State string
	}
	json.Unmarshal(arguments, &a)
	switch a.State {
	case "":
		a.State = "open"
	case "open", "accepted", "dismissed", "all":
	default:
		return "", fmt.Errorf("no state %q: use open, accepted, dismissed or all", a.State)
	}
	repo := s.repo
	if a.All {
		repo = ""
	}
	list, err := fetchIdeas(s.agentry, repo)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, i := range list {
		if a.State != "all" && i.State != a.State {
			continue
		}
		fmt.Fprintf(&b, "%s [%s] %s (%s)\n", i.ID, i.State, i.Title, headerOf(i))
		fmt.Fprintf(&b, "    problem: %s\n    feature: %s\n", tasks.OneLine(i.Problem, 300), tasks.OneLine(i.Proposal, 300))
		if len(i.BackedBy)+len(i.DoubtedBy) > 0 {
			fmt.Fprintf(&b, "    backed by: %s · doubted by: %s\n", namesOf(i.BackedBy), namesOf(i.DoubtedBy))
		}
		switch i.State {
		case "accepted":
			fmt.Fprintf(&b, "    became %s", tasks.ShownID(i.Became, tasks.Kind(i.BecameKind)))
		case "dismissed":
			b.WriteString("    dismissed")
		default:
			continue
		}
		if i.Note != "" {
			b.WriteString(": " + tasks.OneLine(i.Note, 200))
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		which := a.State + " ideas"
		if a.State == "all" {
			which = "ideas"
		}
		return "No " + which + " for " + s.scope(a.All) + ".", nil
	}
	return b.String(), nil
}

// acceptIdea makes an idea a task in your Backlog, or started with start.
// With elicitation Claude Code asks you first, and you can change the kind,
// the note and whether it starts. An idea that looks like an open task
// makes none unless anyway: with elicitation Claude Code shows you that
// task and asks whether to make another; without, the reply names it.
func (s session) acceptIdea(ctx context.Context, arguments json.RawMessage) (string, error) {
	var a struct {
		ID, Kind, Note string
		Start, Anyway  bool
	}
	json.Unmarshal(arguments, &a)
	i, err := s.find(a.ID)
	if err != nil {
		return "", err
	}
	if a.Kind == "" {
		a.Kind = suggestedKind(i)
	}
	if ask := extensions.AskOf(ctx); ask != nil {
		answer, ok, err := ask(ctx, "Accept the idea "+i.ID+", "+i.Title+"? It becomes a task in your Backlog, or one that starts now.\n\n"+aboutIdea(i), schema.Object(schema.Props{
			"decision": schema.Enum("Make it a task, or leave the idea open", "accept", "leave open"),
			"kind":     withDefault(schema.Enum("A task (one change) or an epic (split into stories)", "task", "epic"), a.Kind),
			"note":     withDefault(schema.Text("Optional: what to tell its coordinator"), a.Note),
			"start":    withDefault(schema.Enum("Leave it in the Backlog, or start it now (it spends tokens straight away)", "backlog", "start"), startOf(a.Start)),
		}, "decision"))
		if err != nil {
			return "", err
		}
		if !ok || answer["decision"] != "accept" {
			return "The user left " + i.ID + " open. Nothing was written.", nil
		}
		a.Kind, _ = answer["kind"].(string)
		a.Note, _ = answer["note"].(string)
		a.Start = answer["start"] == "start"
	}
	var got struct {
		Task, Column, Kind string
		Accepted           bool
		Refused            string
	}
	body := map[string]any{"kind": a.Kind, "note": a.Note}
	if a.Start {
		body["start"] = true // left out otherwise, so an accept that doesn't start sends what it always did
	}
	if a.Anyway {
		body["anyway"] = true // as start: left out unless chosen
	}
	err = s.agentry.Call("POST", "/api/ideas/"+i.ID+"/accept", body, &got)
	if like, found := duplicateOf(err); found {
		again, why, askErr := s.makeAnyway(ctx, i, like)
		if askErr != nil || !again {
			return why, askErr
		}
		body["anyway"] = true
		err = s.agentry.Call("POST", "/api/ideas/"+i.ID+"/accept", body, &got)
	}
	if err != nil {
		return "", fmt.Errorf("%s wasn't accepted: %w", i.ID, err)
	}
	text := fmt.Sprintf("%s became %s%s: %s", i.ID, tasks.ShownID(got.Task, tasks.Kind(got.Kind)), placeOf(got.Column), s.agentry.PageURL("/tasks/"+got.Task))
	if !got.Accepted {
		text += "\nThe idea isn't marked accepted: " + got.Refused
	}
	return text, nil
}

// openTask is the open task an idea looks like, as accepting it answers
// (routes.go duplicateView).
type openTask struct{ ID, Title, Column, Kind string }

// duplicateOf is the open task an accept was refused for looking like: a
// 409 whose body is JSON naming it.
func duplicateOf(err error) (openTask, bool) {
	var answer *client.StatusError
	if !errors.As(err, &answer) || answer.Code != http.StatusConflict {
		return openTask{}, false
	}
	var refusal struct{ Duplicate openTask }
	if json.Unmarshal(answer.Body, &refusal) != nil || refusal.Duplicate.ID == "" {
		return openTask{}, false
	}
	return refusal.Duplicate, true
}

// makeAnyway asks you, with elicitation, whether to make the idea a task
// though it looks like the open task like; why is the reply when it
// doesn't, and without elicitation it never does: the reply names the task
// for you to decide.
func (s session) makeAnyway(ctx context.Context, i listed, like openTask) (again bool, why string, err error) {
	task := fmt.Sprintf("%s %q%s: %s", tasks.ShownID(like.ID, tasks.Kind(like.Kind)), like.Title, placeOf(like.Column), s.agentry.PageURL("/tasks/"+like.ID))
	ask := extensions.AskOf(ctx)
	if ask == nil {
		return false, i.ID + " looks like the open task " + task + "\nNothing was written. If the user wants another task anyway, call accept_idea again with anyway.", nil
	}
	answer, ok, err := ask(ctx, "The idea "+i.ID+", "+i.Title+", looks like the open task "+task+"\n\nMake another task anyway?", schema.Object(schema.Props{
		"decision": schema.Enum("Make another task anyway, or leave the idea open", "make another anyway", "leave open"),
	}, "decision"))
	if err != nil {
		return false, "", err
	}
	if !ok || answer["decision"] != "make another anyway" {
		return false, "The user left " + i.ID + " open, as it looks like " + task + "\nNothing was written.", nil
	}
	return true, "", nil
}

// dismissIdea dismisses an idea. With elicitation Claude Code asks you
// first, and you can change the note.
func (s session) dismissIdea(ctx context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ ID, Note string }
	json.Unmarshal(arguments, &a)
	i, err := s.find(a.ID)
	if err != nil {
		return "", err
	}
	if ask := extensions.AskOf(ctx); ask != nil {
		answer, ok, err := ask(ctx, "Dismiss the idea "+i.ID+", "+i.Title+"?\n\n"+aboutIdea(i), schema.Object(schema.Props{
			"decision": schema.Enum("Dismiss it, or leave it open", "dismiss", "leave open"),
			"note":     withDefault(schema.Text("Optional: why, so the Visionaries don't suggest it again"), a.Note),
		}, "decision"))
		if err != nil {
			return "", err
		}
		if !ok || answer["decision"] != "dismiss" {
			return "The user left " + i.ID + " open. Nothing was written.", nil
		}
		a.Note, _ = answer["note"].(string)
	}
	if err := s.agentry.Call("POST", "/api/ideas/"+i.ID+"/dismiss", map[string]string{"note": a.Note}, nil); err != nil {
		return "", fmt.Errorf("%s wasn't dismissed: %w", i.ID, err)
	}
	return "The user dismissed " + i.ID + ", " + i.Title + ".", nil
}

// placeOf says where an accepted idea's task is, from the column Agentry
// answers, not from start: a retry reuses a task made before, started or
// not. "" when the column can't be read.
func placeOf(column string) string {
	switch column {
	case "":
		return ""
	case "backlog":
		return ", in the user's Backlog until they start it"
	case "in-progress", "review", "shipping":
		return ", started (" + tasks.ColumnName(column) + ")"
	case "done":
		return ", " + tasks.ColumnName(column)
	}
	return ", in " + tasks.ColumnName(column)
}

// startOf is the elicitation's answer for start.
func startOf(start bool) string {
	if start {
		return "start"
	}
	return "backlog"
}

// find is one idea, of any repo.
func (s session) find(id string) (listed, error) {
	list, err := fetchIdeas(s.agentry, "")
	if err != nil {
		return listed{}, err
	}
	for _, i := range list {
		if i.ID == id {
			return i, nil
		}
	}
	return listed{}, fmt.Errorf("no idea %s", id)
}

// fetchIdeas are the ideas about a repo, or every repo's for "", newest first.
func fetchIdeas(agentry extensions.API, repo string) ([]listed, error) {
	path := "/api/ideas"
	if repo != "" {
		path += "?repo=" + url.QueryEscape(repo)
	}
	var list []listed
	err := agentry.Call("GET", path, nil, &list)
	return list, err
}

// aboutIdea is what you read before deciding on an idea.
func aboutIdea(i listed) string {
	header := headerOf(i)
	// header[:1] is a whole letter: headerOf always has the kind, and the parts are ASCII.
	return fmt.Sprintf("Problem: %s\n\nFeature: %s\n\n%s%s.", i.Problem, i.Proposal, strings.ToUpper(header[:1]), header[1:])
}

// headerOf is the idea's size, kind and source task, leaving out what it
// doesn't have.
func headerOf(i listed) string {
	var parts []string
	if i.Size != "" {
		parts = append(parts, "size "+i.Size)
	}
	parts = append(parts, suggestedKind(i))
	if i.Task != "" {
		parts = append(parts, "suggested in "+i.Task)
	}
	if i.About != "" {
		parts = append(parts, "after "+tasks.ShownEpic(i.About)+" "+strconv.Quote(i.AboutTitle)+" closed")
	}
	return strings.Join(parts, ", ")
}

// suggestedKind is what the idea suggests making of it: a task or an epic.
func suggestedKind(i listed) string {
	if i.Kind == "epic" {
		return "epic"
	}
	return "task"
}

func namesOf(agents []string) string {
	if len(agents) == 0 {
		return "none"
	}
	return strings.Join(agents, ", ")
}

func withDefault(schema map[string]any, value string) map[string]any {
	if value != "" {
		schema["default"] = value
	}
	return schema
}

// scope is what the session's lists are of: its repo, or every repo.
func (s session) scope(everywhere bool) string {
	if everywhere || s.repo == "" {
		return "every repo"
	}
	return s.repo
}
