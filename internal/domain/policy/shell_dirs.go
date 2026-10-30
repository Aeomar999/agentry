package policy

import (
	"path/filepath"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// dirBuiltins are the builtins that change the shell's folder.
var dirBuiltins = map[string]bool{"cd": true, "pushd": true, "popd": true}

// followCd moves dirs when the command may be cd, pushd or popd, as `builtin
// cd` and `command cd` are too.
func (s *shellInspector) followCd(args []*syntax.Word) {
	names, args := s.dirBuiltin(args)
	switch {
	case len(names) == 0:
	case slices.Contains(names, "popd"), slices.Contains(names, "pushd") && len(args) == 0:
		s.dirs = nil // the folder stack isn't followed
	default:
		s.moveTo(args)
	}
}

// dirBuiltin is the names the command may run when one of them is cd, pushd
// or popd, and the words after the name. A name that can't be known may be
// any of them.
func (s *shellInspector) dirBuiltin(args []*syntax.Word) ([]string, []*syntax.Word) {
	for len(args) > 0 {
		names, ok := s.values(args[0])
		switch {
		case !ok:
			return []string{"popd"}, nil
		case slices.ContainsFunc(names, func(name string) bool { return dirBuiltins[name] }):
			return names, args[1:]
		case len(names) == 1 && (names[0] == "builtin" || names[0] == "command"):
			args = args[1:]
			continue
		}
		return nil, nil
	}
	return nil, nil
}

// cdOptions are cd's and pushd's options that don't change where they go.
var cdOptions = []string{"-L", "-P", "-e", "-@", "--"}

// moveTo adds the folder cd or pushd goes to, from each folder the shell may
// be in. With no folder, cd goes home. cd - and a folder that can't be known
// leave dirs unknown.
func (s *shellInspector) moveTo(args []*syntax.Word) {
	for len(args) > 0 && slices.ContainsFunc(cdOptions, func(option string) bool { return s.isText(args[0], option) }) {
		args = args[1:]
	}
	targets := []string{s.home}
	if len(args) > 0 {
		fields, ok := s.fields(args[0])
		if !ok || len(args) > 1 {
			s.dirs = nil
			return
		}
		targets = nil
		for _, words := range fields {
			if len(words) != 1 || words[0] == "-" || words[0] == "" {
				s.dirs = nil
				return
			}
			targets = append(targets, words[0])
		}
	}
	dirs := s.dirs
	for _, target := range targets {
		for _, dir := range s.resolveClean(target) {
			if !slices.Contains(dirs, dir) {
				dirs = append(dirs, dir)
			}
		}
	}
	if s.dirs == nil || len(dirs) > maxValues {
		dirs = nil
	}
	s.dirs = dirs
}

// resolveClean is resolve's paths cleaned, for comparing with the workspace.
func (s *shellInspector) resolveClean(path string) []string {
	paths := s.resolve(path)
	for i, path := range paths {
		paths[i] = filepath.Clean(path)
	}
	return paths
}

// resolve is every file a path may name from the folders the shell may be
// in. With them unknown, a relative path is unknown too. The paths aren't
// cleaned: through a process's working directory, ".." leaves that
// directory, which inspect judges.
func (s *shellInspector) resolve(path string) []string {
	switch {
	case path == "~" || strings.HasPrefix(path, "~/"):
		return []string{s.home + strings.TrimPrefix(path, "~")}
	case path == "~+": // the shell's folder
		return s.resolve(".")
	case strings.HasPrefix(path, "~+/"):
		return s.resolve(strings.TrimPrefix(path, "~+/"))
	case strings.HasPrefix(path, "~"): // ~user is that user's home
		s.call.opaque = true
		return []string{path}
	case filepath.IsAbs(path):
		return []string{path}
	case s.dirs == nil:
		s.call.opaque = true
		return []string{path}
	}
	paths := make([]string, len(s.dirs))
	for i, dir := range s.dirs {
		paths[i] = dir + "/" + path
	}
	return paths
}

// inSubshell walks a subshell, whose cd ends with it.
func (s *shellInspector) inSubshell(n *syntax.Subshell) {
	dirs := slices.Clone(s.dirs)
	for _, stmt := range n.Stmts {
		syntax.Walk(stmt, s.visit)
	}
	s.dirs = dirs
}

// enterLoop: a cd in a loop moves the commands of its next round, which the
// walk has already passed, so with one the folder can't be known. A cd in a
// subshell ends with it, so it moves no later round.
func (s *shellInspector) enterLoop(loop syntax.Node) {
	syntax.Walk(loop, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Subshell:
			return false
		case *syntax.CallExpr:
			if names, _ := s.dirBuiltin(n.Args); len(names) > 0 {
				s.dirs = nil
			}
		}
		return true
	})
}
