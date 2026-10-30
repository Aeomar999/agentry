package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"debug/macho"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReleaseFilesAreTheNineForAVersion(t *testing.T) {
	var names []string
	for _, file := range releaseFiles("v1.2.3") {
		names = append(names, file.name)
	}
	want := []string{
		"agentry-v1.2.3-mac-apple-silicon.zip",
		"agentry-v1.2.3-mac-intel.zip",
		"agentry-v1.2.3-linux-x64.zip",
		"agentry-v1.2.3-linux-arm64.zip",
		"agentry-v1.2.3-windows-x64.zip",
		"agentry-v1.2.3-windows-arm64.zip",
		"agentry-desktop-v1.2.3-linux-amd64.tar.gz",
		"agentry-desktop-v1.2.3-windows-amd64.zip",
		"agentry-desktop-v1.2.3-macos-universal.zip",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("releaseFiles = %q, want %q", names, want)
	}
}

func TestMissingAndStrayFilesAreReported(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"agentry-v1.2.3-linux-x64.zip",
		"agentry-v1.2.3-dirty-linux-x64.zip",
		"agentry-desktop-v1.2.3-4-g9e240bd7-windows-amd64.zip",
		"agentry-v1.2.2-dirty-linux-x64.zip", // an older release's, left alone
		"agentry-v1.2.3-rc1-linux-x64.zip",   // another tag's, left alone
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	problems := strings.Join(checkRelease(dir, "v1.2.3", "abc"), "\n")
	for _, file := range releaseFiles("v1.2.3") {
		missing := filepath.Join(dir, file.name) + ": missing"
		if strings.Contains(problems, missing) != (file.name != "agentry-v1.2.3-linux-x64.zip") {
			t.Errorf("%s: missing is said wrongly in\n%s", file.name, problems)
		}
	}
	for name, stray := range map[string]bool{
		"agentry-v1.2.3-dirty-linux-x64.zip":                   true,
		"agentry-desktop-v1.2.3-4-g9e240bd7-windows-amd64.zip": true,
		"agentry-v1.2.2-dirty-linux-x64.zip":                   false,
		"agentry-v1.2.3-rc1-linux-x64.zip":                     false,
	} {
		if strings.Contains(problems, name+": not a build of the tagged commit") != stray {
			t.Errorf("%s: stray should be %v in\n%s", name, stray, problems)
		}
	}
}

func TestOnlyATagNamesTheReleaseCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't on the PATH")
	}
	repo := t.TempDir()
	write(t, filepath.Join(repo, "a"), "a")
	runIn(t, repo, "git", "init", "-q")
	runIn(t, repo, "git", "add", ".")
	runIn(t, repo, "git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "a")
	commit := strings.TrimSpace(runIn(t, repo, "git", "rev-parse", "HEAD"))
	runIn(t, repo, "git", "tag", "v1.0.0")
	runIn(t, repo, "git", "-c", "user.name=t", "-c", "user.email=t@t", "tag", "-a", "-m", "v1.1.0", "v1.1.0")
	runIn(t, repo, "git", "branch", "topic")
	t.Chdir(repo)
	for _, tag := range []string{"v1.0.0", "v1.1.0"} {
		if got, err := tagCommit(tag); err != nil || got != commit {
			t.Errorf("tagCommit(%q) = %q, %v, want %q", tag, got, err, commit)
		}
	}
	for _, other := range []string{"topic", "HEAD", commit, "v9.9.9"} {
		if _, err := tagCommit(other); err == nil || !strings.Contains(err.Error(), "isn't a tag") {
			t.Errorf("tagCommit(%q): %v, want it refused", other, err)
		}
	}
}

