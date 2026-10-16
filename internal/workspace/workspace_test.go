package workspace

import (
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

// A workspace after some work: one file changed and committed, one deleted,
// one new and not yet committed, one left alone.
func TestFilesShowWhatChangedSinceTheBase(t *testing.T) {
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
	git(t, dir, "rm", "-q", "old.txt")
	git(t, dir, "commit", "-qam", "work")
	write(t, filepath.Join(dir, "new.go"), "package main\n")
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
	want := map[string]Change{".gitignore": Unchanged, "docs/readme.md": Unchanged, "main.go": Modified, "new.go": Added, "old.txt": Deleted}
	if len(got) != len(want) {
		t.Errorf("files: %v", got)
	}
	for path, change := range want {
		if got[path] != change {
			t.Errorf("%s: %q, want %q", path, got[path], change)
		}
	}

	c, err := w.Read("main.go")
	if err != nil || c.Before != "package main // TODO\n" || c.Now != "package main\n" {
		t.Errorf("main.go: %+v %v", c, err)
	}
	if c, _ := w.Read("old.txt"); c.Before != "going\n" || c.Now != "" {
		t.Errorf("a deleted file: %+v", c)
	}
	if c, _ := w.Read("bin/app"); !c.Binary {
		t.Errorf("a binary file: %+v", c)
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
