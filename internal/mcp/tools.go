package mcp

import (
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles/sharedfilesapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/tasksapi"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// tools are the domains' tools, then those of every extension shown.
func (s *Server) tools() []extensions.Tool {
	tools := s.taskTools().Tools()
	tools = append(tools, sharedfilesapi.Client{Agentry: s.agentry, Shown: func(id string) string { return s.taskTools().ShownNow()(id) }}.Tools()...)
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