func TestEachProgramMustBeBuiltFromTheTagWithNoChanges(t *testing.T) {
	b := newBuilds(t)
	if got := programProblems(b.clean["linux/amd64"], b.commit); len(got) != 0 {
		t.Errorf("a clean build of the commit: %q, want none", got)
	}
	if got := programProblems(b.clean["linux/amd64"], "0123abcd"); !hasProblem(got, "built from "+b.commit+", not the tag's 0123abcd") {
		t.Errorf("another commit: %q", got)
	}
	if got := programProblems(b.modified, b.commit); !hasProblem(got, "built from a tree with changes (changed or untracked files)") {
		t.Errorf("a tree with changes: %q", got)
	}
	if got := programProblems(b.noVCS, b.commit); !hasProblem(got, "no commit in its build info") {
		t.Errorf("no vcs info: %q", got)
	}
	if got := programProblems([]byte("#!/bin/sh\n"), b.commit); !hasProblem(got, "no Go build info") {
		t.Errorf("not a Go program: %q", got)
	}
}

// buildinfo reads only a universal program's first arch, so each is read.
func TestEveryArchOfAUniversalMacProgramIsChecked(t *testing.T) {
	b := newBuilds(t)
	clean := fatMachO(t, b.clean["darwin/arm64"], b.clean["darwin/amd64"])
	if got := programProblems(clean, b.commit); len(got) != 0 {
		t.Errorf("both arches clean: %q, want none", got)
	}
	mixed := fatMachO(t, b.clean["darwin/arm64"], b.modifiedMac)
	got := programProblems(mixed, b.commit)
	if len(got) != 1 || !strings.HasPrefix(got[0], "CpuAmd64: built from a tree with changes") {
		t.Errorf("the second arch built with changes: %q, want its problem alone", got)
	}
}

func TestAReadyReleaseGetsItsSumsAndTheCommandToPublishIt(t *testing.T) {
	b := newBuilds(t)
	dir := t.TempDir()
	program := b.clean["linux/amd64"]
	mac := fatMachO(t, b.clean["darwin/arm64"], b.clean["darwin/amd64"])
	for _, file := range releaseFiles("v1.2.3") {
		contents := map[string][]byte{"Agentry/README.txt": []byte("text")}
		for _, name := range file.programs {
			contents["Agentry/"+name] = program
			if strings.HasPrefix(name, "Agentry.app/") {
				contents["Agentry/"+name] = mac
			}
		}
		writeArchive(t, filepath.Join(dir, file.name), contents)
	}
	if got := checkRelease(dir, "v1.2.3", b.commit); len(got) != 0 {
		t.Fatalf("a ready release: %q, want none", got)
	}

	if err := writeSums(dir, releaseFiles("v1.2.3")); err != nil {
		t.Fatal(err)
	}
	sums, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(sums), "\n"), "\n")
	if len(lines) != 9 || !strings.HasSuffix(lines[0], "  agentry-desktop-v1.2.3-linux-amd64.tar.gz") || !strings.HasSuffix(lines[8], "  agentry-v1.2.3-windows-x64.zip") {
		t.Errorf("SHA256SUMS isn't the 9 files sorted by name:\n%s", sums)
	}
	if sha256sum, err := exec.LookPath("sha256sum"); err == nil {
		check := exec.Command(sha256sum, "-c", "--quiet", "SHA256SUMS")
		check.Dir = dir
		if out, err := check.CombinedOutput(); err != nil {
			t.Errorf("sha256sum -c: %v\n%s", err, out)
		}
	}

	command := publishCommand("v1.2.3", releaseFiles("v1.2.3"))
	if !strings.HasPrefix(command, "gh release create v1.2.3 release/agentry-v1.2.3-mac-apple-silicon.zip ") ||
		!strings.HasSuffix(command, ` release/agentry-desktop-v1.2.3-macos-universal.zip release/SHA256SUMS --repo agent3-labs/agentry --verify-tag --title v1.2.3 --notes "What changed"`) ||
		strings.Count(command, " release/") != 10 {
		t.Errorf("publishCommand = %s", command)
	}

	writeArchive(t, filepath.Join(dir, "agentry-v1.2.3-linux-x64.zip"), map[string][]byte{"Agentry/agentry": program})
	if got := checkRelease(dir, "v1.2.3", b.commit); !hasProblem(got, filepath.Join(dir, "agentry-v1.2.3-linux-x64.zip")+": Agentry/agentry-darwin-arm64: missing") {
		t.Errorf("a zip missing a program: %q", got)
	}
}

func hasProblem(problems []string, want string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, want) {
			return true
		}
	}
	return false
}

