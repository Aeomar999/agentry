package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// A run is one step, or one area of step 7, on the repo at root. With dry
// set it changes nothing and says what it would do.
type run struct {
	root    string
	moves   []move
	edits   []edit
	allowed map[string]string
	dry     bool
	out     io.Writer
}

// do makes the run's moves: it stops before changing anything on a
// relative path it doesn't know, then moves each folder, makes the edits,
// rewrites the old paths and formats the Go files it changed. Each part
// does only what is still undone, so running a step again on a moved tree
// changes nothing, and on a rebased branch only moves stray files and
// rewrites the old paths they brought.
func (r run) do() error {
	if err := r.checkRelativePaths(); err != nil {
		return err
	}
	for _, m := range r.moves {
		if err := r.moveFolder(m); err != nil {
			return err
		}
	}
	var changed []string
	for _, e := range r.edits {
		file, err := r.applyEdit(e)
		if err != nil {
			return err
		}
		changed = append(changed, file...)
	}
	rewritten, err := r.rewritePaths()
	if err != nil {
		return err
	}
	return r.format(append(changed, rewritten...))
}

// say reports one thing the run does, or would do.
func (r run) say(format string, args ...any) {
	if r.dry {
		format = "would " + format
	}
	fmt.Fprintf(r.out, format+"\n", args...)
}

// relativePath finds what section 7's grep finds: a path string starting
// with .., a filepath.Join("..", ...) and a Chdir.
var relativePath = regexp.MustCompile(`"\.\./|"\.\."|Chdir\(`)

// checkRelativePaths stops the run on a relative path or Chdir in a moved
// package's Go file that is in neither the run's edits nor its allowed
// files, since a person has to decide it. It looks before anything moves,
// so it stops the run with the tree as it was.
func (r run) checkRelativePaths() error {
	files, err := r.packageFiles()
	if err != nil {
		return err
	}
	var hits []string
	for _, f := range files {
		if !strings.HasSuffix(f.now, ".go") || r.isEdited(f.today) || r.allowed[f.today] != "" {
			continue
		}
		found, err := grep(filepath.Join(r.root, f.now), relativePath)
		if err != nil {
			return err
		}
		for _, h := range found {
			hits = append(hits, f.now+":"+h)
		}
	}
	if len(hits) > 0 {
		return fmt.Errorf("relative paths in moved packages that tools/relayout/table.go neither edits nor allows; fix each by hand, or add its file to edits or allowed:\n  %s", strings.Join(hits, "\n  "))
	}
	return nil
}

func (r run) isEdited(file string) bool {
	return slices.ContainsFunc(r.edits, func(e edit) bool { return e.file == file })
}

// A packageFile is a tracked file of a package the run moves: where it is
// now, and its path today, before any move.
type packageFile struct{ now, today string }

// packageFiles lists the tracked files of the run's packages, at their old
// folders and their new ones. A folder nested in another row's folder
// (workers/workerlink) keeps its own files.
func (r run) packageFiles() ([]packageFile, error) {
	byNew := slices.Clone(r.moves)
	slices.SortFunc(byNew, func(a, b move) int { return len(b.new) - len(a.new) })
	var files []packageFile
	seen := map[string]bool{}
	for _, m := range byNew {
		for _, folder := range []string{m.new, m.old} {
			tracked, err := trackedFiles(r.root, folder)
			if err != nil {
				return nil, err
			}
			for _, f := range tracked {
				if !seen[f] {
					seen[f] = true
					files = append(files, packageFile{now: f, today: m.old + strings.TrimPrefix(f, folder)})
				}
			}
		}
	}
	return files, nil
}

// grep gives each line of a file that re matches, as "n: line".
func grep(path string, re *regexp.Regexp) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var hits []string
	lines := bufio.NewScanner(f)
	lines.Buffer(nil, 1<<20)
	for n := 1; lines.Scan(); n++ {
		if re.MatchString(lines.Text()) {
			hits = append(hits, fmt.Sprintf("%d: %s", n, strings.TrimSpace(lines.Text())))
		}
	}
	return hits, lines.Err()
}

// moveFolder git mvs a package's folder to its new path. When the new
// folder is already there, as on a branch rebased over the move, it moves
// only the files still in the old folder, the strays.
func (r run) moveFolder(m move) error {
	left, err := trackedFiles(r.root, m.old)
	if err != nil || len(left) == 0 {
		return err
	}
	if !exists(filepath.Join(r.root, m.new)) {
		return r.gitMove(m.old, m.new)
	}
	for _, f := range left {
		to := m.new + strings.TrimPrefix(f, m.old)
		if exists(filepath.Join(r.root, to)) {
			return fmt.Errorf("%s is still there and so is %s: merge them by hand, then run the step again", f, to)
		}
		if err := r.gitMove(f, to); err != nil {
			return err
		}
	}
	if !r.dry {
		removeEmptyFolders(filepath.Join(r.root, m.old))
	}
	return nil
}

