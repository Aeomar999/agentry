package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/desk"
)

// `agentry task` is how the desk (internal/desk) hands work to the
// coordinator: create a task, list them, show one, answer a question. It
// talks to the server named in AGENTRY_SERVER with the token in
// AGENTRY_DESK_TOKEN, which the desk's session has. Approving an agent's
// action isn't here: that takes the UI token, which only the page has.

const taskUsage = `Usage:
  agentry task create --title "..." [--team Writing] [--kind story|epic] [--repo /abs/path [--new-repo]] [--label bug --label ...] [--description "..."]
                      [--story task-N ...]   (an epic, with tasks there already as its stories)
                      [--epic task-N]        (a story of that epic)
                      (or the description on stdin)
  agentry task epic task-E --add task-N [--add ...] [--remove task-M ...]
                      (file tasks under the epic task-E as its stories, or take them out)
  agentry task list
  agentry task show task-N
  agentry task answer task-N "answer"   (or the answer on stdin)`

func runTask(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, taskUsage)
		return 2
	}
	client := taskClient{server: strings.TrimSuffix(os.Getenv(desk.EnvServer), "/"), token: os.Getenv(desk.EnvToken)}
	if client.server == "" || client.token == "" {
		fmt.Fprintf(stderr, "agentry task: %s and %s must be set (the desk's session has both)\n", desk.EnvServer, desk.EnvToken)
		return 2
	}
	var err error
	switch args[0] {
	case "create":
		err = client.create(args[1:], stdin, stdout)
	case "epic":
		err = client.epic(args[1:], stdout)
	case "list":
		err = client.list(stdout)
	case "show":
		err = client.show(args[1:], stdout)
	case "answer":
		err = client.answer(args[1:], stdin, stdout)
	default:
		fmt.Fprintln(stderr, taskUsage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "agentry task:", err)
		return 1
	}
	return 0
}

type taskClient struct{ server, token string }

func (c taskClient) create(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("task create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	title := flags.String("title", "", "")
	repo := flags.String("repo", "", "")
	newRepo := flags.Bool("new-repo", false, "")
	team := flags.String("team", "", "")
	kind := flags.String("kind", "", "")
	epic := flags.String("epic", "", "")
	description := flags.String("description", "", "")
	var labels, stories []string
	flags.Func("label", "", func(label string) error { labels = append(labels, label); return nil })
	flags.Func("story", "", func(story string) error { stories = append(stories, story); return nil })
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%v\n%s", err, taskUsage)
	}
	if strings.TrimSpace(*title) == "" {
		return fmt.Errorf("a task needs --title")
	}
	if *description == "" {
		*description = readPiped(stdin)
	}
	var created struct {
		Stream   string
		NotFiled []string
	}
	if *newRepo && *repo == "" {
		return fmt.Errorf("--new-repo makes the repo named by --repo: give it a path")
	}
	request := map[string]any{"title": *title, "description": *description, "repo": *repo, "newRepo": *newRepo, "team": *team, "kind": *kind, "labels": labels,
		"stories": stories, "epic": *epic}
	if err := c.call("POST", "/api/tasks", request, &created); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Created %s: %s\nThe coordinator reads it next. On the page: %s/tasks/%s\n", created.Stream, *title, c.server, created.Stream)
	for _, refused := range created.NotFiled {
		fmt.Fprintf(stdout, "Not filed: %s\n", refused)
	}
	return nil
}

// epic files tasks under an epic, or takes them out of it.
func (c taskClient) epic(args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("which epic?\n%s", taskUsage)
	}
	epic := args[0]
	flags := flag.NewFlagSet("task epic", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var add, remove []string
	flags.Func("add", "", func(id string) error { add = append(add, id); return nil })
	flags.Func("remove", "", func(id string) error { remove = append(remove, id); return nil })
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("%v\n%s", err, taskUsage)
	}
	if len(add)+len(remove) == 0 {
		return fmt.Errorf("give --add or --remove\n%s", taskUsage)
	}
	for _, id := range add {
		if err := c.call("PUT", "/api/tasks/"+id+"/epic", map[string]string{"epic": epic}, nil); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		fmt.Fprintf(stdout, "%s is a story of %s now\n", id, epic)
	}
	for _, id := range remove {
		if err := c.call("PUT", "/api/tasks/"+id+"/epic", map[string]string{"epic": ""}, nil); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		fmt.Fprintf(stdout, "%s is out of %s\n", id, epic)
	}
	return nil
}

