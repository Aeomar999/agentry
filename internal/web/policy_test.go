package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// A server with a policy, for web's own routes that read or change it: a
// team's profiles, and the position a read was made at. The policy's own
// routes are tested in policyapi.

// policyServer, servePolicy, withoutReadMostly, hash and policyEvents are
// the same as their copies in
// internal/domain/policy/policyapi/policyapi_test.go, and all but hash as
// theirs in internal/domain/agents/agentsapi/agentsapi_test.go: change them
// together.

// policyServer serves the default team and a policy file holding text.
type policyServer struct {
	*httptest.Server
	log  *store.Store
	live *policy.Live
	path string
}

// servePolicy serves the policy text; configure, if given, changes the
// server's Config first.
func servePolicy(t *testing.T, text string, configure ...func(*Config)) policyServer {
	t.Helper()
	data := t.TempDir()
	db := filepath.Join(data, "agentry.db")
	s, state := storetest.Open(t, db)
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	path := filepath.Join(t.TempDir(), "policy.toml")
	os.WriteFile(path, []byte(text), 0o600)
	live, err := policy.OpenLive(path, "/home/dev", []string{db, path})
	if err != nil {
		t.Fatal(err)
	}
	config := Config{UIToken: token, DeskToken: "desk-secret", HelperToken: "helper-secret",
		Policy: live, Local: workers.Machine{Version: "v0.9.0"}, Worktrees: filepath.Join(data, "worktrees")}
	for _, c := range configure {
		c(&config)
	}
	server := httptest.NewServer(NewServer(s, state, config, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return policyServer{Server: server, log: s, live: live, path: path}
}

// withoutReadMostly is the default policy as a version before read-mostly had it.
func withoutReadMostly(t *testing.T) string {
	t.Helper()
	shipped := string(policy.Default())
	start, end := strings.Index(shipped, "[profiles.read-mostly]"), strings.Index(shipped, "[profiles.autonomous]")
	if start < 0 || end < start {
		t.Fatal("the default policy should have read-mostly before autonomous")
	}
	return shipped[:start] + shipped[end:]
}

func (p policyServer) hash() string { return p.live.Current().Hash() }

func (p policyServer) policyEvents(t *testing.T) []events.Event {
	t.Helper()
	stream, err := p.log.Stream(events.PolicyStream)
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

// A version's read carries the log position like every other read, so the
// page's shared cache reads it again only when the log moves past it.
func TestAVersionReadSaysThePositionItWasReadAt(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	server.log.Propose(policy.Loaded(server.path, &policy.Change{Policy: server.live.Current()}, "first loaded", policy.Agentry))
	call(t, server.Server, "POST", "/api/policy/use-for-everyone/off", `{"hash":"`+server.hash()+`"}`)
	path := "/api/policy/history/" + itoa(server.policyEvents(t)[0].Position)
	if code, body := call(t, server.Server, "GET", path, ""); code != 200 {
		t.Fatalf("GET %s: %d %s", path, code, body)
	}
	if got, at := positionOf(t, server.Server, "GET", path), itoa(server.log.LastPosition()); got != at {
		t.Errorf("GET %s: position %q, want %s", path, got, at)
	}
}
