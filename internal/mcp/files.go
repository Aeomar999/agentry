package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// Sharing files between this machine and a task, from Claude Code.

func (s *Server) attachFile(_ context.Context, arguments json.RawMessage) (string, error) {
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
	response, err := s.raw(http.MethodPost, "/api/tasks/"+a.Task+"/shared?name="+url.QueryEscape(name), file)
	if err != nil {
		return "", err
	}
	response.Body.Close()
	return fmt.Sprintf("Shared %s with %s as %s. Its agents find it in .agentry/files.", a.Path, s.shownNow()(a.Task), name), nil
}

func (s *Server) getFile(_ context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task, Name, To string }
	json.Unmarshal(arguments, &a)
	a.Task = tasks.StoredID(a.Task)
	response, err := s.raw(http.MethodGet, "/api/tasks/"+a.Task+"/shared/file?name="+url.QueryEscape(a.Name), nil)
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
	return fmt.Sprintf("Saved %s from %s to %s (%d bytes).", a.Name, s.shownNow()(a.Task), absolute, size), nil
}

// raw calls the server with a body that isn't JSON, such as a file.
func (s *Server) raw(method, path string, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequest(method, strings.TrimSuffix(s.agentry.URL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+s.agentry.Token)
	response, err := s.agentry.HTTP(0).Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		said, _ := io.ReadAll(response.Body)
		response.Body.Close()
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(said)))
	}
	return response, nil
}
