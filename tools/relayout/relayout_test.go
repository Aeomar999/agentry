package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const storeImport = `"gitlab.com/dev.chempa/agentry/internal/store"`

// fixture is a small repo with the shapes a move meets: a package and one
// whose name it starts, importers in two orders, the spec, a build file, a
// design doc, relayout's own table, and a relative path to edit.
var fixture = map[string]string{
	"internal/store/store.go":         "package store\n",
	"internal/store/fixtures_test.go": "package store\n\nconst module = \"../..\"\n",
	"internal/storetest/storetest.go": "package storetest\n\nimport _ " + storeImport + "\n",
	"internal/web/web.go": "package web\n\nimport (\n" +
		"\t_ \"gitlab.com/dev.chempa/agentry/internal/rules\"\n" +
		"\t_ " + storeImport + "\n" +
		"\t_ \"gitlab.com/dev.chempa/agentry/internal/storetest\"\n)\n",
	"spec/functional/a.feature":   "Feature: a\n  # Sources: internal/store and its tests.\n",
	"Makefile":                    "test:\n\tgo test ./internal/store/... ./internal/storetest\n",
	"docs/design/old.md":          "The log is internal/store.\n",
	"docs/design/old.go":          "package old\n\nimport _ " + storeImport + "\n",
	"tools/relayout/table.go":     "package main\n\nvar old = \"internal/store\"\n",
	"ui/src/lib/chat.test.ts":     "import types from \"../../../internal/store/store.go?raw\";\n",
	"ui/src/components/Logo.svg":  "<svg><!-- internal/store --></svg>\n",
	"ui/public/sw.js":             "// It shows the pushes (internal/store).\n",
	"internal/store/testdata/a.x": "internal/store\n",
}

func storeRun(root string) run {
	return run{
		root:  root,
		moves: []move{{"6", "", "internal/store", "internal/core/store"}},
		edits: []edit{{"6", "", "internal/store/fixtures_test.go", `const module = "../.."`, `const module = "../../.."`}},
		out:   &bytes.Buffer{},
	}
}

func TestAStepMovesTheFolderAndRewritesItsPaths(t *testing.T) {
	root := newRepo(t, fixture)
	if err := storeRun(root).do(); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(root, "internal/store")) {
		t.Error("internal/store is still there")
	}
	for file, want := range map[string]string{
		"internal/core/store/store.go":         "package store\n",
		"internal/core/store/testdata/a.x":     "internal/store\n",
		"internal/core/store/fixtures_test.go": "package store\n\nconst module = \"../../..\"\n",
		"internal/storetest/storetest.go":      "package storetest\n\nimport _ \"gitlab.com/dev.chempa/agentry/internal/core/store\"\n",
		// gofmt sorts internal/core/store above internal/rules again.
		"internal/web/web.go": "package web\n\nimport (\n" +
			"\t_ \"gitlab.com/dev.chempa/agentry/internal/core/store\"\n" +
			"\t_ \"gitlab.com/dev.chempa/agentry/internal/rules\"\n" +
			"\t_ \"gitlab.com/dev.chempa/agentry/internal/storetest\"\n)\n",
		"spec/functional/a.feature":  "Feature: a\n  # Sources: internal/core/store and its tests.\n",
		"Makefile":                   "test:\n\tgo test ./internal/core/store/... ./internal/storetest\n",
		"ui/src/lib/chat.test.ts":    "import types from \"../../../internal/core/store/store.go?raw\";\n",
		"ui/public/sw.js":            "// It shows the pushes (internal/core/store).\n",
		"docs/design/old.md":         fixture["docs/design/old.md"],
		"docs/design/old.go":         fixture["docs/design/old.go"],
		"tools/relayout/table.go":    fixture["tools/relayout/table.go"],
		"ui/src/components/Logo.svg": fixture["ui/src/components/Logo.svg"],
	} {
		if got := read(t, root, file); got != want {
			t.Errorf("%s:\n%s\nwant:\n%s", file, got, want)
		}
	}
}

func TestASecondRunChangesNothing(t *testing.T) {
	root := newRepo(t, fixture)
	if err := storeRun(root).do(); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)
	again := storeRun(root)
	if err := again.do(); err != nil {
		t.Fatal(err)
	}
	if status := gitStatus(t, root); status != "" {
		t.Errorf("a second run changed:\n%s", status)
	}
	if said := again.out.(*bytes.Buffer).String(); said != "" {
		t.Errorf("a second run said %q, want nothing", said)
	}
}