// builds are a tiny Go program built in a git repo of its own: from the
// commit, from a tree with changes, and with no vcs info.
type builds struct {
	commit                string
	clean                 map[string][]byte // by goos/goarch
	modified, modifiedMac []byte            // linux/amd64 and darwin/amd64
	noVCS                 []byte
}

var cachedBuilds *builds

func newBuilds(t *testing.T) *builds {
	t.Helper()
	if cachedBuilds != nil {
		return cachedBuilds
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go isn't on the PATH, so no program can be built")
	}
	repo := t.TempDir()
	b := &builds{clean: map[string][]byte{}}
	write(t, filepath.Join(repo, "go.mod"), "module example.com/tiny\n\ngo 1.21\n")
	write(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	runIn(t, repo, "git", "init", "-q")
	runIn(t, repo, "git", "add", ".")
	runIn(t, repo, "git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "tiny")
	b.commit = strings.TrimSpace(runIn(t, repo, "git", "rev-parse", "HEAD"))
	for _, target := range []string{"linux/amd64", "darwin/arm64", "darwin/amd64"} {
		b.clean[target] = buildTiny(t, repo, target, "-buildvcs=true")
	}
	b.noVCS = buildTiny(t, repo, "linux/amd64", "-buildvcs=false")
	write(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() { println() }\n")
	b.modified = buildTiny(t, repo, "linux/amd64", "-buildvcs=true")
	b.modifiedMac = buildTiny(t, repo, "darwin/amd64", "-buildvcs=true")
	cachedBuilds = b
	return b
}

// buildTiny builds the program in repo for target, without cgo, as the
// release builds the server.
func buildTiny(t *testing.T, repo, target, vcs string) []byte {
	t.Helper()
	goos, goarch, _ := strings.Cut(target, "/")
	out := filepath.Join(repo, "tiny-"+goos+"-"+goarch)
	build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", vcs, "-o", out, ".")
	build.Dir = repo
	build.Env = append(gitFree(os.Environ()), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOFLAGS=")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build for %s: %v\n%s", target, err, output)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(out) // or the next build's tree has changes
	return data
}

func runIn(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = dir
	command.Env = gitFree(os.Environ())
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

// gitFree is env without git's variables, so a test run from a git hook
// never reaches this repo's own .git.
func gitFree(env []string) []string {
	var kept []string
	for _, v := range env {
		if !strings.HasPrefix(v, "GIT_") {
			kept = append(kept, v)
		}
	}
	return kept
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fatMachO joins thin Mach-O programs into a universal one, as lipo does,
// since lipo is Mac only: a big-endian header, one entry per arch, and each
// program at a 16 KB boundary.
func fatMachO(t *testing.T, programs ...[]byte) []byte {
	t.Helper()
	const align = 14
	var header, body bytes.Buffer
	binary.Write(&header, binary.BigEndian, []uint32{macho.MagicFat, uint32(len(programs))})
	offset := uint32(1 << align)
	for _, program := range programs {
		thin, err := macho.NewFile(bytes.NewReader(program))
		if err != nil {
			t.Fatal(err)
		}
		binary.Write(&header, binary.BigEndian, []uint32{uint32(thin.Cpu), thin.SubCpu, offset, uint32(len(program)), align})
		offset += (uint32(len(program)) + 1<<align - 1) &^ (1<<align - 1)
	}
	fat := make([]byte, 1<<align)
	copy(fat, header.Bytes())
	for _, program := range programs {
		body.Write(program)
		body.Write(make([]byte, (1<<align-len(program)%(1<<align))%(1<<align)))
	}
	return append(fat, body.Bytes()...)
}

// writeArchive writes contents as a zip, or a .tar.gz for that name.
func writeArchive(t *testing.T, path string, contents map[string][]byte) {
	t.Helper()
	var out bytes.Buffer
	if strings.HasSuffix(path, ".zip") {
		z := zip.NewWriter(&out)
		for name, data := range contents {
			w, err := z.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			w.Write(data)
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(&out)
		tw := tar.NewWriter(gz)
		for name, data := range contents {
			if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
				t.Fatal(err)
			}
			tw.Write(data)
		}
		tw.Close()
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
