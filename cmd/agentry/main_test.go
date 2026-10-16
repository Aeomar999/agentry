package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	cases := []struct {
		args     []string
		wantCode int
		wantOut  string // in stdout
		wantErr  string // in stderr
	}{
		{nil, 0, "Usage: agentry", ""},
		{[]string{"help"}, 0, "serve", ""},
		{[]string{"serve", "-bogus"}, 2, "", "flag provided but not defined"},
		{[]string{"worker"}, 2, "", "not built yet (T-022"},
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
