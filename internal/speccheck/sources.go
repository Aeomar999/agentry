package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// sourcesWords are the words a Sources sentence may hold besides its paths:
// "Sources: internal/x, ui/src/y.tsx and their tests." Every other word is a
// path.
var sourcesWords = map[string]bool{"Sources": true, "and": true, "their": true, "its": true, "tests": true}

// sourcesLines reads a feature file's "# Sources:" sentence: the comment
// line that starts with it and each comment line after it, up to the line
// that ends the sentence with a full stop, or up to a blank line, one that
// isn't a comment or a "# ----" rule.
func sourcesLines(text string) []line {
	var sources []line
	in := false
	for i, raw := range strings.Split(text, "\n") {
		l := line{number: i + 1, text: strings.TrimSpace(raw)}
		if !in {
			in = strings.HasPrefix(l.text, "# Sources:")
		} else if !isComment(l.text) || strings.HasPrefix(l.text, "# ----") {
			in = false
		}
		if in {
			sources = append(sources, l)
			in = !strings.HasSuffix(l.text, ".")
		}
	}
	return sources
}

// sourcesProblems checks that every path a feature file's Sources names is
// in the repo, as a file or a folder, so a move that forgets the spec is
// caught. A functional file must have a Sources sentence; any other may.
func (c *citer) sourcesProblems(path, text string, required bool) []Problem {
	sources := sourcesLines(text)
	if required && len(sources) == 0 {
		return []Problem{{path, 1, "no # Sources: line under the description; name the code behind this file"}}
	}
	var problems []Problem
	for _, l := range sources {
		for _, token := range commentTokens(l.text) {
			if message := c.sourceProblem(token); message != "" {
				problems = append(problems, Problem{path, l.number, message})
			}
		}
	}
	return problems
}

// sourceProblem says what is wrong with one word of a Sources sentence, or
// "". Every word but sourcesWords is a path from the repo root, with or
// without a slash, so a typo such as intenal/x or a bare AgentPage.tsx fails
// rather than passing as a word. A path that starts with ./ is refused, so
// the spec names each path one way.
func (c *citer) sourceProblem(token string) string {
	if sourcesWords[token] {
		return ""
	}
	if strings.HasPrefix(token, "./") {
		return fmt.Sprintf("Sources names %s; name the path from the repo root, without ./", token)
	}
	if folder, ok := unlistedFolder(token); ok && !isPath(token) && c.isRepoFolder(folder) {
		return c.check(citation{path: token, folder: folder})
	}
	switch {
	case existsExactly(c.root, token):
		return ""
	case strings.Contains(token, "/"):
		return fmt.Sprintf("Sources names %s, which doesn't exist", token)
	case strings.Contains(token, "."):
		return fmt.Sprintf("Sources names %s, which isn't at the repo root; name the path from the repo root", token)
	default:
		return fmt.Sprintf(`Sources names %s, which isn't a path; a Sources sentence holds only paths and "and their/its tests", ending with a full stop`, token)
	}
}

// existsExactly is true when the path, with slashes, is under root with
// each part's case as written. os.Stat alone would let Internal/x pass on a
// case-insensitive filesystem, such as the Mac's.
func existsExactly(root, path string) bool {
	dir := root
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil || !slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.Name() == part }) {
			return false
		}
		dir = filepath.Join(dir, part)
	}
	return true
}