// A branch rebased over the move brings a new file in the old folder and a
// new import of the old path; a run moves and rewrites those, and only
// those.
func TestARunOnARebasedBranchMovesStraysAndRewritesOldImports(t *testing.T) {
	root := newRepo(t, fixture)
	if err := storeRun(root).do(); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)
	write(t, root, "internal/store/stray.go", "package store\n")
	write(t, root, "internal/store/sub/deep.go", "package sub\n")
	write(t, root, "internal/web/new.go", "package web\n\nimport _ "+storeImport+"\n")
	if _, err := git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := storeRun(root).do(); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(root, "internal/store")) {
		t.Error("internal/store is still there")
	}
	if got := read(t, root, "internal/web/new.go"); !strings.Contains(got, "internal/core/store") {
		t.Errorf("new.go still imports the old path:\n%s", got)
	}
	want := "A  internal/core/store/stray.go\nA  internal/core/store/sub/deep.go\nAM internal/web/new.go\n"
	if status := gitStatus(t, root); status != want {
		t.Errorf("status:\n%s\nwant:\n%s", status, want)
	}
}

// A stray whose name the new folder already holds is for a person.
func TestAStrayThatClashesStops(t *testing.T) {
	root := newRepo(t, fixture)
	if err := storeRun(root).do(); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)
	write(t, root, "internal/store/store.go", "package store\n\n// changed\n")
	if _, err := git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	err := storeRun(root).do()
	if err == nil || !strings.Contains(err.Error(), "internal/store/store.go is still there and so is internal/core/store/store.go") {
		t.Errorf("err = %v, want the clash named", err)
	}
}

// A relative path relayout doesn't know stops the run before it changes
// anything, naming the file and line; the same in an allowed file doesn't.
func TestAnUnknownRelativePathStopsTheRunFirst(t *testing.T) {
	files := map[string]string{"internal/store/walk.go": "package store\n\nimport \"os\"\n\nfunc f() { os.Chdir(\"..\") }\n"}
	for name, text := range fixture {
		files[name] = text
	}
	root := newRepo(t, files)
	err := storeRun(root).do()
	if err == nil || !strings.Contains(err.Error(), `internal/store/walk.go:5: func f() { os.Chdir("..") }`) {
		t.Errorf("err = %v, want walk.go:5 named", err)
	}
	if status := gitStatus(t, root); status != "" {
		t.Errorf("a stopped run changed:\n%s", status)
	}

	r := storeRun(root)
	r.allowed = map[string]string{"internal/store/walk.go": "a test of Chdir"}
	if err := r.do(); err != nil {
		t.Errorf("an allowed file stopped the run: %v", err)
	}
}

func TestAnEditThatNoLongerFitsStops(t *testing.T) {
	root := newRepo(t, fixture)
	write(t, root, "internal/store/fixtures_test.go", "package store\n\nconst root = \"../..\"\n")
	err := storeRun(root).do()
	if err == nil || !strings.Contains(err.Error(), "internal/core/store/fixtures_test.go holds neither") {
		t.Errorf("err = %v, want the edit named", err)
	}
}

func TestADryRunChangesNothingAndSaysWhat(t *testing.T) {
	root := newRepo(t, fixture)
	r := storeRun(root)
	r.dry = true
	if err := r.do(); err != nil {
		t.Fatal(err)
	}
	if status := gitStatus(t, root); status != "" {
		t.Errorf("a dry run changed:\n%s", status)
	}
	want := "would git mv internal/store internal/core/store\n" +
		"would edit internal/store/fixtures_test.go: const module = \"../..\" -> const module = \"../../..\"\n" +
		"would rewrite old paths in 6 files\n"
	if said := r.out.(*bytes.Buffer).String(); said != want {
		t.Errorf("said:\n%s\nwant:\n%s", said, want)
	}
}

// newRepo makes a git repo in a temp folder holding files, committed.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if _, err := git(root, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		write(t, root, name, text)
	}
	commitAll(t, root)
	return root
}

func commitAll(t *testing.T, root string) {
	t.Helper()
	if _, err := git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := git(root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "x"); err != nil {
		t.Fatal(err)
	}
}

func gitStatus(t *testing.T, root string) string {
	t.Helper()
	out, err := git(root, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func write(t *testing.T, root, name, text string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// gofmt gets each changed Go file once, though an edited file can also be
// rewritten.
func TestEachGoFileIsFormattedOnce(t *testing.T) {
	got := goFilesOf([]string{"b.go", "a.go", "spec/check", "b.go"})
	if want := []string{"a.go", "b.go"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
