package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/hookcall"
)

func TestRun(t *testing.T) {
	// Run inside an Agentry session, the hook case would call the live server.
	t.Setenv(hookcall.EnvTask, "")
	t.Setenv(hookcall.EnvServer, "")
	cases := []struct {
		args     []string
		wantCode int
		wantOut  string // in stdout
		wantErr  string // in stderr
	}{
		{nil, 0, "Usage: agentry", ""},
		{[]string{"help"}, 0, "serve", ""},
		{[]string{"serve", "-bogus"}, 2, "", "flag provided but not defined"},
		{[]string{"worker", "-dir", "/nonexistent/agentry-worker"}, 1, "", "hasn't joined a server yet"},
		{[]string{"worker", "join", "https://agentry.example.com"}, 2, "", "usage: agentry worker join"},
		{[]string{"worker", "help"}, 0, "leave [-purge]", ""},
		{[]string{"worker", "status", "-dir", "/nonexistent/agentry-worker"}, 1, "", "hasn't joined a server yet"},
		{[]string{"worker", "explode"}, 2, "", `unknown command "explode"`},
		{[]string{"hook", "PreToolUse"}, 2, "", "not running in an Agentry session"},
		{[]string{"deploy"}, 2, "", `unknown command "deploy"`},
	}
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		code := run(c.args, strings.NewReader("{}"), &stdout, &stderr)
		if code != c.wantCode || !strings.Contains(stdout.String(), c.wantOut) || !strings.Contains(stderr.String(), c.wantErr) {
			t.Errorf("run(%q) = %d, stdout %q, stderr %q", c.args, code, stdout.String(), stderr.String())
		}
	}
}

// Installing Agentry's Claude Code hooks keeps yours, and installing again
// replaces Agentry's rather than adding them twice.
func TestAddHooksKeepsYoursAndReplacesItsOwn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"model": "opus", "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "say done"}]}]}}`), 0o644)
	for range 2 {
		if err := addHooks(path, "'/bin/agentry' claude-hook"); err != nil {
			t.Fatal(err)
		}
	}
	var settings struct {
		Model string
		Hooks map[string][]struct {
			Hooks []struct {
				Command     string
				AsyncRewake bool
			}
		}
	}
	data, _ := os.ReadFile(path)
	json.Unmarshal(data, &settings)
	stop := settings.Hooks["Stop"]
	if settings.Model != "opus" || len(stop) != 2 || stop[0].Hooks[0].Command != "say done" ||
		stop[1].Hooks[0].Command != "'/bin/agentry' claude-hook rewake" || !stop[1].Hooks[0].AsyncRewake {
		t.Errorf("settings: %s", data)
	}
	if len(settings.Hooks["UserPromptSubmit"]) != 1 || len(settings.Hooks["SessionStart"]) != 1 {
		t.Errorf("Agentry's hooks should be there once: %s", data)
	}
}
