package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// specifiedFolders are the folders, from the repo root, whose every Go
// package a feature file's Sources must name: a package nobody specified is
// either dead or missing its spec.
var specifiedFolders = []string{"internal/domain", "internal/extensions"}

// noSpecYet is what the list "Code with no spec yet" in spec/README.md held
// when this rule came in. An entry comes off the list when a feature file's
// Sources names it; a package not here may not go on, so the list only
// shrinks.
var noSpecYet = []string{"internal/extensions/schedules"}

const noSpecYetHeading = "## Code with no spec yet"

// nameSources records the paths a feature file's Sources names, each
// without a folder's slash at the end, so that unspecifiedProblems can tell
// which packages no feature file names.
func (c *citer) nameSources(path, text string) {
	for _, l := range sourcesLines(text) {
		for _, token := range commentTokens(l.text) {
			name := strings.TrimSuffix(token, "/")
			if _, seen := c.sourced[name]; !seen && !sourcesWords[token] {
				c.sourced[name] = path
			}
		}
	}
}

// unspecifiedProblems checks that every package under specifiedFolders is
// named by a feature file's Sources, as a folder: a folder names its own
// package and not its subpackages, so internal/extensions is the framework
// and not internal/extensions/ideas. A package on the folder's README.md
// list "Code with no spec yet" is let off. The list may only shrink: an
// entry fails when noSpecYet doesn't hold it, when a Sources names it, or
// when it is not such a package. noSpecYet shrinks with it: a package it
// holds fails when the list doesn't, so an entry taken off can't come back.
func (c *citer) unspecifiedProblems(folder string) ([]Problem, error) {
	readme := filepath.Join(folder, "README.md")
	listed, heading, err := noSpecYetList(readme)
	if err != nil {
		return nil, err
	}
	packages, err := specifiedPackages(c.root)
	if err != nil {
		return nil, err
	}
	var problems []Problem
	for _, pkg := range packages {
		if _, ok := c.sourced[pkg]; !ok && !slices.ContainsFunc(listed, func(e listEntry) bool { return e.path == pkg }) {
			problems = append(problems, Problem{readme, heading, fmt.Sprintf(
				"%s is named by no feature file's # Sources: line; name it in the Sources of the feature file that specifies it", pkg)})
		}
	}
	for _, e := range listed {
		message := ""
		switch file, named := c.sourced[e.path]; {
		case !slices.Contains(noSpecYet, e.path):
			message = fmt.Sprintf("%s is on Code with no spec yet, which may only shrink; name it in the Sources of the feature file that specifies it", e.path)
		case named:
			message = fmt.Sprintf("%s is on Code with no spec yet, and %s's Sources names it; take it off the list", e.path, file)
		case !slices.Contains(packages, e.path):
			message = fmt.Sprintf("%s is on Code with no spec yet, and is no package under %s; take it off the list", e.path, strings.Join(specifiedFolders, " or "))
		}
		if message != "" {
			problems = append(problems, Problem{readme, e.line, message})
		}
	}
	for _, pkg := range noSpecYet {
		if slices.Contains(packages, pkg) && !slices.ContainsFunc(listed, func(e listEntry) bool { return e.path == pkg }) {
			problems = append(problems, Problem{readme, heading, fmt.Sprintf(
				"%s is in noSpecYet but not on Code with no spec yet; drop it from noSpecYet in tools/speccheck/specified.go too", pkg)})
		}
	}
	return problems, nil
}

// A listEntry is one path on the list "Code with no spec yet".
type listEntry struct {
	path string
	line int
}

// noSpecYetList reads the list "Code with no spec yet" from a README: the
// first quoted path of each "- " line under the heading, up to the next
// heading. It gives back the heading's line too, or 1 when there is no
// such heading or no README.
func noSpecYetList(readme string) ([]listEntry, int, error) {
	text, err := os.ReadFile(readme)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 1, nil
	} else if err != nil {
		return nil, 0, err
	}
	var entries []listEntry
	heading, in := 1, false
	for i, raw := range strings.Split(string(text), "\n") {
		switch {
		case raw == noSpecYetHeading:
			heading, in = i+1, true
		case strings.HasPrefix(raw, "#"):
			in = false
		case in && strings.HasPrefix(raw, "- `"):
			path, _, _ := strings.Cut(strings.TrimPrefix(raw, "- `"), "`")
			entries = append(entries, listEntry{strings.TrimSuffix(path, "/"), i + 1})
		}
	}
	return entries, heading, nil
}

// specifiedPackages lists the Go packages under specifiedFolders, as paths
// from the repo root: each folder that holds a .go file other than a test.
// A test helper, a <name>test package beside its <name>, is left out, and
// so is what the go command skips: testdata and folders starting with . or _.
func specifiedPackages(root string) ([]string, error) {
	var packages []string
	for _, top := range specifiedFolders {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, entry fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && path == filepath.Join(root, top) {
				return fs.SkipDir
			} else if err != nil {
				return err
			}
			name := entry.Name()
			if entry.IsDir() && path != filepath.Join(root, top) &&
				(name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return fs.SkipDir
			}
			if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			dir := filepath.Dir(path)
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				return err
			}
			pkg := filepath.ToSlash(rel)
			if filepath.Base(dir) != filepath.Base(filepath.Dir(dir))+"test" && !slices.Contains(packages, pkg) {
				packages = append(packages, pkg)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return packages, nil
}
