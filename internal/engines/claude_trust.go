package engines

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Claude Code asks "Do you trust this folder?" the first time it opens one,
// and waits there until someone answers. A task's worktree is a new folder,
// and on a worker nothing above it is trusted either, so each turn would
// stop at that question. Claude keeps its answer per folder in its own
// config; trustFolder writes that answer before the session starts.

// claudeConfig is Claude Code's config file: ~/.claude.json, or in
// CLAUDE_CONFIG_DIR when that is set.
func claudeConfig() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude.json")
}

// trustFolder marks dir as trusted in Claude Code's config, keeping
// everything else in it as it was. With no config yet (Claude has never run
// here), it does nothing: Claude's first run sets it up.
func trustFolder(dir string) error {
	path := claudeConfig()
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var config map[string]any
	if err := json.Unmarshal(content, &config); err != nil {
		return err
	}
	projects, _ := config["projects"].(map[string]any)
	if projects == nil {
		projects = map[string]any{}
		config["projects"] = projects
	}
	changed := false
	for _, key := range folderKeys(dir) {
		project, _ := projects[key].(map[string]any)
		if project == nil {
			project = map[string]any{}
			projects[key] = project
		}
		if project["hasTrustDialogAccepted"] != true {
			project["hasTrustDialogAccepted"], changed = true, true
		}
	}
	if !changed {
		return nil
	}

	// Written whole and moved into place, so Claude never reads half a file.
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	info, _ := os.Stat(path)
	temporary := path + ".agentry"
	if err := os.WriteFile(temporary, updated, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// folderKeys are the ways Claude may name dir in its config. On Windows it
// writes C:/Users/…, with forward slashes; both forms are trusted.
func folderKeys(dir string) []string {
	if slashed := filepath.ToSlash(dir); slashed != dir {
		return []string{dir, slashed}
	}
	return []string{dir}
}

// TrustFolder trusts dir for a Claude session that isn't an agent's turn,
// such as the one you open beside a task, so it starts without asking.
func TrustFolder(dir string) error { return trustFolder(dir) }
