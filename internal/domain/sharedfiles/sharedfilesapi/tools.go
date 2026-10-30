package sharedfilesapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
	"gitlab.com/dev.chempa/agentry/internal/platform/schema"
)

// Client is what the tools need of agentry mcp, which fills it: Agentry is
// the server the files go to and come from; Shown is a task's id as the
// answers name it, epic-N for an epic.
type Client struct {
	Agentry client.Client
	Shown   func(id string) string
}

// Tools share files between this machine and a task, from Claude Code.
func (c Client) Tools() []extensions.Tool {
	return []extensions.Tool{
		{Name: "attach_file", Description: "Share a file on this machine with a task: its agents find it in .agentry/files, on whichever machine works on it. For data, documents or builds that aren't in git.",
			Input: schema.Object(schema.Props{"task": schema.Text("its id"), "path": schema.Text("the file's path here"), "as": schema.Text("optional: the name to share it by, such as data/sales.csv")}, "task", "path"), Run: c.attachFile},
		{Name: "get_file", Description: "Download one of a task's shared files to this machine.",
			Input: schema.Object(schema.Props{"task": schema.Text("its id"), "name": schema.Text("the shared file's name"), "to": schema.Text("optional: where to save it; this folder if left out")}, "task", "name"), Run: c.getFile},
	}
}

func (c Client) attachFile(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task, Path, As string }
	json.Unmarshal(arguments, &a)
	a.Task = tasks.StoredID(a.Task)
	file, err := os.Open(a.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || info.IsDir() {
		return "", fmt.Errorf("%s isn't a file: share a folder's files one by one", a.Path)
	}
	name := a.As
	if name == "" {
		name = filepath.Base(a.Path)
	}
	response, err := c.Agentry.Raw(http.MethodPost, "/api/tasks/"+a.Task+"/shared?name="+url.QueryEscape(name), file)
	if err != nil {
		return "", err
	}
	response.Body.Close()
	return fmt.Sprintf("Shared %s with %s as %s. Its agents find it in .agentry/files.", a.Path, c.Shown(a.Task), name), nil
}

func (c Client) getFile(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task, Name, To string }
	json.Unmarshal(arguments, &a)
	a.Task = tasks.StoredID(a.Task)
	response, err := c.Agentry.Raw(http.MethodGet, "/api/tasks/"+a.Task+"/shared/file?name="+url.QueryEscape(a.Name), nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	to := a.To
	if to == "" {
		to = filepath.Base(a.Name)
	}
	if info, err := os.Stat(to); err == nil && info.IsDir() {
		to = filepath.Join(to, filepath.Base(a.Name))
	}
	out, err := os.Create(to)
	if err != nil {
		return "", err
	}
	size, err := io.Copy(out, response.Body)
	out.Close()
	if err != nil {
		return "", err
	}
	absolute, _ := filepath.Abs(to)
	return fmt.Sprintf("Saved %s from %s to %s (%d bytes).", a.Name, c.Shown(a.Task), absolute, size), nil
}
