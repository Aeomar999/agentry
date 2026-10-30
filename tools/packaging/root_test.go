package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRoot makes a repo laid out as Agentry's, with ui/ committed and a page
// built from HEAD, and runs the test in it. It returns HEAD.
func newRoot(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't on the PATH")
	}
	repo := t.TempDir()
	for _, dir := range []string{"ui/src", page} {
		if err := os.MkdirAll(filepath.Join(repo, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(repo, "ui/src/main.tsx"), "render()\n")
	write(t, filepath.Join(repo, "README.md"), "Agentry\n")
	write(t, filepath.Join(repo, ".gitignore"), page+"/\n")
	runIn(t, repo, "git", "init", "-q")
	runIn(t, repo, "git", "add", ".")
	runIn(t, repo, "git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "a")
	head := strings.TrimSpace(runIn(t, repo, "git", "rev-parse", "HEAD"))
	write(t, filepath.Join(repo, page, "index.html"), `<div id="root"></div>`)
	write(t, filepath.Join(repo, page, "built-from"), head+"\n")
	t.Chdir(repo)
	return head
}

func TestCheckRootTakesAPageBuiltFromACleanHEAD(t *testing.T) {
	newRoot(t)
	if err := checkRoot(); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRootRefusesARootWithNoPage(t *testing.T) {
	newRoot(t)
	os.Remove(filepath.Join(page, "index.html"))
	if err := checkRoot(); err == nil || !strings.Contains(err.Error(), "with the page built (make ui)") {
		t.Fatalf("checkRoot: %v, want the page refused", err)
	}
}

func TestCheckRootRefusesAPageBuiltFromAnotherCommit(t *testing.T) {
	built := newRoot(t)
	runIn(t, ".", "git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "b")
	head := strings.TrimSpace(runIn(t, ".", "git", "rev-parse", "HEAD"))
	want := "the page was built from " + built + ", not from HEAD (" + head + "): run `make ui`, or copy in a page built from this commit"
	if err := checkRoot(); err == nil || err.Error() != want {
		t.Fatalf("checkRoot: %v\nwant: %s", err, want)
	}
}

func TestCheckRootRefusesAPageBuiltWithUIChangesThatWerentCommitted(t *testing.T) {
	head := newRoot(t)
	write(t, filepath.Join(page, "built-from"), head+"-dirty\n")
	want := "the page was built from " + head + " with ui/ changes that weren't committed, so it isn't HEAD's (" + head + "): commit them, then run `make ui`, or copy in a page built from this commit"
	if err := checkRoot(); err == nil || err.Error() != want {
		t.Fatalf("checkRoot: %v\nwant: %s", err, want)
	}
}

func TestCheckRootRefusesAPageThatDoesntSayItsCommit(t *testing.T) {
	head := newRoot(t)
	os.Remove(filepath.Join(page, "built-from"))
	err := checkRoot()
	if err == nil || !strings.Contains(err.Error(), "no "+page+"/built-from") || !strings.Contains(err.Error(), "HEAD ("+head+")") {
		t.Fatalf("checkRoot: %v, want the page refused", err)
	}
}

func TestCheckRootRefusesUIChangesThatArentCommitted(t *testing.T) {
	newRoot(t)
	write(t, "ui/src/main.tsx", "render(changed)\n")
	err := checkRoot()
	if err == nil || !strings.Contains(err.Error(), "ui/ has changes that aren't committed") || !strings.Contains(err.Error(), "ui/src/main.tsx") {
		t.Fatalf("checkRoot with an edit: %v, want it refused", err)
	}
	runIn(t, ".", "git", "checkout", "--", "ui")
	write(t, "ui/src/new.tsx", "new()\n")
	if err := checkRoot(); err == nil || !strings.Contains(err.Error(), "ui/src/new.tsx") {
		t.Fatalf("checkRoot with a new file: %v, want it refused", err)
	}
}

func TestCheckRootLeavesChangesOutsideUIAlone(t *testing.T) {
	newRoot(t)
	write(t, "README.md", "Agentry, changed\n")
	if err := checkRoot(); err != nil {
		t.Fatalf("checkRoot with README.md changed: %v", err)
	}
}

// A tree that isn't a git checkout, such as an unpacked source archive, has
// no HEAD to compare the page with, so only the page itself is checked.
func TestCheckRootOutsideGitChecksOnlyThePage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", root)
	if err := os.MkdirAll(filepath.Join(root, page), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, page, "index.html"), `<div id="root"></div>`)
	t.Chdir(root)
	if err := checkRoot(); err != nil {
		t.Fatalf("checkRoot outside git: %v", err)
	}
}
