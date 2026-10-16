// Package web is the HTTP side of `agentry serve`: the one page, the API it
// uses, and the endpoint the hooks post to.
package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

type Config struct {
	UIToken     string // opens the page and the API; printed when the server starts
	DefaultRepo string // the repo a new task gets when it names none
	Policy      PolicySource
}

// PolicySource is the policy in force and any problem with the file on disk.
type PolicySource interface {
	Current() *policy.Policy
	Problem() string
	Path() string
}

type Server struct {
	store    *store.Store
	config   Config
	hooks    http.Handler
	terminal http.Handler // the live terminal websocket, T-018

	creating sync.Mutex // one task created at a time, so IDs never collide
}

func NewServer(s *store.Store, config Config, hooks, terminal http.Handler) *Server {
	return &Server{store: s, config: config, hooks: hooks, terminal: terminal}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST "+hookcall.Path, s.hooks) // has its own token

	mux.HandleFunc("GET /{$}", s.page)
	api := func(pattern string, handler http.HandlerFunc) { mux.Handle(pattern, s.requireUI(handler)) }
	api("GET /api/tasks", s.listTasks)
	api("POST /api/tasks", s.createTask)
	api("GET /api/tasks/{task}", s.showTask)
	api("POST /api/tasks/{task}/answer", s.answer)
	api("POST /api/tasks/{task}/actions/{action}/approve", s.approve)
	api("POST /api/tasks/{task}/actions/{action}/deny", s.deny)
	api("GET /api/agents", s.listAgents)
	api("GET /api/config", s.showConfig)
	api("GET /api/policy", s.showPolicy)
	api("GET /api/changes", s.waitForChanges)
	if s.terminal != nil {
		mux.Handle("GET /api/tasks/{task}/terminal", s.requireUI(s.terminal.ServeHTTP))
	}
	return mux
}

const uiCookie = "agentry_ui"

// requireUI lets in the page and scripts that carry the UI token: as a
// bearer token (the page sends it on every call), as ?token= (the terminal's
// websocket can't send headers), or as the cookie. The hook token, which
// every agent session has, doesn't open the API: an agent can't approve its
// own requests.
func (s *Server) requireUI(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.carriesUIToken(r) {
			http.Error(w, "open the link `agentry serve` printed", http.StatusUnauthorized)
			return
		}
		next(w, r)
	})
}

func (s *Server) carriesUIToken(r *http.Request) bool {
	given := []string{strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), r.URL.Query().Get("token")}
	if cookie, err := r.Cookie(uiCookie); err == nil {
		given = append(given, cookie.Value)
	}
	for _, token := range given {
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.UIToken)) == 1 {
			return true
		}
	}
	return false
}
