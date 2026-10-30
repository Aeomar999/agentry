package policy

import (
	"path/filepath"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// shells run the string given after -c as a command of their own.
var shells = map[string]bool{"bash": true, "sh": true, "zsh": true, "dash": true, "ksh": true}

// maxNesting is how deep bash -c and eval may run inside one another before
// the command counts as unknown.
const maxNesting = 4

// addInnerCommands judges the commands a call hands to a shell as a string:
// bash -c 'cat x', a shell started further along (timeout 9 sh -c 'cat x'),
// and eval "cat x", also behind command, builtin or exec. The inner
// command's paths and flags count for the call; a string whose value can't
// be known makes it unknown.
func (s *shellInspector) addInnerCommands(args []*syntax.Word) {
	program := s.wrappedProgram(args)
	for i, arg := range args {
		names, ok := s.values(arg)
		if !ok {
			continue // the word already makes the call opaque
		}
		for _, name := range names {
			switch {
			case i == program && name == "eval":
				commands, known := s.evalStrings(args[i+1:])
				s.addInnerCommand(commands, known, true)
			case shells[filepath.Base(name)]:
				if script := s.shellScript(args[i+1:]); script != nil {
					commands, known := s.values(script)
					s.addInnerCommand(commands, known, false)
				}
			}
		}
	}
}

// wrappers run the command named after them and their options.
var wrappers = map[string]bool{"command": true, "builtin": true, "exec": true}

// wrappedProgram is the index of the word that names the program a command
// runs, past any wrappers and their options: 2 in command -p eval "x".
// exec's -a takes a value, alone or clustered (-la NAME).
func (s *shellInspector) wrappedProgram(args []*syntax.Word) int {
	value := func(word *syntax.Word) string { // "" when not one known value
		if values, ok := s.values(word); ok && len(values) == 1 {
			return values[0]
		}
		return ""
	}
	i := 0
	for i < len(args) && wrappers[value(args[i])] {
		wrapper := value(args[i])
		for i++; i < len(args); i++ {
			option := value(args[i])
			if option == "--" {
				i++
				break
			}
			if !strings.HasPrefix(option, "-") {
				break
			}
			if wrapper == "exec" && strings.Contains(option[1:], "a") {
				i++
			}
		}
	}
	return i
}

// shellScript is the word a shell runs as a command, when its options
// include -c (alone or clustered: -lc, -ec), or nil. -o and -O take a value.
func (s *shellInspector) shellScript(words []*syntax.Word) *syntax.Word {
	runsString := false
	for i := 0; i < len(words); i++ {
		values, ok := s.values(words[i])
		if !ok || len(values) != 1 {
			s.call.opaque = true
			return nil
		}
		option := values[0]
		switch {
		case option == "--":
			if runsString && i+1 < len(words) {
				return words[i+1]
			}
			return nil
		case strings.HasPrefix(option, "--"):
		case len(option) > 1 && (option[0] == '-' || option[0] == '+'):
			runsString = runsString || option[0] == '-' && strings.Contains(option, "c")
			if strings.ContainsAny(option[len(option)-1:], "oO") {
				i++
			}
		case runsString:
			return words[i]
		default:
			return nil // a script file and its arguments
		}
	}
	return nil
}

// evalStrings is every command eval can run: its words joined by spaces.
func (s *shellInspector) evalStrings(words []*syntax.Word) ([]string, bool) {
	choices := make([][]string, len(words))
	for i, word := range words {
		values, ok := s.values(word)
		if !ok {
			return nil, false
		}
		choices[i] = values
	}
	ways, complete := combinations(choices)
	if !complete {
		return nil, false
	}
	commands := make([]string, len(ways))
	for i, way := range ways {
		commands[i] = strings.Join(way, " ")
	}
	return commands, true
}

// addInnerCommand merges what each of the commands does into the call. Each
// starts in the folders the shell may be in; eval runs in this shell, so
// with inThisShell its cd moves the commands after it too.
func (s *shellInspector) addInnerCommand(commands []string, known, inThisShell bool) {
	if !known || s.depth >= maxNesting {
		s.call.opaque = true
		return
	}
	var dirs []string // where the shell may be after them
	dirsKnown := true
	for _, command := range commands {
		inner := nestedInspector(command, s.workspace, s.home, slices.Clone(s.dirs), s.depth+1)
		s.call.paths = append(s.call.paths, inner.call.paths...)
		s.call.named = append(s.call.named, inner.call.named...)
		s.call.recursive = s.call.recursive || inner.call.recursive
		s.call.opaque = s.call.opaque || inner.call.opaque || inner.call.invalid
		dirsKnown = dirsKnown && inner.dirs != nil
		for _, dir := range inner.dirs {
			if !slices.Contains(dirs, dir) {
				dirs = append(dirs, dir)
			}
		}
	}
	if inThisShell {
		s.dirs = nil
		if dirsKnown && len(dirs) <= maxValues {
			s.dirs = dirs
		}
	}
}