type taskLine struct {
	ID, Title, Status, CurrentAgent, NeedsYou, Repo, Kind, Epic string
	Labels                                                      []string
	Stories, StoriesDone                                        int
}

func (c taskClient) list(stdout io.Writer) error {
	var tasks []taskLine
	if err := c.call("GET", "/api/tasks", nil, &tasks); err != nil {
		return err
	}
	if len(tasks) == 0 {
		fmt.Fprintln(stdout, "No tasks yet.")
	}
	for _, t := range tasks {
		fmt.Fprintf(stdout, "%-8s %-10s %s", t.ID, t.Status, t.Title)
		switch {
		case t.Kind == "epic":
			fmt.Fprintf(stdout, "  (epic, %d of %d stories done)", t.StoriesDone, t.Stories)
		case t.Epic != "":
			fmt.Fprintf(stdout, "  (story of %s)", t.Epic)
		}
		if len(t.Labels) > 0 {
			fmt.Fprintf(stdout, "  [%s]", strings.Join(t.Labels, ", "))
		}
		fmt.Fprintln(stdout)
		if t.NeedsYou != "" {
			fmt.Fprintf(stdout, "         waiting for you: %s\n", oneLine(t.NeedsYou, 160))
		}
	}
	return nil
}

func (c taskClient) show(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("show which task?\n%s", taskUsage)
	}
	var t struct {
		taskLine
		Description, Summary string
		Trail                []struct {
			At             time.Time
			From, To, Text string
		}
	}
	if err := c.call("GET", "/api/tasks/"+args[0], nil, &t); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s: %s\nStatus: %s   Repo: %s\n", t.ID, t.Title, t.Status, t.Repo)
	if t.Description != "" {
		fmt.Fprintf(stdout, "\n%s\n", t.Description)
	}
	if t.NeedsYou != "" {
		fmt.Fprintf(stdout, "\nWaiting for you: %s\n", t.NeedsYou)
	}
	if t.Summary != "" {
		fmt.Fprintf(stdout, "\nDone: %s\n", t.Summary)
	}
	fmt.Fprintln(stdout, "\nHandoffs, oldest first:")
	for _, step := range t.Trail {
		fmt.Fprintf(stdout, "  %s  %s -> %s: %s\n", step.At.Local().Format("15:04"), step.From, step.To, oneLine(step.Text, 200))
	}
	return nil
}

func (c taskClient) answer(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("answer which task?\n%s", taskUsage)
	}
	text := strings.Join(args[1:], " ")
	if text == "" {
		text = readPiped(stdin)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("an answer needs some text")
	}
	if err := c.call("POST", "/api/tasks/"+args[0]+"/answer", map[string]string{"answer": text}, nil); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Sent your answer to %s's coordinator.\n", args[0])
	return nil
}

func (c taskClient) call(method, path string, body, into any) error {
	var payload io.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, c.server+path, payload)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("can't reach Agentry at %s: %w", c.server, err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", strings.TrimSpace(string(content)))
	}
	if into == nil {
		return nil
	}
	return json.Unmarshal(content, into)
}

// readPiped reads stdin when something is piped in, and nothing when it is
// a terminal, so a command without a heredoc doesn't wait for typing.
func readPiped(stdin io.Reader) string {
	if file, ok := stdin.(*os.File); ok {
		if info, err := file.Stat(); err != nil || info.Mode()&os.ModeCharDevice != 0 {
			return ""
		}
	}
	content, _ := io.ReadAll(stdin)
	return strings.TrimSpace(string(content))
}

func oneLine(text string, most int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > most {
		return text[:most] + "..."
	}
	return text
}
