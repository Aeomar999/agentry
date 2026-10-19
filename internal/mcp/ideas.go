package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/client"
)

// Ideas are features the Visionaries suggest. Claude only relays them: you
// accept an idea, which makes it a task in your Backlog, or dismiss it.

// The parts of the API's ideas the tools read.
type idea struct {
	ID, Title, Problem, Proposal, Size, Task, Repo, Kind, State, Note, Became string
	About, AboutTitle                                                         string // the epic whose close started its session
	BackedBy, DoubtedBy                                                       []string
	AutoAccepted                                                              bool // Agentry accepted it for you
}

func (s *Server) listIdeas(_ context.Context, arguments json.RawMessage) (string, error) {
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
	list, err := ideasOf(s.agentry, repo)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, i := range list {
		if a.State != "all" && i.State != a.State {
			continue
		}
		fmt.Fprintf(&b, "%s [%s] %s (%s)\n", i.ID, i.State, i.Title, headerOf(i))
		fmt.Fprintf(&b, "    problem: %s\n    feature: %s\n", oneLine(i.Problem, 300), oneLine(i.Proposal, 300))
		if len(i.BackedBy)+len(i.DoubtedBy) > 0 {
			fmt.Fprintf(&b, "    backed by: %s · doubted by: %s\n", namesOf(i.BackedBy), namesOf(i.DoubtedBy))
		}
		switch i.State {
		case "accepted":
			fmt.Fprintf(&b, "    became %s", i.Became)
		case "dismissed":
			b.WriteString("    dismissed")
		default:
			continue
		}
		if i.Note != "" {
			b.WriteString(": " + oneLine(i.Note, 200))
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
// the note and whether it starts.
func (s *Server) acceptIdea(ctx context.Context, arguments json.RawMessage) (string, error) {
	var a struct {
		ID, Kind, Note string
		Start          bool
	}
	json.Unmarshal(arguments, &a)
	i, err := s.idea(a.ID)
	if err != nil {
		return "", err
	}
	if a.Kind == "" {
		a.Kind = kindOf(i)
	}
	if s.canElicit {
		answer, ok, err := s.elicit(ctx, "Accept the idea "+i.ID+", "+i.Title+"? It becomes a task in your Backlog, or one that starts now.\n\n"+aboutIdea(i), object(props{
			"decision": enum("Make it a task, or leave the idea open", "accept", "leave open"),
			"kind":     withDefault(enum("A task (one change) or an epic (split into stories)", "task", "epic"), a.Kind),
			"note":     withDefault(text("Optional: what to tell its coordinator"), a.Note),
			"start":    withDefault(enum("Leave it in the Backlog, or start it now (it spends tokens straight away)", "backlog", "start"), startOf(a.Start)),
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
		Task, Column string
		Accepted     bool
		Refused      string
	}
	body := map[string]any{"kind": a.Kind, "note": a.Note}
	if a.Start {
		body["start"] = true // left out otherwise, so an accept that doesn't start sends what it always did
	}
	if err := s.agentry.Call("POST", "/api/ideas/"+i.ID+"/accept", body, &got); err != nil {
		return "", fmt.Errorf("%s wasn't accepted: %w", i.ID, err)
	}
	text := fmt.Sprintf("%s became %s%s: %s", i.ID, got.Task, placeOf(got.Column), s.agentry.PageURL("/tasks/"+got.Task))
	if !got.Accepted {
		text += "\nThe idea isn't marked accepted: " + got.Refused
	}
	return text, nil
}

// dismissIdea dismisses an idea. With elicitation Claude Code asks you
// first, and you can change the note.
func (s *Server) dismissIdea(ctx context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ ID, Note string }
	json.Unmarshal(arguments, &a)
	i, err := s.idea(a.ID)
	if err != nil {
		return "", err
	}
	if s.canElicit {
		answer, ok, err := s.elicit(ctx, "Dismiss the idea "+i.ID+", "+i.Title+"?\n\n"+aboutIdea(i), object(props{
			"decision": enum("Dismiss it, or leave it open", "dismiss", "leave open"),
			"note":     withDefault(text("Optional: why, so the Visionaries don't suggest it again"), a.Note),
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
	}
	return ", started (" + boardName(column) + ")"
}

// startOf is the elicitation's answer for start.
func startOf(start bool) string {
	if start {
		return "start"
	}
	return "backlog"
}

// idea is one idea, of any repo.
func (s *Server) idea(id string) (idea, error) {
	list, err := ideasOf(s.agentry, "")
	if err != nil {
		return idea{}, err
	}
	for _, i := range list {
		if i.ID == id {
			return i, nil
		}
	}
	return idea{}, fmt.Errorf("no idea %s", id)
}

// ideasOf are the ideas about a repo, or every repo's for "", newest first.
func ideasOf(agentry client.Client, repo string) ([]idea, error) {
	path := "/api/ideas"
	if repo != "" {
		path += "?repo=" + url.QueryEscape(repo)
	}
	var list []idea
	err := agentry.Call("GET", path, nil, &list)
	return list, err
}

// openIdeas is one entry for every open idea, of every repo: how many, and
// the newest titles. It points at list_ideas with all when some are in
// another repo than repo, or when repo is "" (not known). Its key is the
// number of ideas ever suggested, so a new idea is news and one you accept or
// dismiss isn't; one Agentry accepted for you was never yours to decide, so
// it isn't counted. ok is false with none open.
func openIdeas(agentry client.Client, repo string) (w Waiting, ok bool) {
	all, err := ideasOf(agentry, "")
	if err != nil {
		return Waiting{}, false
	}
	var open []idea
	yours := 0
	pointer := "list_ideas"
	if repo == "" {
		pointer = "list_ideas all"
	}
	for _, i := range all {
		if !i.AutoAccepted {
			yours++
		}
		if i.State != "open" {
			continue
		}
		open = append(open, i)
		if repo != "" && i.Repo != repo {
			pointer = "list_ideas all"
		}
	}
	if len(open) == 0 {
		return Waiting{}, false
	}
	var titles []string
	for _, i := range open[:min(3, len(open))] {
		titles = append(titles, oneLine(i.Title, 60))
	}
	if more := len(open) - len(titles); more > 0 {
		titles = append(titles, fmt.Sprintf("and %d more", more))
	}
	what := "open ideas"
	if len(open) == 1 {
		what = "open idea"
	}
	return Waiting{Key: "ideas/" + strconv.Itoa(yours), Task: open[0].Task, Kind: "ideas",
		Line: fmt.Sprintf("%d %s from the Visionaries: %s (%s)", len(open), what, strings.Join(titles, ", "), pointer)}, true
}

// aboutIdea is what you read before deciding on an idea.
func aboutIdea(i idea) string {
	header := headerOf(i)
	// header[:1] is a whole letter: headerOf always has the kind, and the parts are ASCII.
	return fmt.Sprintf("Problem: %s\n\nFeature: %s\n\n%s%s.", i.Problem, i.Proposal, strings.ToUpper(header[:1]), header[1:])
}

// headerOf is the idea's size, kind and source task, leaving out what it
// doesn't have.
func headerOf(i idea) string {
	var parts []string
	if i.Size != "" {
		parts = append(parts, "size "+i.Size)
	}
	parts = append(parts, kindOf(i))
	if i.Task != "" {
		parts = append(parts, "suggested in "+i.Task)
	}
	if i.About != "" {
		parts = append(parts, "after the epic "+i.About+" "+strconv.Quote(i.AboutTitle)+" closed")
	}
	return strings.Join(parts, ", ")
}

// kindOf is what the idea suggests making of it: a task or an epic.
func kindOf(i idea) string {
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
