package policy

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
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
	writes    []string // redirect targets written to: > f, >> f, &> f, >| f, <> f
	runs      []run    // each simple command, with where it starts and what it names
	opaque    bool     // part of it can't be known before it runs: $(...), an unknown variable
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
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return shellCall{invalid: true}
	}
	inspector := &shellInspector{variables: knownVariables(workspace, home), home: home, dirs: []string{workspace}}
	if !expandBraces(file) {
		inspector.call.opaque = true
	}
	inspector.learnVariables(file)
	syntax.Walk(file, inspector.visit)
	return inspector.call
}

type shellInspector struct {
	call      shellCall
	variables map[string][]string // name → the literal values it can hold
	home      string

	// dirs are the folders the shell may be in at the command the walk has
	// reached: the workspace, where Agentry starts it, and wherever a cd or
	// pushd before it may have moved it. A cd can fail and leave the shell
	// where it was, so a cd adds folders rather than replacing them. nil when
	// they can't be known: after cd -, popd or a cd in a loop. Relative paths
	// are counted from each of them.
	dirs []string
}

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
	}
	s.call.runs = append(s.call.runs, run{texts: texts, dirs: slices.Clone(s.dirs), files: s.namedFiles(args[1:])})
	if name, ok := s.values(args[0]); ok && len(name) == 1 && strings.Contains(name[0], "/") {
		s.addPath(args[0]) // ./test.sh runs a file
	}
	for _, arg := range args[1:] {
		s.addPath(arg)
	}
	s.followCd(args)
}

// namedFiles is every file the arguments may name: each argument, an
// option's value after "=" (--config=x) or joined to a short option (-cx),
// and each item of a list or a JSON value in them (--reporters=a,b,
// --transform='{"x":"t.js"}'). Most are no file at all; a word that isn't
// one is counted from the shell's folder and stays inside the workspace.
func (s *shellInspector) namedFiles(args []*syntax.Word) []string {
	var files []string
	for _, arg := range args {
		fields, _ := s.fields(arg) // a word that can't be known makes the call opaque elsewhere
		for _, value := range slices.Concat(fields...) {
			texts, ok := decoded(optionValue(value))
			if !ok {
				files = append(files, value) // not a path in the workspace, so it counts as outside
				continue
			}
			for _, text := range texts {
				for _, item := range strings.FieldsFunc(text, isListSeparator) {
					files = append(files, s.resolve(item)...)
				}
			}
		}
	}
	return files
}

// decoded is the strings a value holds, read as JSON when it starts like JSON,
// since jest decodes such values itself ("\/tmp" and "/tmp" are /tmp). A
// value that starts like JSON but doesn't decode can't be read: not ok.
func decoded(value string) ([]string, bool) {
	if !strings.HasPrefix(value, "{") && !strings.HasPrefix(value, "[") {
		return []string{value}, true
	}
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil, false
	}
	return jsonStrings(parsed), true
}

// jsonStrings is every string in a decoded JSON value, keys included.
func jsonStrings(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []any:
		var texts []string
		for _, item := range v {
			texts = append(texts, jsonStrings(item)...)
		}
		return texts
	case map[string]any:
		var texts []string
		for key, item := range v {
			texts = append(texts, key)
			texts = append(texts, jsonStrings(item)...)
		}
		return texts
	}
	return nil
}

// optionValue is what an option gives as its value: --x=v and -xv give v. A
// word that isn't an option is its own value.
func optionValue(word string) string {
	if !strings.HasPrefix(word, "-") {
		return word
	}
	if _, value, found := strings.Cut(word, "="); found {
		return value
	}
	if !strings.HasPrefix(word, "--") && len(word) > 2 {
		return word[2:]
	}
	return ""
}

