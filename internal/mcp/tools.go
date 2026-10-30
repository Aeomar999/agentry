package mcp

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles/sharedfilesapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/tasksapi"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// tools are the domains' tools, then those of every extension shown.
func (s *Server) tools() []extensions.Tool {
	tools := s.taskTools().Tools()
	tools = append(tools, sharedfilesapi.Client{Raw: s.raw, Shown: func(id string) string { return s.taskTools().ShownNow()(id) }}.Tools()...)
	for _, f := range s.shownExtensions() {
		if f.Tools != nil {
			tools = append(tools, f.Tools(s.agentry, s.repo)...)
		}
	}
	return tools
}

// taskTools are the task, epic and question tools, for this session.
func (s *Server) taskTools() tasksapi.Client {
	return tasksapi.Client{Agentry: s.agentry, Repo: s.repo}
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
