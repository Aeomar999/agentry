package policy

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

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
// `git grep -nO'cmd'` opens its files with cmd, `gh run view -vw` opens a
// browser, and `pytest -vo cache_dir=x` sets where pytest writes. Their options are split apart so a rule for -o
// sees it. Only these: find's -name and go's -run are single options, not
// clusters.
var clusteringTools = map[string]bool{
	"sort": true, "tree": true, "file": true, "git grep": true, "gh run view": true, "npx jest": true, "npx vitest": true, "npx eslint": true,
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
		name, _, joined := strings.Cut(word, "=")
		switch {
		case gitOptions[word]:
			i++
			continue
		case gitOptionsWithValue[word] && i+1 < len(words):
			name = word
			i += 2
		case joined && strings.HasPrefix(name, "--") && (gitOptionsWithValue[name] || name == "--exec-path"):
			i++
		default:
			return append([]string{"git"}, words[i:]...)
		}
		if name == "-c" || name == "--config-env" || name == "--exec-path" {
			s.call.opaque = true
		}
	}
	return []string{"git"}
}