// isListSeparator splits a value into what it may name. = splits a nested
// option, as in --execArgv=--require=f.
func isListSeparator(r rune) bool { return strings.ContainsRune(`,:;"'{}[]= `, r) }

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
		for _, dir := range s.resolve(target) {
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

// resolve is every file a path may name from the folders the shell may be
// in. With them unknown, a relative path is unknown too.
func (s *shellInspector) resolve(path string) []string {
	switch {
	case path == "~" || strings.HasPrefix(path, "~/"):
		return []string{s.home + strings.TrimPrefix(path, "~")}
	case strings.HasPrefix(path, "~"): // ~user is that user's home
		s.call.opaque = true
		return []string{path}
	case filepath.IsAbs(path):
		return []string{filepath.Clean(path)}
	case s.dirs == nil:
		s.call.opaque = true
		return []string{path}
	}
	paths := make([]string, len(s.dirs))
	for i, dir := range s.dirs {
		paths[i] = filepath.Join(dir, path)
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

// isDescriptor: the target of >& is a file descriptor (2>&1) or "-" (close).
func isDescriptor(target string) bool {
	return target == "-" || target != "" && strings.Trim(target, "0123456789") == ""
}

// addPath records a command argument that may name a file. Flags, URLs and
// empty words don't; a word whose value can't be known makes the call opaque.
func (s *shellInspector) addPath(word *syntax.Word) { s.addWordAsPath(word, false) }

func (s *shellInspector) addWordAsPath(word *syntax.Word, isTarget bool) {
	values, ok := s.values(word)
	if !isTarget { // a redirect target isn't split: the shell refuses one that would be
		values = nil
		fields, known := s.fields(word)
		for _, words := range fields {
			values = append(values, words...)
		}
		ok = known
	}
	if !ok {
		s.call.opaque = true
		return
	}
	for _, value := range values {
		if value == "" || strings.Contains(value, "://") || (!isTarget && strings.HasPrefix(value, "-")) {
			continue
		}
		s.call.paths = append(s.call.paths, s.resolve(value)...)
	}
}

// commandTexts is a command as the rules match it, once for each way its
// words can expand: `for c in status commit; do git $c; done` runs both
// `git status` and `git commit`, and each is judged, so the strictest rule
// wins. Past maxValues ways the call is opaque, and only the first ways are
// judged; a word that can't be known is "$?", and opaque too.
func (s *shellInspector) commandTexts(args []*syntax.Word) []string {
	choices := make([][][]string, len(args))
	for i, arg := range args {
		fields, ok := s.fields(arg)
		if !ok {
			s.call.opaque = true
			fields = [][]string{{"$?"}}
		}
		choices[i] = fields
	}
	ways, complete := combinations(choices)
	if !complete {
		s.call.opaque = true
	}
	var texts []string
	for _, way := range ways {
		words := slices.Concat(way...)
		if len(words) == 0 {
			continue // every word expanded to nothing: no command runs
		}
		if words[0] == "git" {
			words = s.withoutGitOptions(words)
		}
		texts = append(texts, strings.Join(withShortOptionsApart(words), " "))
	}
	return texts
}

// combinations is every way to take one item from each choice, in order. It
// stops growing at maxValues ways: past that, complete is false and later
// choices give only their first item.
func combinations[T any](choices [][]T) (ways [][]T, complete bool) {
	ways, complete = [][]T{nil}, true
	for _, choice := range choices {
		if len(ways)*len(choice) > maxValues {
			choice, complete = choice[:1], false
		}
		next := make([][]T, 0, len(ways)*len(choice))
		for _, way := range ways {
			for _, item := range choice {
				next = append(next, append(way[:len(way):len(way)], item))
			}
		}
		ways = next
	}
	return ways, complete
}

// Tools whose short options cluster, getopt style, and that have one which
// writes a file or runs a program: `sort -uo out` is `sort -u -o out`,
// `git grep -nO'cmd'` opens its files with cmd, and `pytest -vo cache_dir=x`
// sets where pytest writes. Their options are split apart so a rule for -o
// sees it. Only these: find's -name and go's -run are single options, not
// clusters.
var clusteringTools = map[string]bool{
	"sort": true, "tree": true, "file": true, "git grep": true, "npx jest": true, "npx vitest": true, "npx eslint": true,
	"pytest": true, "python -m pytest": true, "python3 -m pytest": true,
}

// withShortOptionsApart writes each cluster of a clustering tool's short
// options as one option per letter, up to "--". A value joined to its option
// (-oout) splits too; the letters it leaves only matter to a rule naming them.
func withShortOptionsApart(words []string) []string {
	start := 0
	for n := 1; n <= min(len(words), 3); n++ {
		if clusteringTools[strings.Join(words[:n], " ")] {
			start = n
		}
	}
	if start == 0 {
		return words
	}
	apart := slices.Clone(words[:start])
	for i, word := range words[start:] {
		if word == "--" {
			return append(apart, words[start+i:]...)
		}
		if len(word) < 3 || word[0] != '-' || word[1] == '-' {
			apart = append(apart, word)
			continue
		}
		for _, letter := range word[1:] {
			apart = append(apart, "-"+string(letter))
		}
	}
	return apart
}

// git's options before its subcommand that take a value: -C dir, -c k=v,
// --git-dir dir or --git-dir=dir, and the like.
var gitOptionsWithValue = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true, "--config-env": true, "--attr-source": true,
}

var gitOptions = map[string]bool{
	"-p": true, "--paginate": true, "-P": true, "--no-pager": true, "--bare": true, "--no-replace-objects": true,
	"--no-lazy-fetch": true, "--no-optional-locks": true, "--no-advice": true, "--literal-pathspecs": true,
	"--glob-pathspecs": true, "--noglob-pathspecs": true, "--icase-pathspecs": true,
}

// withoutGitOptions drops git's options before its subcommand, so one
// pattern, 'git commit *', matches `git -C dir commit` and `git --git-dir=x
// commit` too. A directory they name stays a path, judged by `where`. A
// setting (-c, --config-env, --exec-path=) can make git run a command of its
// own, core.pager or core.fsmonitor, so it makes the call opaque. An option
// it doesn't know stops the dropping: what is left then matches no git
// pattern, so it is asked about rather than guessed.
func (s *shellInspector) withoutGitOptions(words []string) []string {
	for i := 1; i < len(words); {
		word := words[i]
		name, value, joined := strings.Cut(word, "=")
		switch {
		case gitOptions[word]:
			i++
			continue
		case gitOptionsWithValue[word] && i+1 < len(words):
			name, value, joined = word, words[i+1], false
			i += 2
		case joined && strings.HasPrefix(name, "--") && (gitOptionsWithValue[name] || name == "--exec-path"):
			i++
		default:
			return append([]string{"git"}, words[i:]...)
		}
		if name == "-c" || name == "--config-env" || name == "--exec-path" {
			s.call.opaque = true
		}
		if joined && (name == "--git-dir" || name == "--work-tree") {
			s.call.paths = append(s.call.paths, s.resolve(value)...) // addPath skips the whole word: it starts with "-"
		}
	}
	return []string{"git"}
}

// wordText is a word that must have one value, as a declaration's options
// and a redirect's descriptor are matched: "$?", and the call opaque, when it
// can't be known or could be one of several.
func (s *shellInspector) wordText(word *syntax.Word) string {
	values, ok := s.values(word)
	if !ok || len(values) > 1 {
		s.call.opaque = true
		return "$?"
	}
	if len(values) == 0 {
		return ""
	}
	return values[0]
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

// maxValues is how many values a word, or ways a command, may have before it
// counts as unknown.
const maxValues = 64

// fields is every list of words a shell word can become: each of its values,
// split at spaces when the word holds an unquoted $variable, as the shell
// splits it (c='push -f'; git $c is git push -f).
func (s *shellInspector) fields(word *syntax.Word) ([][]string, bool) {
	values, ok := s.values(word)
	if !ok {
		return nil, false
	}
	splits := slices.ContainsFunc(word.Parts, func(part syntax.WordPart) bool {
		_, isVariable := part.(*syntax.ParamExp)
		return isVariable
	})
	fields := make([][]string, len(values))
	for i, value := range values {
		if splits {
			fields[i] = strings.Fields(value)
		} else {
			fields[i] = []string{value}
		}
	}
	return fields, true
}

// values is every string a word can expand to, when that can be known without
// running anything: literals, quotes, and variables learned from the command.
func (s *shellInspector) values(word *syntax.Word) ([]string, bool) {
	return s.partsValues(word.Parts, false)
}

func (s *shellInspector) partsValues(parts []syntax.WordPart, quoted bool) ([]string, bool) {
	if joinsDollar(parts) {
		return nil, false
	}
	results := []string{""}
	for _, part := range parts {
		pieces, ok := s.partValues(part, quoted)
		if !ok || len(results)*len(pieces) > maxValues {
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

// joinsDollar: a $ is followed by more of the word, which only a backslash and
// a newline between them make. mvdan drops those and keeps the $ apart, but
// bash joins the lines first, so $\<newline>x is $x.
func joinsDollar(parts []syntax.WordPart) bool {
	for _, part := range parts[:max(len(parts)-1, 0)] {
		if lit, isLit := part.(*syntax.Lit); isLit && lit.Value == "$" {
			return true
		}
	}
	return false
}

// partValues is every string one part of a word can be. Inside double quotes
// a literal loses fewer backslashes than outside, so the parts of a quoted
// string are read with quoted set. Glob characters (* ? [) are judged as the
// text they are: a relative glob can't reach .. (bash 5.2 skips dot files'
// . and .. by default), and an absolute one starts with / and so is outside.
func (s *shellInspector) partValues(part syntax.WordPart, quoted bool) ([]string, bool) {
	switch p := part.(type) {
	case *syntax.Lit:
		return []string{unescaped(p.Value, quoted)}, true
	case *syntax.SglQuoted:
		if p.Dollar && strings.Contains(p.Value, `\`) { // $'\x41' is A
			return nil, false
		}
		return []string{p.Value}, true
	case *syntax.DblQuoted:
		return s.partsValues(p.Parts, true)
	case *syntax.ParamExp:
		isPlain := p.Exp == nil && p.Repl == nil && p.Index == nil && p.Slice == nil && !p.Length && !p.Excl && !p.Width && p.Names == 0
		if values, known := s.variables[p.Param.Value]; isPlain && known && len(values) > 0 {
			return values, true
		}
	}
	return nil, false
}

// unescaped is a literal as the shell hands it on. Outside quotes a backslash
// keeps the character after it as text (\/tmp is /tmp, --exec\-argv is
// --exec-argv); inside double quotes only before $ ` " \ or a newline, and
// stays otherwise ("a\x" is a\x). A backslash and a newline are nothing.
func unescaped(text string, quoted bool) string {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' || i+1 == len(text) || quoted && !strings.ContainsRune("$`\"\\\n", rune(text[i+1])) {
			b.WriteByte(text[i])
			continue
		}
		i++
		if text[i] != '\n' {
			b.WriteByte(text[i])
		}
	}
	return b.String()
}

// expandBraces replaces each word of a call or loop list with the words brace
// expansion makes of it, before anything else reads them: go test ./{a,b}/ is
// go test ./a/ ./b/. It rewrites the parsed file in place. A word it can't
// expand, and any brace that would expand in a declaration or a redirect,
// leaves the call unknown: not ok (declare x={a,} sets x to "", while x={a,}
// sets {a,}).
func expandBraces(file *syntax.File) (ok bool) {
	ok = true
	expandAll := func(words []*syntax.Word) []*syntax.Word {
		var all []*syntax.Word
		for _, word := range words {
			expanded, known := braceWords(word)
			if !known {
				ok = false
				expanded = []*syntax.Word{word}
			}
			all = append(all, expanded...)
		}
		return all
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			n.Args = expandAll(n.Args)
		case *syntax.WordIter:
			n.Items = expandAll(n.Items)
		case *syntax.DeclClause:
			for _, assign := range n.Args {
				if assign.Value != nil && !unbraced(assign.Value) {
					ok = false
				}
			}
		case *syntax.Redirect:
			if n.Word != nil && n.Hdoc == nil && n.Op != syntax.WordHdoc && !unbraced(n.Word) {
				ok = false
			}
		}
		return true
	})
	return ok
}

// unbraced: brace expansion leaves the word as it is.
func unbraced(word *syntax.Word) bool {
	words, ok := braceWords(word)
	return ok && len(words) == 1 && words[0] == word
}

// braceAtom is one piece of a word for brace expansion: a part that isn't a
// literal, or one character of a literal's text. brace marks a { , or } the
// shell reads as brace syntax: not quoted and not after a backslash.
type braceAtom struct {
	part  syntax.WordPart
	text  string
	brace bool
}

// braceWords is the words brace expansion makes of word: {a,b}c is ac bc,
// x{a,} is just xa, x{1..3} is x1 x2 x3. Not ok for a sequence this can't
// read ({1..a}), for more than maxValues words, and next to a $name or a $
// that joins the next line, which bash reads again after expanding, so that
// $x{y,} is $xy $x.
func braceWords(word *syntax.Word) ([]*syntax.Word, bool) {
	atoms := braceAtoms(word)
	if !slices.ContainsFunc(atoms, func(a braceAtom) bool { return a.brace && a.text == "{" }) {
		return []*syntax.Word{word}, true
	}
	ways, ok := expandAtoms(atoms)
	if !ok || len(ways) > maxValues {
		return nil, false
	}
	if len(ways) == 1 && len(ways[0]) == len(atoms) { // nothing expanded; {5..5} did
		return []*syntax.Word{word}, true
	}
	if joinsDollar(word.Parts) || slices.ContainsFunc(word.Parts, func(part syntax.WordPart) bool {
		p, isVariable := part.(*syntax.ParamExp)
		return isVariable && p.Short
	}) {
		return nil, false
	}
	var words []*syntax.Word
	for _, way := range ways {
		if len(way) > 0 { // an empty one is no word at all
			words = append(words, wordOf(way))
		}
	}
	return words, true
}

func braceAtoms(word *syntax.Word) []braceAtom {
	var atoms []braceAtom
	for _, part := range word.Parts {
		lit, isLit := part.(*syntax.Lit)
		if !isLit {
			atoms = append(atoms, braceAtom{part: part})
			continue
		}
		for i := 0; i < len(lit.Value); i++ {
			if lit.Value[i] == '\\' && i+1 < len(lit.Value) {
				atoms = append(atoms, braceAtom{text: lit.Value[i : i+2]}) // kept for values to unescape
				i++
				continue
			}
			char := lit.Value[i : i+1]
			atoms = append(atoms, braceAtom{text: char, brace: strings.Contains("{,}", char)})
		}
	}
	return atoms
}

// expandAtoms expands the first {…} holding a comma at its own level, or a
// sequence, and the rest after it, as bash does. A { that doesn't close stays
// text, but a brace after it may still expand ({x{a,b}} is {xa} {xb}).
func expandAtoms(atoms []braceAtom) ([][]braceAtom, bool) {
	for open, a := range atoms {
		if !a.brace || a.text != "{" || open == 0 && isBrace(atoms, 1, "}") { // a word's first {} is text, as in find -exec
			continue
		}
		end, commas := closingBrace(atoms, open)
		if end < 0 {
			continue
		}
		choices, ok := braceChoices(atoms, open, end, commas)
		if !ok {
			return nil, false
		}
		rest, ok := expandAtoms(atoms[end+1:])
		if !ok || len(choices)*len(rest) > maxValues {
			return nil, false
		}
		var ways [][]braceAtom
		for _, choice := range choices {
			for _, after := range rest {
				ways = append(ways, slices.Concat(atoms[:open], choice, after))
			}
		}
		return ways, true
	}
	return [][]braceAtom{atoms}, true
}

// braceChoices is what one {…} expands to: each of its comma-separated parts
// expanded, or a sequence's terms. Not ok for a sequence this can't read.
func braceChoices(atoms []braceAtom, open, end int, commas []int) ([][]braceAtom, bool) {
	if len(commas) == 0 {
		return sequence(atoms[open+1 : end])
	}
	var choices [][]braceAtom
	start := open
	for _, stop := range append(commas, end) {
		parts, ok := expandAtoms(atoms[start+1 : stop])
		if !ok || len(choices)+len(parts) > maxValues {
			return nil, false
		}
		choices = append(choices, parts...)
		start = stop
	}
	return choices, true
}

// sequence is the terms of a brace sequence as bash makes them: {1..3} is
// 1 2 3, {3..1} is 3 2 1, {1..10..3} is 1 4 7 10, {01..03} is 01 02 03, and
// {a..c} is a b c. Not ok for any other {x..y}: a sign, a negative number
// padded with zeros, letters of mixed case, or a step that isn't a number.
func sequence(inner []braceAtom) ([][]braceAtom, bool) {
	for _, a := range inner {
		if a.part != nil || a.brace || len(a.text) != 1 {
			return nil, false
		}
	}
	ends := strings.Split(atomsText(inner), "..")
	if len(ends) != 2 && len(ends) != 3 {
		return nil, false
	}
	step := 1
	if len(ends) == 3 {
		var ok bool
		if step, ok = sequenceNumber(ends[2]); !ok {
			return nil, false
		}
		step = max(step, -step, 1)
	}
	first, last, format, ok := sequenceEnds(ends[0], ends[1])
	if !ok || (max(first, last)-min(first, last))/step+1 > maxValues {
		return nil, false
	}
	if last < first {
		step = -step
	}
	var terms [][]braceAtom
	for n := first; step > 0 && n <= last || step < 0 && n >= last; n += step {
		term := format(n)
		atoms := make([]braceAtom, len(term))
		for i := range term {
			atoms[i] = braceAtom{text: term[i : i+1]}
		}
		terms = append(terms, atoms)
	}
	return terms, true
}

// sequenceEnds reads a sequence's two ends, both numbers or both letters of
// one case, and how each term is written.
func sequenceEnds(from, to string) (first, last int, format func(int) string, ok bool) {
	isLetter := func(s string) bool { return len(s) == 1 && ('a' <= s[0] && s[0] <= 'z' || 'A' <= s[0] && s[0] <= 'Z') }
	if isLetter(from) && isLetter(to) {
		sameCase := ('a' <= from[0]) == ('a' <= to[0])
		return int(from[0]), int(to[0]), func(n int) string { return string(rune(n)) }, sameCase
	}
	first, okFirst := sequenceNumber(from)
	last, okLast := sequenceNumber(to)
	if !okFirst || !okLast {
		return 0, 0, nil, false
	}
	padded := func(s string) bool {
		digits := strings.TrimPrefix(s, "-")
		return len(digits) > 1 && digits[0] == '0'
	}
	if !padded(from) && !padded(to) {
		return first, last, strconv.Itoa, true
	}
	if first < 0 || last < 0 {
		return 0, 0, nil, false
	}
	width := max(len(from), len(to))
	return first, last, func(n int) string { return fmt.Sprintf("%0*d", width, n) }, true
}

// sequenceNumber reads a whole number of up to nine digits, maybe after a -.
func sequenceNumber(s string) (int, bool) {
	digits := strings.TrimPrefix(s, "-")
	if digits == "" || len(digits) > 9 || strings.Trim(digits, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// closingBrace is where the { at open closes, -1 when it doesn't, and the
// commas at its own level. As in bash, a } before any comma, which isn't the
// end of something like a sequence, leaves the { open: {a}b,c} is a}b c.
func closingBrace(atoms []braceAtom, open int) (end int, commas []int) {
	depth, dots := 0, false
	for i := open; i < len(atoms); i++ {
		switch {
		case !atoms[i].brace:
			dots = dots || atoms[i].text == "." && atoms[i-1].text == "."
		case atoms[i].text == "{":
			depth++
		case atoms[i].text == "}":
			depth--
			if depth > 0 {
				continue
			}
			if len(commas) > 0 || dots {
				return i, commas
			}
			depth = 1
		case depth == 1:
			commas = append(commas, i)
		}
	}
	return -1, nil
}

func isBrace(atoms []braceAtom, i int, brace string) bool {
	return i < len(atoms) && atoms[i].brace && atoms[i].text == brace
}

func atomsText(atoms []braceAtom) string {
	var b strings.Builder
	for _, a := range atoms {
		b.WriteString(a.text)
	}
	return b.String()
}

// wordOf joins atoms back into a word, each run of text into one literal.
func wordOf(atoms []braceAtom) *syntax.Word {
	word := &syntax.Word{}
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			word.Parts = append(word.Parts, &syntax.Lit{Value: text.String()})
			text.Reset()
		}
	}
	for _, a := range atoms {
		if a.part != nil {
			flush()
			word.Parts = append(word.Parts, a.part)
			continue
		}
		text.WriteString(a.text)
	}
	flush()
	return word
}
