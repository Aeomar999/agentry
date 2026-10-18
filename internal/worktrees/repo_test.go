package worktrees

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestANewRepoIsReadyForATask(t *testing.T) {
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	path := filepath.Join(t.TempDir(), "projects", "market-data")
	if reason := IsRepo(path); reason == "" {
		t.Error("a folder that isn't there is not a repo")
	}
	if err := NewRepo(path, "Design the market data system"); err != nil {
		t.Fatal(err)
	}
	if reason := IsRepo(path); reason != "" {
		t.Errorf("the new repo: %s", reason)
	}
	if subject, _ := git(path, "log", "-1", "--format=%s"); subject != "Start: Design the market data system" {
		t.Errorf("first commit: %q", subject)
	}
	w, err := Create(path, t.TempDir(), "task-9", 7909)
	if err != nil || w.Branch != "agentry/task-9" {
		t.Fatalf("a task's worktree from it: %+v %v", w, err)
	}

	// Never over work that is already there, and never a relative path.
	os.WriteFile(filepath.Join(path, "notes.md"), []byte("mine\n"), 0o644)
	if err := NewRepo(path, "again"); err == nil || !strings.Contains(err.Error(), "isn't empty") {
		t.Errorf("a folder with files in it: %v", err)
	}
	if err := NewRepo("projects/x", "x"); err == nil {
		t.Error("a relative path should be refused")
	}
}

func TestATaskThatMovesRepoGetsAWorktreeOfTheNewOne(t *testing.T) {
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	root := t.TempDir()
	first, second := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	for _, repo := range []string{first, second} {
		if err := NewRepo(repo, filepath.Base(repo)); err != nil {
			t.Fatal(err)
		}
	}
	old, err := Create(first, root, "task-7", 7907)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(old.Path, "draft.md"), []byte("kept\n"), 0o644)

	moved, err := Create(second, root, "task-7", 7907)
	if err != nil {
		t.Fatal(err)
	}
	if !sameRepo(moved.Path, second) || moved.Path != old.Path {
		t.Errorf("the task's worktree should now be of the second repo, at the same place: %+v", moved)
	}
	if content, err := os.ReadFile(old.Path + ".before-move/draft.md"); err != nil || string(content) != "kept\n" {
		t.Errorf("the old worktree's work should be kept aside: %q %v", content, err)
	}
	if list, _ := git(first, "worktree", "list"); !strings.Contains(list, ".before-move") {
		t.Errorf("the first repo should know where its worktree went:\n%s", list)
	}
}

func TestARepoNamedByItsAddressIsClonedOnceAndKeptUpToDate(t *testing.T) {
	for address, want := range map[string]bool{
		"git@gitlab.com:dev.chempa/agentry.git": true,
		"https://github.com/team/app":           true,
		"ssh://git@host:2222/team/app.git":      true,
		"/home/you/projects/app":                false,
		"C:/projects/app":                       false,
		"projects/app":                          false,
	} {
		if got := IsAddress(address); got != want {
			t.Errorf("IsAddress(%q) = %v", address, got)
		}
	}
	if name := cloneName("git@gitlab.com:dev.chempa/agentry.git"); !strings.HasPrefix(name, "agentry-") {
		t.Errorf("the clone's folder: %q", name)
	}

	remote := newRepo(t)
	address := "file://" + filepath.ToSlash(remote)
	clones := t.TempDir()
	first, err := Clone(clones, address)
	if err != nil {
		t.Fatal(err)
	}
	if reason := IsRepo(first); reason != "" {
		t.Fatalf("the clone: %s", reason)
	}

	write(t, filepath.Join(remote, "new.txt"), "later\n")
	mustGit(t, remote, "add", "new.txt")
	mustGit(t, remote, "commit", "-q", "-m", "later")
	again, err := Clone(clones, address)
	if err != nil || again != first {
		t.Fatalf("named again: %s %v, want %s", again, err, first)
	}
	if _, err := os.Stat(filepath.Join(again, "new.txt")); err != nil {
		t.Error("named again, the clone should have what the remote has now")
	}

	if _, err := Clone(clones, "file://"+filepath.ToSlash(filepath.Join(t.TempDir(), "nothing"))); err == nil {
		t.Error("an address with no repo should fail")
	}
}

