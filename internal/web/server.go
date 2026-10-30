// Package web is the HTTP side of `agentry serve`: the UI, the API it uses,
// and the endpoint the hooks post to.
package web

import (
	"bytes"
	"crypto/subtle"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/connections"
	"gitlab.com/dev.chempa/agentry/internal/domain/notifications/notificationsapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/notifications/webpush"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy/hooks"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy/policyapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles/sharedfilesapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/tasksapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

type Config struct {
	UIToken     string // opens the page and the API; printed when the server starts
	DeskToken   string // the desk's: creates, reads and answers tasks, and nothing else
	HelperToken string // the Claude sessions' in tasks' Workspaces: sends the messages you ask them to, and nothing else
	DefaultRepo string // the repo a new task gets when it names none
	Clones      string // where repos named by a git address are cloned
	Budget      int    // the token budget a new task gets when it names none; 0 is no limit
	MaxTurns    int    // the limit of turns without your answer a new task gets when it names none; 0 is no limit
	Policy      PolicySource
	Desk        http.Handler       // the desk's terminal websocket; nil when there is no desk
	Helper      http.Handler       // the terminal of the Claude session beside a task, started on the way; nil for none
	SetTheme    func(theme string) // the page tells the server its theme, for the sessions it starts

	Fleet       *machines.Fleet                // where tasks' work is: for their Workspace page
	Workers     *workerlink.Server             // the server's end of the workers' links; nil for none
	PublicURL   string                         // the address people and workers reach; "" for the one a request came to
	CA          string                         // Agentry's own CA certificate, PEM, when the server is served with it (-https); "" for none
	NetworkURLs []string                       // the server on each of this machine's network addresses, when it listens on every one
	LocalURL    string                         // the server on this computer, http://localhost:port; "" when localhost doesn't reach it
	Binary      string                         // this agentry, which a joining machine can download
	Local       workers.Machine                // what this machine is, as a worker would say it, for the Workers page
	Blobs       sharedfiles.Disk               // where tasks' shared files are kept
	Extensions  []extensions.Extension         // what teams and settings can turn on, for GET /api/features; their routes are mounted too
	Logs        map[string]extensions.Proposer // each extension's proposer from extensions.Checked, by its Name: all its routes may write through
	Worktrees   string                         // where this machine keeps tasks' worktrees: a call tried on the Policy page runs in one there
	Push        *webpush.Store                 // the VAPID keys and the browsers subscribed to pushes; nil for none
	StopSession func(task string)              // ends a task's agent session, wherever it runs (turns.Runner.Stop); nil for none
	Snapshots   *current.SnapshotWriter        // writes the state's snapshot; nil for none
	Redactions  *hooks.Redactions              // the secrets redacted from the agents' knowledge, for the turns running then; nil for none

	Settings func(extension string) extensions.Settings // each extension's own table in the settings file, for its routes; nil for none
}

// defaults are the limits a new task gets when it names none.
func (c Config) defaults() tasks.Limits { return tasks.Limits{Budget: c.Budget, MaxTurns: c.MaxTurns} }

// PolicySource is the policy in force and any problem with the file on
// disk, and the one way to change it.
type PolicySource = policyapi.Source

type Server struct {
	store    *store.Store        // the log: what to propose, and events to show
	events   extensions.EventLog // the store, read-only: what extensions' routes read of the log
	state    *current.State      // every task and roster, as of the log's last event
	config   Config
	hooks    http.Handler
	terminal http.Handler // the live terminal websocket, T-018

	signins  *signins           // the one-time codes that sign in another device
	archives *tasksapi.Archives // archived tasks read back from their blobs, the last few kept
}

func NewServer(s *store.Store, state *current.State, config Config, hooks, terminal http.Handler) *Server {
	return &Server{store: s, events: extensions.ReadOnly(s), state: state, config: config, hooks: hooks, terminal: terminal, signins: newSignins(time.Now),
		archives: tasksapi.NewArchives(config.Blobs)}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.register(mux)
	return s.atPosition(mux)
}

// register puts every route on mux: web's own, then each domain's and
// extension's, mounted with its owner. web's own are only these; a new
// route belongs in a domain's api package or an extension
// (TestTheRouteTableIsUpToDate).
func (s *Server) register(mux routeTable) {
	mux.Handle("POST "+hookcall.Path, s.hooks) // has its own token

	mux.HandleFunc("GET /", s.page) // the UI: its files, and index.html for its pages
	api := func(pattern string, handler http.HandlerFunc) { mux.Handle(pattern, s.requireUI(handler)) }
	mux.Handle("POST /api/tasks/{task}/claude/message", s.requireClaude(s.claudeMessage))
	api("GET /api/features", s.listFeatures)
	api("GET /api/waiting", s.listWaiting)
	api("GET /api/config", s.showConfig)
	api("GET /api/changes", s.waitForChanges)
	api("POST /api/theme", s.setTheme)
	// Signing in: a device without the token swaps a one-time code, or the
	// token pasted in, for the cookie and the token.
	mux.Handle("POST /api/signin/codes", s.requireRealUIToken(s.issueSigninCode))
	mux.HandleFunc("POST /api/signin", s.signIn)
	mux.HandleFunc("POST /api/signout", s.signOut)
	if s.terminal != nil {
		mux.Handle("GET /api/tasks/{task}/terminal", s.requireUI(s.terminal.ServeHTTP))
	}
	if s.config.Helper != nil {
		mux.Handle("GET /api/tasks/{task}/claude/terminal", s.requireUI(s.config.Helper.ServeHTTP))
	}
	if s.config.Desk != nil {
		mux.Handle("GET /api/desk/terminal", s.requireUI(s.config.Desk.ServeHTTP))
	}
	s.mount("the tasks domain", s.tasksAPI().Routes(), mux)
	s.mount("the settings domain", s.settingsAPI().Routes(), mux)
	s.mount("the connections domain", s.connectionsAPI().Routes(), mux)
	s.mount("the notifications domain", notificationsapi.API{Push: s.config.Push, PublicURL: s.config.PublicURL}.Routes(), mux)
	s.mount("the shared files domain", sharedfilesapi.API{Log: s.store, State: s.state, Blobs: s.config.Blobs, HasWorkers: s.config.Workers != nil}.Routes(), mux)
	s.mount("the policy domain", s.policyAPI().Routes(), mux)
	s.mount("the workers domain", s.workersAPI().Routes(), mux)
	s.mount("the agents domain", s.agentsAPI().Routes(), mux)
	// Last, so an extension's route that clashes with any core or domain
	// route is the one that fails, naming its extension.
	for _, f := range s.config.Extensions {
		s.mount("the "+f.Name+" extension", s.routesOf(f), mux)
	}
}

// positionHeader is the log position an API read saw everything up to.
const positionHeader = "X-Agentry-Position"

// atPosition tells each API read the log position it was read at, so the
// page refetches a path only when the log has moved past it. The position
// is taken before the handler reads: an event that lands in between makes
// the reply newer than its header says, which costs the page one extra
// refetch and never a stale screen. It is the store's applied position,
// the one /api/changes reports: the state has every event up to it, and
// so do the log's reads, which the state's own position can be one ahead
// of while an event is being applied.
func (s *Server) atPosition(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set(positionHeader, strconv.FormatInt(s.store.LastPosition(), 10))
		}
		next.ServeHTTP(w, r)
	})
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

