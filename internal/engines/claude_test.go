package engines

import (
	"runtime"
	"testing"
)

// On Windows Claude names a folder C:/Users/…; it is trusted in both forms.
func TestAWindowsFolderIsTrustedBothWays(t *testing.T) {
	keys := folderKeys(`C:\Users\me\worktrees\task-1`)
	if runtime.GOOS == "windows" && (len(keys) != 2 || keys[1] != "C:/Users/me/worktrees/task-1") {
		t.Errorf("keys: %v", keys)
	}
	if runtime.GOOS != "windows" && len(keys) != 1 {
		t.Errorf("a Unix path has one form: %v", keys) // backslashes are just characters here
	}
}

// Every tool's call says what it does, so two different searches never look
// like the same command.
func TestCommandOfSaysWhatACallDoes(t *testing.T) {
	cases := map[string]string{
		`{"command": "git status"}`:                             "git status",
		`{"query": "sourdough lactobacilli"}`:                   "search: sourdough lactobacilli",
		`{"url": "https://example.com/starter", "prompt": "x"}`: "https://example.com/starter",
		`{"notebook_path": "/w/a.ipynb"}`:                       "/w/a.ipynb",
		`{"todos": []}`:                                         "",
	}
	for input, want := range cases {
		if got := commandOf([]byte(input)); got != want {
			t.Errorf("commandOf(%s) = %q, want %q", input, got, want)
		}
	}
}
