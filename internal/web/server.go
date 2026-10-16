// Package web is the HTTP side of `agentry serve`: the UI, the API it uses,
// and the endpoint the hooks post to.
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
	DeskToken   string // the desk's: creates, reads and answers tasks, and nothing else
	DefaultRepo string // the repo a new task gets when it names none
	Budget      int    // the token budget a new task gets when it names none; 0 is no limit
	Policy      PolicySource
	Desk        http.Handler       // the desk's terminal websocket; nil when there is no desk
	SetTheme    func(theme string) // the page tells the server its theme, for the sessions it starts
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

	mux.HandleFunc("GET /", s.page) // the UI: its files, and index.html for its pages
	api := func(pattern string, handler http.HandlerFunc) { mux.Handle(pattern, s.requireUI(handler)) }
	// What the desk may do too: hand work to the coordinator and read it back.
	deskToo := func(pattern string, handler http.HandlerFunc) { mux.Handle(pattern, s.requireUIOrDesk(handler)) }
	deskToo("GET /api/tasks", s.listTasks)
	deskToo("POST /api/tasks", s.createTask)
	deskToo("GET /api/tasks/{task}", s.showTask)
	deskToo("POST /api/tasks/{task}/answer", s.answer)
	api("PUT /api/tasks/{task}/labels", s.label)
	api("PUT /api/tasks/{task}/budget", s.setBudget)
	api("POST /api/tasks/{task}/direct", s.direct)
	api("POST /api/tasks/{task}/actions/{action}/approve", s.approve)
	api("POST /api/tasks/{task}/actions/{action}/deny", s.deny)
	api("GET /api/agents", s.listAgents)
	api("POST /api/agents", s.createAgent)
	api("PUT /api/agents/{name}", s.changeAgent)
	api("DELETE /api/agents/{name}", s.removeAgent)
	api("GET /api/config", s.showConfig)
	api("GET /api/policy", s.showPolicy)
	api("GET /api/changes", s.waitForChanges)
	api("POST /api/theme", s.setTheme)
	if s.terminal != nil {
		mux.Handle("GET /api/tasks/{task}/terminal", s.requireUI(s.terminal.ServeHTTP))
	}
	if s.config.Desk != nil {
		mux.Handle("GET /api/desk/terminal", s.requireUI(s.config.Desk.ServeHTTP))
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

// requireUIOrDesk also lets in the desk's token. Approving an action never
// takes it: the desk runs as a model, and a model doesn't approve agents.
func (s *Server) requireUIOrDesk(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		desk := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		isDesk := s.config.DeskToken != "" && subtle.ConstantTimeCompare([]byte(desk), []byte(s.config.DeskToken)) == 1
		if !isDesk && !s.carriesUIToken(r) {
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
