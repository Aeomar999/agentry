package policy

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// shellCall is what a shell command does, as the rules see it. It comes from
// parsing the command with a real shell parser, so quotes, heredocs and
// redirects are read the way the shell reads them (T-035 found the regexes
// before it mistaking a commit message for a redirect).
type shellCall struct {
	commands  []string // each simple command, its words joined by single spaces
	pipelines []string // each pipeline of two or more commands, as "a | b"
	paths     []string // every word that may name a file, and every redirect target
	opaque    bool     // part of it can't be known before it runs: $(...), an unknown variable
	invalid   bool     // the shell couldn't parse it
}

// variables Agentry knows the value of in an agent's session.
func knownVariables(workspace, home string) map[string][]string {
	return map[string][]string{"HOME": {home}, "PWD": {workspace}}
}

func inspectShell(command, workspace, home string) shellCall {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return shellCall{invalid: true}
	}
	inspector := &shellInspector{variables: knownVariables(workspace, home)}
	inspector.learnVariables(file)
	syntax.Walk(file, inspector.visit)
	return inspector.call
}

type shellInspector struct {
	call      shellCall
	variables map[string][]string // name → the literal values it can hold
}

// learnVariables records variables whose every possible value is written in
// the command: NAME=literal, and the items of `for NAME in literal ...`. So
// `S=out; ls $S` and `for f in a b; do cat $f; done` can still be judged.
func (s *shellInspector) learnVariables(file *syntax.File) {
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Assign:
			if n.Value != nil && !n.Append && n.Index == nil {
				if values, ok := s.values(n.Value); ok {
					s.variables[n.Name.Value] = append(s.variables[n.Name.Value], values...)
				}
			}
		case *syntax.ForClause:
			if iteration, ok := n.Loop.(*syntax.WordIter); ok {
				var items []string
				for _, word := range iteration.Items {
					values, ok := s.values(word)
					if !ok {
						return true
					}
					items = append(items, values...)
				}
				s.variables[iteration.Name.Value] = append(s.variables[iteration.Name.Value], items...)
			}
		}
		return true
	})
}

func (s *shellInspector) visit(node syntax.Node) bool {
	switch n := node.(type) {
	case *syntax.CmdSubst, *syntax.ProcSubst, *syntax.FuncDecl, *syntax.CoprocClause:
		s.call.opaque = true
	case *syntax.CallExpr:
		s.addCommand(n.Args, n.Assigns)
	case *syntax.DeclClause:
		s.addDeclaration(n)
	case *syntax.Redirect:
		s.addRedirect(n)
	case *syntax.BinaryCmd:
		if n.Op == syntax.Pipe || n.Op == syntax.PipeAll {
			s.call.pipelines = append(s.call.pipelines, strings.Join(s.pipeline(n), " | "))
		}
	}
	return true
}

func (s *shellInspector) addCommand(args []*syntax.Word, assigns []*syntax.Assign) {
	for _, assign := range assigns {
		if assign.Value != nil {
			s.addPath(assign.Value)
		}
	}
	if len(args) == 0 {
		return
	}
	s.call.commands = append(s.call.commands, s.commandText(args))
	if name, ok := s.values(args[0]); ok && len(name) == 1 && strings.Contains(name[0], "/") {
		s.addPath(args[0]) // ./test.sh runs a file
	}
	for _, arg := range args[1:] {
		s.addPath(arg)
	}
}

func (s *shellInspector) addDeclaration(n *syntax.DeclClause) {
	words := []string{n.Variant.Value}
	for _, assign := range n.Args {
		text := assign.Name.Value
		if assign.Value != nil {
			value := s.wordText(assign.Value)
			text += "=" + value
			s.addPath(assign.Value)
		}
		words = append(words, text)
	}
	s.call.commands = append(s.call.commands, strings.Join(words, " "))
}

func (s *shellInspector) addRedirect(r *syntax.Redirect) {
	switch r.Op {
	case syntax.DplIn, syntax.DplOut, syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
		return // 2>&1 joins outputs; a heredoc is the command's input, not a file
	}
	if r.Word != nil {
		s.addWordAsPath(r.Word, true)
	}
}

// addPath records a command argument that may name a file. Flags, URLs and
// empty words don't; a word whose value can't be known makes the call opaque.
func (s *shellInspector) addPath(word *syntax.Word) { s.addWordAsPath(word, false) }

func (s *shellInspector) addWordAsPath(word *syntax.Word, isTarget bool) {
	values, ok := s.values(word)
	if !ok {
		s.call.opaque = true
		return
	}
	for _, value := range values {
		if value == "" || strings.Contains(value, "://") || (!isTarget && strings.HasPrefix(value, "-")) {
			continue
		}
		s.call.paths = append(s.call.paths, value)
	}
}

func (s *shellInspector) commandText(args []*syntax.Word) string {
	words := make([]string, len(args))
	for i, arg := range args {
		words[i] = s.wordText(arg)
	}
	return strings.Join(words, " ")
}

// wordText is a word as the rules match it: its value, or its first value
// when a variable could hold several, or "$?" when it can't be known.
func (s *shellInspector) wordText(word *syntax.Word) string {
	values, ok := s.values(word)
	if !ok {
		s.call.opaque = true
		return "$?"
	}
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (s *shellInspector) pipeline(n *syntax.BinaryCmd) []string {
	var parts []string
	for _, side := range []*syntax.Stmt{n.X, n.Y} {
		switch cmd := side.Cmd.(type) {
		case *syntax.BinaryCmd:
			if cmd.Op == syntax.Pipe || cmd.Op == syntax.PipeAll {
				parts = append(parts, s.pipeline(cmd)...)
				continue
			}
		case *syntax.CallExpr:
			if len(cmd.Args) > 0 {
				parts = append(parts, s.commandText(cmd.Args))
				continue
			}
		}
		parts = append(parts, "$?")
	}
	return parts
}

// values is every string a word can expand to, when that can be known without
// running anything: literals, quotes, and variables learned from the command.
func (s *shellInspector) values(word *syntax.Word) ([]string, bool) {
	results := []string{""}
	for _, part := range word.Parts {
		pieces, ok := s.partValues(part)
		if !ok || len(results)*len(pieces) > 64 {
			return nil, false
		}
		var next []string
		for _, prefix := range results {
			for _, piece := range pieces {
				next = append(next, prefix+piece)
			}
		}
		results = next
	}
	return results, true
}

func (s *shellInspector) partValues(part syntax.WordPart) ([]string, bool) {
	switch p := part.(type) {
	case *syntax.Lit:
		return []string{p.Value}, true
	case *syntax.SglQuoted:
		return []string{p.Value}, true
	case *syntax.DblQuoted:
		return s.values(&syntax.Word{Parts: p.Parts})
	case *syntax.ParamExp:
		isPlain := p.Exp == nil && p.Repl == nil && p.Index == nil && p.Slice == nil && !p.Length && !p.Excl && !p.Width && p.Names == 0
		if values, known := s.variables[p.Param.Value]; isPlain && known {
			return values, true
		}
	}
	return nil, false
}
