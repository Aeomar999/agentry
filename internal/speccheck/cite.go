package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// rootFolders are the repo's top folders: a cited token that starts with
// one of them and a slash is a path. A token under another folder of the
// repo root is a problem, so the list is kept up through what the spec
// cites. rootFiles are the files at the repo root a comment may cite by
// their bare name. spec/README.md, Citations, repeats both lists.
var (
	rootFolders = []string{"cmd", "deploy", "docs", "internal", "packaging", "spec", "tools", "ui"}
	rootFiles   = []string{
		".dockerignore", ".gitignore", ".gitlab-ci.yml", "Dockerfile", "LICENSE", "Makefile", "NOTICE",
		"PLAN.md", "README.md", "docker-stack.dev.yml", "go.mod", "go.sum", "spec.md",
	}
)

// testName matches a Go test's name at the start of a token.
var testName = regexp.MustCompile(`^Test[A-Z0-9_][A-Za-z0-9_]*`)

// lineSuffix is a line number or range after a path, as in x.go:42 or
// x.go:3-5.
var lineSuffix = regexp.MustCompile(`:[0-9]+(-[0-9]+)?$`)

// folderName is a word that could name a folder: the part of a token
// before its first slash.
var folderName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// A citation is a repo path, or a Go test in a test file, that a comment
// line under a scenario names. A token such as scripts/x.sh, under a
// folder rootFolders does not list, is a citation of that folder: it is a
// problem only when the folder is there at the repo root.
type citation struct {
	line   int
	path   string // relative to the repo root, with slashes
	test   string // the Go test's name, or "" for a path on its own
	folder string // the unlisted top folder a token starts with, or ""
}

// citations reads what a scenario's comment lines cite, in order. A Go test
// name belongs to the test file named right after it ("TestA in x_test.go"),
// or else to the last Go test file cited above it in the scenario, on its
// own line or an earlier one. A test name with no such file has an empty
// path. A glob is not a path and is skipped.
func citations(s scenario) []citation {
	var cited []citation
	lastTestFile := ""
	for _, l := range s.body {
		if !isComment(l.text) {
			continue
		}
		tokens := commentTokens(l.text)
		for i, token := range tokens {
			if isPath(token) {
				cited = append(cited, citation{line: l.number, path: token})
				if isGoTestFile(token) {
					lastTestFile = token
				}
				continue
			}
			name := testName.FindString(token)
			if name == "" {
				if folder, ok := unlistedFolder(token); ok {
					cited = append(cited, citation{line: l.number, path: token, folder: folder})
				}
				continue
			}
			file := lastTestFile
			if i+2 < len(tokens) && tokens[i+1] == "in" && isPath(tokens[i+2]) && isGoTestFile(tokens[i+2]) {
				file = tokens[i+2]
			}
			cited = append(cited, citation{line: l.number, path: file, test: name})
		}
	}
	return cited
}

