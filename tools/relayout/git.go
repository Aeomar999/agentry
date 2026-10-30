package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// git runs git in the repo at root and returns its output.
func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	// A GIT_DIR or GIT_WORK_TREE from a calling git (a hook, rebase --exec)
	// would point every command at that repo, not root.
	cmd.Env = withoutGitRepoVars(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func withoutGitRepoVars(env []string) []string {
	var kept []string
	for _, v := range env {
		if !strings.HasPrefix(v, "GIT_DIR=") && !strings.HasPrefix(v, "GIT_WORK_TREE=") && !strings.HasPrefix(v, "GIT_INDEX_FILE=") {
			kept = append(kept, v)
		}
	}
	return kept
}

// trackedFiles lists the files git tracks under path, or in the whole repo
// when path is ".", relative to root.
func trackedFiles(root, path string) ([]string, error) {
	out, err := git(root, "ls-files", "-z", "--", path)
	if err != nil {
		return nil, err
	}
	return strings.FieldsFunc(out, func(r rune) bool { return r == 0 }), nil
}