func TestARepoNamedByALinkToAnArchiveIsDownloaded(t *testing.T) {
	var archive bytes.Buffer
	zipped := gzip.NewWriter(&archive)
	files := tar.NewWriter(zipped)
	for name, contents := range map[string]string{"app-1.2/main.go": "package main\n", "app-1.2/app-1.2/notes.md": "inner\n"} {
		files.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(contents)), Typeflag: tar.TypeReg})
		files.Write([]byte(contents))
	}
	files.Close()
	zipped.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/app-1.2.tar.gz" {
			http.NotFound(w, r)
			return
		}
		w.Write(archive.Bytes())
	}))
	defer server.Close()

	address := server.URL + "/releases/app-1.2.tar.gz?download=1"
	if !IsAddress(address) {
		t.Fatal("a link to an archive is an address")
	}
	folder, err := Clone(t.TempDir(), address)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(folder), "app-1.2-") {
		t.Errorf("its folder: %s", folder)
	}
	if reason := IsRepo(folder); reason != "" {
		t.Fatalf("the download should be a repo: %s", reason)
	}
	for _, name := range []string{"main.go", "app-1.2/notes.md"} {
		if _, err := os.Stat(filepath.Join(folder, name)); err != nil {
			t.Errorf("%s should be at the top of the repo, out of the archive's folder", name)
		}
	}
	if tracked, _ := git(folder, "ls-files"); !strings.Contains(tracked, "main.go") {
		t.Errorf("everything should be committed: %q", tracked)
	}
	if _, err := Clone(t.TempDir(), server.URL+"/missing.zip"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("a link that isn't there: %v", err)
	}
}

// serveArchive serves contents at /app.<kind> and gives its link.
func serveArchive(t *testing.T, kind string, contents []byte) string {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(contents) }))
	t.Cleanup(server.Close)
	return server.URL + "/app" + kind
}

func tarball(t *testing.T, headers ...tar.Header) []byte {
	var archive bytes.Buffer
	files := tar.NewWriter(&archive)
	for _, header := range headers {
		header.Mode = 0o644
		contents := strings.Repeat("x", int(header.Size))
		if err := files.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		files.Write([]byte(contents))
	}
	files.Close()
	return archive.Bytes()
}

// An archive can't write outside its folder, bring git's own files (whose
// config and hooks would run), make links, or unpack to more than the limit.
func TestADownloadedArchiveStaysInItsFolder(t *testing.T) {
	var zipped bytes.Buffer
	files := zip.NewWriter(&zipped)
	escape, _ := files.Create("../escaped.txt")
	escape.Write([]byte("out\n"))
	files.Close()
	clones := t.TempDir()
	if _, err := Clone(clones, serveArchive(t, ".zip", zipped.Bytes())); err == nil || !strings.Contains(err.Error(), "outside its folder") {
		t.Errorf("a zip entry out of the folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(clones, "escaped.txt")); err == nil {
		t.Error("the escaping file was written")
	}
	if entries, _ := os.ReadDir(clones); len(entries) != 0 {
		t.Errorf("a failed download should leave nothing behind: %v", entries)
	}

	folder, err := Clone(t.TempDir(), serveArchive(t, ".tar", tarball(t,
		tar.Header{Name: "app/main.go", Size: 4, Typeflag: tar.TypeReg},
		tar.Header{Name: "app/.git/config", Size: 4, Typeflag: tar.TypeReg},
		tar.Header{Name: "app/sub/.GIT", Size: 4, Typeflag: tar.TypeReg},
		tar.Header{Name: "app/passwd", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink})))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, "main.go")); err != nil {
		t.Error("the archive's own file should be there")
	}
	for _, name := range []string{"sub/.GIT", "passwd"} {
		if _, err := os.Lstat(filepath.Join(folder, name)); err == nil {
			t.Errorf("%s should be left out", name)
		}
	}
	if config, _ := os.ReadFile(filepath.Join(folder, ".git", "config")); strings.Contains(string(config), "xxxx") {
		t.Error("the archive's .git/config should be left out")
	}

	defer func(limit int64) { maxArchive = limit }(maxArchive)
	maxArchive = 200
	if _, err := Clone(t.TempDir(), serveArchive(t, ".tar", tarball(t, tar.Header{Name: "big.bin", Size: 512, Typeflag: tar.TypeReg}))); err == nil || !strings.Contains(err.Error(), "bigger than") {
		t.Errorf("a download over the limit: %v", err)
	}
	var bomb bytes.Buffer
	files = zip.NewWriter(&bomb)
	big, _ := files.Create("big.txt")
	big.Write(bytes.Repeat([]byte("x"), 5000))
	files.Close()
	if bomb.Len() > 200 {
		t.Fatalf("the test's zip should compress under the limit: %d bytes", bomb.Len())
	}
	if _, err := Clone(t.TempDir(), serveArchive(t, ".zip", bomb.Bytes())); err == nil || !strings.Contains(err.Error(), "unpacks to more than") {
		t.Errorf("an archive unpacking over the limit: %v", err)
	}
}

func TestOnlyWebLinksAreArchives(t *testing.T) {
	for address, want := range map[string]string{
		"https://example.com/app-1.2.tar.gz": ".tar.gz",
		"http://example.com/app.ZIP?x=1":     ".zip",
		"https://github.com/team/app":        "",
		"file:///srv/app.zip":                "",
		"ftp://example.com/app.tgz":          "",
		"git@example.com:team/app.tar.gz":    "",
	} {
		if got := archiveKind(address); got != want {
			t.Errorf("archiveKind(%q) = %q, want %q", address, got, want)
		}
	}
}