// commentTokens splits a comment line into words with the punctuation
// around them cut off, so "internal/x_test.go;" reads as the path, and with
// a line number cut off, so "internal/x.go:42" does too.
func commentTokens(text string) []string {
	var tokens []string
	for _, field := range strings.Fields(strings.TrimPrefix(text, "#")) {
		if token := lineSuffix.ReplaceAllString(trimToken(field), ""); token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// trimToken cuts the punctuation around a word of a comment.
func trimToken(field string) string {
	token := strings.TrimLeft(field, "(\"'`")
	return strings.TrimRight(token, ",;:.)\"'`")
}

// citesLineOfPath is true when a comment line cites a path with a line
// number after it, such as internal/x.go:42 or internal/x.go:3-5.
func citesLineOfPath(text string) bool {
	for _, field := range strings.Fields(strings.TrimPrefix(text, "#")) {
		token := trimToken(field)
		if lineSuffix.MatchString(token) && isPath(lineSuffix.ReplaceAllString(token, "")) {
			return true
		}
	}
	return false
}

// isPath is true for a token that names a file or folder of the repo: one
// that starts with a top folder and a slash, or a root file's bare name. A
// glob such as ui/src/**/*.tsx is not a path.
func isPath(token string) bool {
	if strings.ContainsAny(token, "*?[{<") {
		return false
	}
	if slices.Contains(rootFiles, token) {
		return true
	}
	first, _, found := strings.Cut(token, "/")
	return found && slices.Contains(rootFolders, first)
}

// unlistedFolder gives the first part of a token that is not a path but
// could be one under a top folder rootFolders does not list. A dot folder,
// such as .agentry, is not the repo's.
func unlistedFolder(token string) (folder string, ok bool) {
	if strings.ContainsAny(token, "*?[{<") {
		return "", false
	}
	folder, rest, found := strings.Cut(token, "/")
	if !found || rest == "" || !folderName.MatchString(folder) {
		return "", false
	}
	return folder, true
}

func isGoTestFile(path string) bool { return strings.HasSuffix(path, "_test.go") }

// A citer judges citations against the repo, reading each test file once.
type citer struct {
	root    string
	files   map[string]testFile // by path
	ignored []string            // top folders .gitignore leaves out, read once
	paths   int                 // cited paths checked
	tests   int                 // cited Go tests checked
}

// A testFile is a cited test file as it was read.
type testFile struct {
	text     string
	readable bool
}

func newCiter(root string) *citer {
	return &citer{root: root, files: map[string]testFile{}}
}

// problems checks a @test scenario's citations: every cited path exists,
// and every cited Go test is defined in its test file.
func (c *citer) problems(s scenario) []Problem {
	var problems []Problem
	for _, cite := range citations(s) {
		if message := c.check(cite); message != "" {
			problems = append(problems, Problem{s.file, cite.line, message})
		}
	}
	return problems
}

// check says what is wrong with one citation, or "".
func (c *citer) check(cite citation) string {
	if cite.folder != "" {
		if !c.isRepoFolder(cite.folder) {
			return "" // a word such as and/or, or a folder git leaves out
		}
		return fmt.Sprintf("cited path %s is under %s/, a top folder rootFolders does not list; add %s to rootFolders in internal/speccheck/cite.go and to the list in spec/README.md",
			cite.path, cite.folder, cite.folder)
	}
	if cite.test == "" {
		c.paths++
		if _, err := os.Stat(filepath.Join(c.root, filepath.FromSlash(cite.path))); err != nil {
			return fmt.Sprintf("cited path %s does not exist", cite.path)
		}
		return ""
	}
	if cite.path == "" {
		return fmt.Sprintf("%s is cited with no Go test file before it; name the _test.go file", cite.test)
	}
	text, readable := c.read(cite.path)
	if !readable {
		return "" // the missing path is already a problem of its own
	}
	c.tests++
	if !regexp.MustCompile(`(?m)^func ` + cite.test + `\(`).MatchString(text) {
		return fmt.Sprintf("%s is not defined in %s", cite.test, cite.path)
	}
	return ""
}

// isRepoFolder is true when the name is a folder at the repo root that
// .gitignore does not leave out, as it does bin/ and release/.
func (c *citer) isRepoFolder(name string) bool {
	info, err := os.Stat(filepath.Join(c.root, name))
	if err != nil || !info.IsDir() {
		return false
	}
	if c.ignored == nil {
		c.ignored = ignoredFolders(c.root)
	}
	return !slices.Contains(c.ignored, name)
}

// ignoredFolders are the folders the repo root's .gitignore names on a
// line of their own, such as "bin/".
func ignoredFolders(root string) []string {
	folders := []string{}
	text, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	for _, l := range strings.Split(string(text), "\n") {
		if name, isFolder := strings.CutSuffix(strings.TrimSpace(l), "/"); isFolder && folderName.MatchString(name) {
			folders = append(folders, name)
		}
	}
	return folders
}

// read gives a cited test file's text, reading the file the first time.
func (c *citer) read(path string) (text string, readable bool) {
	file, read := c.files[path]
	if !read {
		bytes, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(path)))
		file = testFile{text: string(bytes), readable: err == nil}
		c.files[path] = file
	}
	return file.text, file.readable
}
