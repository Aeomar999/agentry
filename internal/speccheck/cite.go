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
// one of them and a slash is a path. rootFiles are the files at the repo
// root a comment may cite by their bare name. spec/README.md, Citations,
// repeats both lists.
var (
	rootFolders = []string{"cmd", "deploy", "docs", "internal", "packaging", "spec", "tools", "ui"}
	rootFiles   = []string{".gitlab-ci.yml", "Dockerfile", "Makefile", "PLAN.md", "README.md", "docker-stack.dev.yml", "go.mod", "spec.md"}
)

// testName matches a Go test's name at the start of a token.
var testName = regexp.MustCompile(`^Test[A-Z0-9_][A-Za-z0-9_]*`)

// A citation is a repo path, or a Go test in a test file, that a comment
// line under a scenario names.
type citation struct {
	line int
	path string // relative to the repo root, with slashes
	test string // the Go test's name, or "" for a path on its own
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
// around them cut off, so "internal/x_test.go;" reads as the path.
func commentTokens(text string) []string {
	var tokens []string
	for _, field := range strings.Fields(strings.TrimPrefix(text, "#")) {
		token := strings.TrimLeft(field, "(\"'`")
		token = strings.TrimRight(token, ",;:.)\"'`")
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
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

func isGoTestFile(path string) bool { return strings.HasSuffix(path, "_test.go") }

// A citer judges citations against the repo, reading each test file once.
type citer struct {
	root  string
	files map[string]string // test file path -> its text, "" when unreadable
	paths int               // cited paths checked
	tests int               // cited Go tests checked
}

func newCiter(root string) *citer {
	return &citer{root: root, files: map[string]string{}}
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
	text, readable := c.testFile(cite.path)
	if !readable {
		return "" // the missing path is already a problem of its own
	}
	c.tests++
	if !regexp.MustCompile(`(?m)^func ` + cite.test + `\(`).MatchString(text) {
		return fmt.Sprintf("%s is not defined in %s", cite.test, cite.path)
	}
	return ""
}

func (c *citer) testFile(path string) (text string, readable bool) {
	if text, read := c.files[path]; read {
		return text, text != ""
	}
	bytes, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(path)))
	c.files[path] = string(bytes)
	return string(bytes), err == nil && len(bytes) > 0
}