func (r run) gitMove(from, to string) error {
	r.say("git mv %s %s", from, to)
	if r.dry {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(r.root, filepath.Dir(to)), 0o755); err != nil {
		return err
	}
	_, err := git(r.root, "mv", from, to)
	return err
}

// removeEmptyFolders removes folder and the folders in it that git mv of
// their files left empty; a folder that still holds a file stays.
func removeEmptyFolders(folder string) {
	var folders []string
	filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			folders = append(folders, path)
		}
		return nil
	})
	for _, f := range slices.Backward(folders) {
		os.Remove(f) // fails, as it should, on a folder that isn't empty
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// applyEdit makes one edit in its file, wherever the run's moves put it,
// and returns the file when it changed. A done edit, its new text there and
// its old text gone, is skipped; an edit whose texts are both gone no
// longer fits the file, so it stops the run.
func (r run) applyEdit(e edit) ([]string, error) {
	file := r.whereNow(e.file)
	path := filepath.Join(r.root, file)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("edit of %s: %w", e.file, err)
	}
	text := string(data)
	switch {
	case strings.Contains(text, e.old):
	case strings.Contains(text, e.new):
		return nil, nil
	default:
		return nil, fmt.Errorf("%s holds neither %q nor %q: update its edit in tools/relayout/table.go", file, e.old, e.new)
	}
	r.say("edit %s: %s -> %s", file, e.old, e.new)
	if r.dry {
		return nil, nil
	}
	return []string{file}, overwrite(path, strings.ReplaceAll(text, e.old, e.new))
}

// overwrite writes text over a file that is there, so its mode, such as
// spec/check's executable bit, stays.
func overwrite(path, text string) error {
	return os.WriteFile(path, []byte(text), 0)
}

// whereNow gives a file's path after the run's moves, or its path today
// when it hasn't moved yet, as in a dry run.
func (r run) whereNow(file string) string {
	moved := rewriteText(file, r.moves)
	if exists(filepath.Join(r.root, moved)) {
		return moved
	}
	return file
}

// rewritePaths rewrites the run's old paths in every file that names
// packages by path (see namesPaths), and returns the files it changed.
func (r run) rewritePaths() ([]string, error) {
	files, err := trackedFiles(r.root, ".")
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, f := range files {
		if !namesPaths(f) {
			continue
		}
		path := filepath.Join(r.root, f)
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue // a deletion not yet committed
		} else if err != nil {
			return nil, err
		}
		text := rewriteText(string(data), r.moves)
		if text == string(data) {
			continue
		}
		changed = append(changed, f)
		if !r.dry {
			if err := overwrite(path, text); err != nil {
				return nil, err
			}
		}
	}
	if len(changed) > 0 {
		r.say("rewrite old paths in %d files", len(changed))
	}
	return changed, nil
}

// buildFiles are the files outside spec/, the Go code and the UI's sources
// that name packages by path: the build files, and the service worker,
// whose comment names the package that sends its pushes.
var buildFiles = []string{"Makefile", ".gitlab-ci.yml", "README.md", "ui/vite.config.ts", "ui/public/sw.js"}

// namesPaths says whether the rewrite reads a file: Go files, everything
// in spec/, the build files, and the UI's sources, whose comments name Go
// packages and whose tests import a Go file (?raw). Design docs keep the
// paths they were written with, and relayout's own table its old paths.
func namesPaths(file string) bool {
	switch {
	case strings.HasPrefix(file, "docs/design/"), strings.HasPrefix(file, "tools/relayout/"):
		return false
	case strings.HasSuffix(file, ".go"), strings.HasPrefix(file, "spec/"):
		return true
	case strings.HasPrefix(file, "ui/src/"):
		return strings.HasSuffix(file, ".ts") || strings.HasSuffix(file, ".tsx")
	}
	return slices.Contains(buildFiles, file)
}

// format runs gofmt -w on the Go files among files, so the import blocks
// are sorted again.
func (r run) format(files []string) error {
	goFiles := goFilesOf(files)
	if r.dry || len(goFiles) == 0 {
		return nil
	}
	cmd := exec.Command("gofmt", append([]string{"-w"}, goFiles...)...)
	cmd.Dir = r.root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gofmt: %v: %s", err, out)
	}
	return nil
}

// goFilesOf gives the Go files among files, each once: a file both edited
// and rewritten is listed twice, and gofmt formats its files in parallel,
// so two writes of one file would race.
func goFilesOf(files []string) []string {
	var goFiles []string
	for _, f := range files {
		if strings.HasSuffix(f, ".go") {
			goFiles = append(goFiles, f)
		}
	}
	slices.Sort(goFiles)
	return slices.Compact(goFiles)
}