// carriesConnection is a request from a connected Claude Code: its
// credential opens what the page's token does, until it is revoked.
func (s *Server) carriesConnection(r *http.Request) bool {
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if credential == "" {
		return false
	}
	roster, err := s.connections()
	if err != nil {
		return false
	}
	_, ok := roster.WithCredential(credential)
	return ok
}

func (s *Server) connections() (connections.Roster, error) {
	return s.state.Connections(), s.state.Broken(events.ConnectionsStream)
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

// signed lets in a request whose body verify accepts, such as a git host's
// webhook signed with that hook's secret, and gives next the body again. No
// token opens it, the UI's included: a webhook's caller has none, and the
// page has no business writing as System.
func signed(verify func(r *http.Request, body []byte) error, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			r.Body = http.NoBody // a hand-built request may have none
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, httpapi.MaxBody))
		if err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := verify(r, body); err != nil {
			http.Error(w, "this route takes only a request signed with its secret", http.StatusUnauthorized)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next(w, r)
	})
}

// requireRealUIToken lets in only the UI token, not a connection's
// credential: for what hands the token out, so it gives nobody more.
func (s *Server) requireRealUIToken(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hasUIToken(r) {
			http.Error(w, "only a page signed in with the UI token can do this", http.StatusUnauthorized)
			return
		}
		next(w, r)
	})
}

func (s *Server) carriesUIToken(r *http.Request) bool {
	return s.hasUIToken(r) || s.carriesConnection(r)
}

// hasUIToken is the UI token itself, in any of the places the page puts it;
// a connection's credential is not it.
func (s *Server) hasUIToken(r *http.Request) bool {
	given := []string{strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), r.URL.Query().Get("token")}
	if cookie, err := r.Cookie(uiCookie); err == nil {
		given = append(given, cookie.Value)
	}
	return slices.ContainsFunc(given, s.isUIToken)
}

func (s *Server) isUIToken(token string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.UIToken)) == 1
}

// setUICookie keeps the token in the browser for the page and the API. It
// lasts a year, so an installed app that kept it stays signed in; it goes
// over https only when the page came that way, else plain http would lose it.
func setUICookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: uiCookie, Value: token, Path: "/", MaxAge: 365 * 24 * 60 * 60,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: overHTTPS(r)})
}

func clearUICookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: uiCookie, Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: overHTTPS(r)})
}
