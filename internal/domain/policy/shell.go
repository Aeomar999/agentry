package policy

import (
	"slices"
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
	named     []string // the names a path that can't be fully known still spells out: tls in $X/tls
	writes    []string // redirect targets written to: > f, >> f, &> f, >| f, <> f
	runs      []run    // each simple command, with where it starts and what it names
	opaque    bool     // part of it can't be known before it runs: $(...), an unknown variable
	recursive bool     // some program may read the folders it is given whole: any but ls, cd, stat and the like
	invalid   bool     // the shell couldn't parse it
}

// run is one simple command as a runner rule sees it: the folders it may
// start in (nil when they can't be known), and every file its arguments may
// name, options' values included, so a rule can tell whether it may load
// code from outside the workspace.
type run struct {
	texts []string // the command, once for each way its words can expand
	dirs  []string
	files []string
}

// variables Agentry knows the value of in an agent's session.
func knownVariables(workspace, home string) map[string][]string {
	return map[string][]string{"HOME": {home}, "PWD": {workspace}}
}

func inspectShell(command, workspace, home string) shellCall {
	return nestedInspector(command, workspace, home, []string{workspace}, 0).call
}

// nestedInspector inspects a command run depth levels inside others, by
// bash -c or eval, started in one of dirs (nil: unknown). It is left with
// the folders the shell may be in after the command.
func nestedInspector(command, workspace, home string, dirs []string, depth int) *shellInspector {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return &shellInspector{call: shellCall{invalid: true}}
	}
	inspector := &shellInspector{variables: knownVariables(workspace, home), workspace: workspace, home: home, dirs: dirs, depth: depth}
	if !expandBraces(file) {
		inspector.call.opaque = true
	}
	inspector.learnVariables(file)
	syntax.Walk(file, inspector.visit)
	return inspector
}

type shellInspector struct {
	call            shellCall
	variables       map[string][]string // name → the literal values it can hold
	workspace, home string
	depth           int // how many bash -c or eval this command runs inside

	// dirs are the folders the shell may be in at the command the walk has
	// reached: the workspace, where Agentry starts it, and wherever a cd or
	// pushd before it may have moved it. A cd can fail and leave the shell
	// where it was, so a cd adds folders rather than replacing them. nil when
	// they can't be known: after cd -, popd or a cd in a loop. Relative paths
	// are counted from each of them.
	dirs []string
}

func (s *shellInspector) visit(node syntax.Node) bool {
	switch n := node.(type) {
	case *syntax.CmdSubst, *syntax.ProcSubst, *syntax.FuncDecl, *syntax.CoprocClause:
		s.call.opaque = true
	case *syntax.Subshell:
		s.inSubshell(n)
		return false
	case *syntax.ForClause:
		if iteration, ok := n.Loop.(*syntax.WordIter); ok {
			s.setVariable(iteration.Name.Value, false)
		}
		s.enterLoop(n)
	case *syntax.WhileClause:
		s.enterLoop(n)
	case *syntax.CallExpr:
		s.addCommand(n.Args, n.Assigns)
	case *syntax.DeclClause:
		s.addDeclaration(n)
	case *syntax.Redirect:
		s.addRedirect(n)
	case *syntax.BinaryCmd:
		if n.Op == syntax.Pipe || n.Op == syntax.PipeAll {
			ways, complete := combinations(s.pipeline(n))
			if !complete {
				s.call.opaque = true
			}
			for _, way := range ways {
				s.call.pipelines = append(s.call.pipelines, strings.Join(way, " | "))
			}
		}
	}
	return true
}

func (s *shellInspector) addCommand(args []*syntax.Word, assigns []*syntax.Assign) {
	for _, assign := range assigns {
		if assign.Name != nil {
			s.setVariable(assign.Name.Value, len(args) > 0) // FOO=1 cmd hands FOO to cmd
		}
		if assign.Value != nil {
			s.addPath(assign.Value)
		}
	}
	if len(args) == 0 {
		return
	}
	texts := s.commandTexts(args)
	for _, text := range texts {
		if name, found := printfVariable(text); found {
			s.setVariable(name, false)
		}
		s.call.commands = append(s.call.commands, text)
		s.call.recursive = s.call.recursive || readsFoldersWhole(strings.Fields(text))
	}
	s.call.runs = append(s.call.runs, run{texts: texts, dirs: slices.Clone(s.dirs), files: s.namedFiles(args[1:])})
	if name, ok := s.values(args[0]); ok && len(name) == 1 && strings.Contains(name[0], "/") {
		s.addPath(args[0]) // ./test.sh runs a file
	}
	for _, arg := range args[1:] {
		s.addPath(arg)
	}
	s.addInnerCommands(args)
	s.followCd(args)
}

func (s *shellInspector) addDeclaration(n *syntax.DeclClause) {
	words := []string{n.Variant.Value}
	exported := s.exports(n)
	for _, assign := range n.Args {
		if assign.Name == nil { // an option: declare -x
			words = append(words, s.wordText(assign.Value))
			continue
		}
		s.setVariable(assign.Name.Value, exported)
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
	case syntax.DplIn, syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
		return // a heredoc is the command's input, not a file
	case syntax.DplOut:
		if r.Word == nil || isDescriptor(s.wordText(r.Word)) {
			return // 2>&1 joins outputs; only bash's >&file names a file
		}
	}
	if r.Word == nil {
		return
	}
	s.addWordAsPath(r.Word, true)
	if r.Op != syntax.RdrIn {
		values, ok := s.values(r.Word)
		if !ok {
			s.call.writes = append(s.call.writes, "$?") // a file it can't name is still written
			return
		}
		for _, value := range values {
			s.call.writes = append(s.call.writes, s.resolve(value)...)
		}
	}
}

// isDescriptor: the target of >& is a file descriptor (2>&1) or "-" (close).
func isDescriptor(target string) bool {
	return target == "-" || target != "" && strings.Trim(target, "0123456789") == ""
}

// pipeline is each command of a pipeline, in every way it can expand.
func (s *shellInspector) pipeline(n *syntax.BinaryCmd) [][]string {
	var parts [][]string
	for _, side := range []*syntax.Stmt{n.X, n.Y} {
		switch cmd := side.Cmd.(type) {
		case *syntax.BinaryCmd:
			if cmd.Op == syntax.Pipe || cmd.Op == syntax.PipeAll {
				parts = append(parts, s.pipeline(cmd)...)
				continue
			}
		case *syntax.CallExpr:
			if texts := s.commandTexts(cmd.Args); len(texts) > 0 {
				parts = append(parts, texts)
				continue
			}
		}
		parts = append(parts, []string{"$?"})
	}
	return parts
}
