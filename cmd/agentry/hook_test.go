package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/hookcall"
)

func hookEnv(t *testing.T, server string) {
	t.Setenv(hookcall.EnvServer, server)
	t.Setenv(hookcall.EnvCA, "")
	t.Setenv(hookcall.EnvToken, "secret")
	t.Setenv(hookcall.EnvTask, "task-1")
	t.Setenv(hookcall.EnvTurn, "turn-2")
	t.Setenv(hookcall.EnvAgent, "engineer")
	t.Setenv(hookcall.EnvEngine, "claude")
	t.Setenv(hookcall.EnvDeadline, "300ms")
}

func runHookWith(payload string, event string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := runHook([]string{event}, strings.NewReader(payload), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestTheHookPassesTheServersReplyThrough(t *testing.T) {
	var got hookcall.Call
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != hookcall.Path || r.Header.Get(hookcall.TokenHeader) != "secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
		json.NewEncoder(w).Encode(hookcall.Reply{Stdout: `{"decision":"allow"}`})
	}))
	defer server.Close()
	hookEnv(t, server.URL)

	code, stdout, _ := runHookWith(`{"tool_name":"Bash"}`, "PreToolUse")
	if code != 0 || stdout != `{"decision":"allow"}` {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
	if got.Task != "task-1" || got.Turn != "turn-2" || got.Agent != "engineer" || got.Event != "PreToolUse" || string(got.Payload) != `{"tool_name":"Bash"}` {
		t.Errorf("the server got %+v", got)
	}
}

// Proof 01: every way the call can fail exits 2, so the gate fails closed.
func TestTheHookFailsClosed(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer slow.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer broken.Close()

	for name, server := range map[string]string{"unreachable": "http://127.0.0.1:1", "slow": slow.URL, "broken": broken.URL} {
		hookEnv(t, server)
		started := time.Now()
		code, _, stderr := runHookWith(`{}`, "PreToolUse")
		if code != 2 || !strings.Contains(stderr, "fail closed") {
			t.Errorf("%s: exit %d, stderr %q", name, code, stderr)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Errorf("%s: took %s; the hook's own deadline is 300ms", name, elapsed)
		}
	}
}
