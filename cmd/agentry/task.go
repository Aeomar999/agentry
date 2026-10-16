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
  agentry task create --title "..." [--repo /abs/path] [--label bug --label ...] [--description "..."]
                      (or the description on stdin)
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
	description := flags.String("description", "", "")
	var labels []string
	flags.Func("label", "", func(label string) error { labels = append(labels, label); return nil })
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%v\n%s", err, taskUsage)
	}
	if strings.TrimSpace(*title) == "" {
		return fmt.Errorf("a task needs --title")
	}
	if *description == "" {
		*description = readPiped(stdin)
	}
	var created struct{ Stream string }
	request := map[string]any{"title": *title, "description": *description, "repo": *repo, "labels": labels}
	if err := c.call("POST", "/api/tasks", request, &created); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Created %s: %s\nThe coordinator reads it next. On the page: %s/tasks/%s\n", created.Stream, *title, c.server, created.Stream)
	return nil
}

type taskLine struct {
	ID, Title, Status, CurrentAgent, NeedsYou, Repo string
	Labels                                          []string
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
