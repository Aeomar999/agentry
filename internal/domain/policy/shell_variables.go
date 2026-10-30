package policy

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// learnVariables records variables whose every possible value is written in
// the command: NAME=literal, and the items of `for NAME in literal ...`. So
// `S=out; ls $S` and `for f in a b; do cat $f; done` can still be judged. A
// loop with no items leaves its variable unset, so it isn't recorded.
//
// A variable set any other way (read, printf -v, unset, c+=x, c=(a b),
// ${c:=x}, c=$x) would run with a value this can't see, and many of those
// ways name their variable in arguments only the builtin can read. So when
// any of them appears, no variable is known, Agentry's own included, and
// every command that uses one asks.
//
// A loop can give a word a new value after the walk has passed it, so the
// walk runs again until no variable gains a value; its last run judges every
// word against all the values it can hold. A variable whose values never
// stop growing (c=${c}x) isn't known either.
func (s *shellInspector) learnVariables(file *syntax.File) {
	for range maxLearningRounds {
		before := valueCount(s.variables)
		if s.learnOnce(file) {
			break
		}
		if valueCount(s.variables) == before {
			return
		}
	}
	s.variables = map[string][]string{}
}

// maxLearningRounds is how many times learnVariables walks the command before
// it gives up on the values settling.
const maxLearningRounds = 8

func valueCount(variables map[string][]string) int {
	count := 0
	for _, values := range variables {
		count += len(values)
	}
	return count
}

// learnOnce walks the command once, adding the values it finds, and says
// whether a variable is set in a way that can't be read.
func (s *shellInspector) learnOnce(file *syntax.File) (unsure bool) {
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Assign:
			if n.Name == nil { // an option: declare -x
				return true
			}
			if values, ok := s.assignedValues(n); ok {
				s.learn(n.Name.Value, values)
			} else {
				unsure = true
			}
		case *syntax.DeclClause:
			unsure = unsure || slices.ContainsFunc(n.Args, func(a *syntax.Assign) bool { return a.Name == nil }) // declare -n, -a, -i
		case *syntax.ForClause:
			if iteration, ok := n.Loop.(*syntax.WordIter); ok {
				items, ok := s.loopItems(iteration)
				if n.Select { // an answer not in the list leaves it empty
					items = append(items, "")
				}
				if !ok {
					unsure = true
				} else if len(items) > 0 {
					s.learn(iteration.Name.Value, items)
				}
			}
		case *syntax.ParamExp:
			unsure = unsure || n.Exp != nil && (n.Exp.Op == syntax.AssignUnset || n.Exp.Op == syntax.AssignUnsetOrNull)
		case *syntax.BinaryArithm:
			unsure = unsure || slices.Contains(arithmeticAssignments, n.Op)
		case *syntax.UnaryArithm:
			unsure = unsure || n.Op == syntax.Inc || n.Op == syntax.Dec
		case *syntax.CallExpr:
			unsure = unsure || s.setsVariables(n.Args)
		}
		return true
	})
	return unsure
}

// learn adds the values a variable doesn't have yet. It stops at maxValues+1:
// a variable with more than maxValues values is unknown in every word, so
// which values past that it holds changes no verdict, and a full list no
// longer grows, so learnVariables still sees the values settle.
func (s *shellInspector) learn(name string, values []string) {
	for _, value := range values {
		if len(s.variables[name]) > maxValues {
			return
		}
		if !slices.Contains(s.variables[name], value) {
			s.variables[name] = append(s.variables[name], value)
		}
	}
}

// arithmeticAssignments set the variable on their left: ((c=1)), let c+=1.
var arithmeticAssignments = []syntax.BinAritOperator{
	syntax.Assgn, syntax.AddAssgn, syntax.SubAssgn, syntax.MulAssgn, syntax.QuoAssgn, syntax.RemAssgn,
	syntax.AndAssgn, syntax.OrAssgn, syntax.XorAssgn, syntax.ShlAssgn, syntax.ShrAssgn,
}

// assignedValues is every value NAME=value can give its variable; an append,
// an array, an element or a bare `export NAME` can't be read here.
func (s *shellInspector) assignedValues(n *syntax.Assign) ([]string, bool) {
	switch {
	case n.Append, n.Array != nil, n.Index != nil, n.Naked:
		return nil, false
	case n.Value == nil: // c=
		return []string{""}, true
	}
	return s.values(n.Value)
}

// loopItems is every value a for loop gives its variable. Without "in" it
// loops over the script's arguments, which can't be read here.
func (s *shellInspector) loopItems(iteration *syntax.WordIter) ([]string, bool) {
	if !iteration.InPos.IsValid() {
		return nil, false
	}
	var items []string
	for _, word := range iteration.Items {
		values, ok := s.values(word)
		if !ok {
			return nil, false
		}
		items = append(items, values...)
	}
	return items, true
}

