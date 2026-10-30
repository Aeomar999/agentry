package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}

func write(t *testing.T, path, text string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A workspace after two rounds of work: both committed, then some work in
// progress: a file changed, one staged new, one new and not added, one
// deleted.
func TestFilesShowTheirGitStatus(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	git(t, dir, "init", "-q", "-b", "dev")
	write(t, filepath.Join(dir, "main.go"), "package main // TODO\n")
	write(t, filepath.Join(dir, "old.txt"), "going\n")
	write(t, filepath.Join(dir, "docs", "readme.md"), "# Hi\n")
	write(t, filepath.Join(dir, ".gitignore"), "bin/\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	base := git(t, dir, "rev-parse", "HEAD")[:40]

	write(t, filepath.Join(dir, "main.go"), "package main\n")
	write(t, filepath.Join(dir, "first.go"), "package main // round one\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "round one")
	git(t, dir, "rm", "-q", "old.txt")
	git(t, dir, "commit", "-qm", "round two")

	write(t, filepath.Join(dir, "docs", "readme.md"), "# Hello\n")
	write(t, filepath.Join(dir, "staged.go"), "package main\n")
	git(t, dir, "add", "staged.go")
	write(t, filepath.Join(dir, "new.go"), "package main\n")
	os.Remove(filepath.Join(dir, "first.go"))
	write(t, filepath.Join(dir, "bin", "app"), "\x00binary")

	w := Workspace{Dir: dir, Base: base}
	files, err := w.Files()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Change{}
	for _, f := range files {
		got[f.Path] = f.Change
	}
	// Committed work is plain, as in an editor: main.go was changed, but in a commit.
	want := map[string]Change{".gitignore": Unchanged, "main.go": Unchanged, "docs/readme.md": Modified,
		"staged.go": Added, "new.go": Untracked, "first.go": Deleted}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("files:\n got %v\nwant %v", got, want)
	}

	commits, err := w.Commits()
	if err != nil || len(commits) != 2 {
		t.Fatalf("commits: %+v %v", commits, err)
	}
	if c := commits[0]; c.Subject != "round two" || fmt.Sprint(c.Files) != "[{old.txt D}]" {
		t.Errorf("the newest commit: %+v", c)
	}
	if c := commits[1]; c.Subject != "round one" || fmt.Sprint(c.Files) != "[{first.go A} {main.go M}]" || c.Author != "Test" {
		t.Errorf("the first commit: %+v", c)
	}

	// With no base, as in a repo made for the task, every commit is its work.
	if all, err := (Workspace{Dir: dir}).Commits(); err != nil || len(all) != 3 || all[2].Subject != "base" {
		t.Errorf("all of the repo's commits: %d %v", len(all), err)
	}

	// Now, next to the last commit; in a commit, next to the one before it.
	if c, _ := w.Read("docs/readme.md"); c.Before != "# Hi\n" || c.Now != "# Hello\n" {
		t.Errorf("readme now: %+v", c)
	}
	inRoundOne := Workspace{Dir: dir, Base: base, Commit: commits[1].Hash}
	if c, _ := inRoundOne.Read("main.go"); c.Before != "package main // TODO\n" || c.Now != "package main\n" {
		t.Errorf("main.go in round one: %+v", c)
	}
	if _, err := (Workspace{Dir: dir, Base: base, Commit: "--output=/tmp/x"}).Read("main.go"); err == nil {
		t.Error("a commit that isn't a hash should be refused")
	}
	if c, _ := w.Read("old.txt"); c.Before != "" || c.Now != "" {
		t.Errorf("a file deleted in a commit is gone now: %+v", c)
	}
	if c, _ := w.Read("bin/app"); !c.Binary || c.Data != "AGJpbmFyeQ==" {
		t.Errorf("a binary file, with its bytes for a preview: %+v", c)
	}

	// Paths are pinned inside the worktree: a file beside it is never read.
	write(t, filepath.Join(filepath.Dir(dir), "secret.txt"), "secret\n")
	for _, outside := range []string{"../secret.txt", "docs/../../secret.txt", "/../secret.txt"} {
		if c, _ := w.Read(outside); c.Now == "secret\n" {
			t.Errorf("%s read a file outside the workspace", outside)
		}
	}
	if _, err := w.Read(".git/config"); err != ErrOutside {
		t.Errorf(".git/config: %v", err)
	}
	os.Symlink("/etc", filepath.Join(dir, "link"))
	if _, err := w.Read("link/passwd"); err != ErrOutside {
		t.Errorf("a link out of the workspace: %v", err)
	}
}
