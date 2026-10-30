package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sourcesLines reads a feature file's "# Sources:" sentence: the comment
// line that starts with it and each comment line after it, up to the line
// that ends the sentence with a full stop, or up to a blank line or one that
// isn't a comment. The words in it are read as citations are (commentTokens):
// a token is a path only when it starts with a top folder and a slash, or is
// a root file's bare name, so "and their tests" or "its tests" is never one.
func sourcesLines(text string) []line {
	var sources []line
	in := false
	for i, raw := range strings.Split(text, "\n") {
		l := line{number: i + 1, text: strings.TrimSpace(raw)}
		if !in {
			in = strings.HasPrefix(l.text, "# Sources:")
		} else if !isComment(l.text) {
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
// caught.
func (c *citer) sourcesProblems(path, text string) []Problem {
	var problems []Problem
	for _, l := range sourcesLines(text) {
		for _, token := range commentTokens(l.text) {
			message := ""
			if isPath(token) {
				if _, err := os.Stat(filepath.Join(c.root, filepath.FromSlash(token))); err != nil {
					message = fmt.Sprintf("Sources names %s, which doesn't exist", token)
				}
			} else if folder, ok := unlistedFolder(token); ok {
				message = c.check(citation{line: l.number, path: token, folder: folder})
			}
			if message != "" {
				problems = append(problems, Problem{path, l.number, message})
			}
		}
	}
	return problems
}