// variableSetters are the builtins that set or unset a variable named in
// their arguments.
var variableSetters = map[string]bool{
	"read": true, "mapfile": true, "readarray": true, "getopts": true, "unset": true, "let": true,
	"declare": true, "typeset": true, "local": true, "export": true, "readonly": true,
	"eval": true, "source": true, ".": true,
}

// setsVariables: the command may set a variable, or its name can't be read.
func (s *shellInspector) setsVariables(args []*syntax.Word) bool {
	if len(args) == 0 {
		return false
	}
	names, ok := s.values(args[0])
	if !ok {
		return true
	}
	for _, name := range names {
		if variableSetters[name] {
			return true
		}
		if (name == "builtin" || name == "command") && s.setsVariables(s.withoutOption(args[1:], "-p")) {
			return true // they run the builtin after them; command -v only looks it up
		}
		if name == "printf" && slices.ContainsFunc(args[1:], s.mayBeVariableOption) {
			return true
		}
		if name == "wait" && slices.ContainsFunc(args[1:], s.mayBeWaitVariableOption) {
			return true
		}
	}
	return false
}

// withoutOption is the words after any leading copies of option.
func (s *shellInspector) withoutOption(words []*syntax.Word, option string) []*syntax.Word {
	for len(words) > 0 && s.isText(words[0], option) {
		words = words[1:]
	}
	return words
}

// isText: the word can only be text, as the shell reads it (-\p is -p).
func (s *shellInspector) isText(word *syntax.Word, text string) bool {
	values, ok := s.values(word)
	return ok && len(values) == 1 && values[0] == text
}

// mayBeVariableOption: the word may be printf's -v NAME or -vNAME.
func (s *shellInspector) mayBeVariableOption(word *syntax.Word) bool {
	values, ok := s.values(word)
	return !ok || slices.ContainsFunc(values, func(value string) bool { return strings.HasPrefix(value, "-v") })
}

// mayBeWaitVariableOption: the word may be wait's -p NAME, alone or in a
// cluster of its options (-np NAME).
func (s *shellInspector) mayBeWaitVariableOption(word *syntax.Word) bool {
	values, ok := s.values(word)
	return !ok || slices.ContainsFunc(values, func(value string) bool {
		return strings.HasPrefix(value, "-") && strings.Contains(value, "p")
	})
}

// Variables a command may be given without being judged differently. The
// programs agents run read their settings from the environment, and some of
// those run a command or write a file: GIT_EXTERNAL_DIFF, GIT_CONFIG_*,
// GOFLAGS=-toolexec, PATH, PAGER, LD_PRELOAD, npm's npm_config_script_shell
// (npm takes npm_config_* in any case), and any variable make is handed, which
// a Makefile may use. So setting an upper-case or npm_config_ variable, or
// handing a lower-case one to a program (x=1 make test, export x), makes the
// call unknown until it runs, as git -c does. These only choose a toolchain, a
// platform or how output looks. A lower-case variable the shell keeps to
// itself, as in `for f in a b`, is the command's own.
var harmlessVariables = map[string]bool{
	"GOTOOLCHAIN": true, "GOPROXY": true, "CGO_ENABLED": true, "GOOS": true, "GOARCH": true,
	"NO_COLOR": true, "LC_ALL": true, "LANG": true, "TZ": true,
}

// setVariable judges a variable the command sets: before a command
// (FOO=1 cmd), on its own, with export or declare, by a for loop or by
// printf -v. An upper-case one may already be exported, so any of them can
// reach the programs it runs; exported says this one is handed to them.
func (s *shellInspector) setVariable(name string, exported bool) {
	lower := strings.ToLower(name)
	switch {
	case harmlessVariables[name]:
	case lower != name, strings.HasPrefix(lower, "npm_config_"), exported:
		s.call.opaque = true
	}
}

// exports: the declaration hands its variables to the programs the command
// runs, as export and declare -x do.
func (s *shellInspector) exports(n *syntax.DeclClause) bool {
	if n.Variant.Value == "export" {
		return true
	}
	for _, assign := range n.Args {
		if assign.Name != nil {
			continue
		}
		values, ok := s.values(assign.Value)
		if !ok {
			return true
		}
		for _, option := range values {
			if strings.HasPrefix(option, "-") && strings.Contains(option, "x") {
				return true
			}
		}
	}
	return false
}

// printfVariable is the variable printf -v sets, if it is given one.
func printfVariable(command string) (string, bool) {
	words := strings.Fields(command)
	for i := 1; i < len(words) && words[0] == "printf"; i++ {
		switch {
		case words[i] == "-v" && i+1 < len(words):
			return words[i+1], true
		case strings.HasPrefix(words[i], "-v"):
			return strings.TrimPrefix(words[i], "-v"), true
		}
	}
	return "", false
}
