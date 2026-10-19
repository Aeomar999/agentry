package policy

import (
	"regexp"
	"strings"
)

// A glob in policy.toml. Two kinds, because commands and paths read differently:
//
//	commands  `*` matches anything, spaces included: 'git push *'. A trailing
//	          " *" may match nothing, so 'git push *' also matches 'git push'.
//	          `#` matches a number: 'sed -n #,#p *'. `\` before a character
//	          matches that character as it is: '\#' is a #, '\*' a *.
//	paths     `*` stays inside one folder, `**` crosses folders, `~` is your home.
//	          A pattern with no "/" matches that name in any folder: '.env'.
type glob struct {
	text    string
	pattern *regexp.Regexp
}

func (g glob) matches(text string) bool { return g.pattern.MatchString(text) }

func anyMatches(globs []glob, text string) bool {
	for _, g := range globs {
		if g.matches(text) {
			return true
		}
	}
	return false
}

func commandGlob(text string) (glob, error) {
	body, optionalRest := text, false
	if strings.HasSuffix(text, " *") {
		body, optionalRest = strings.TrimSuffix(text, " *"), true
	}
	var re strings.Builder
	re.WriteString("(?s)^") // a command's words may hold newlines, as a commit message does
	escaped := false
	for _, r := range body {
		switch {
		case escaped:
			re.WriteString(regexp.QuoteMeta(string(r)))
			escaped = false
		case r == '\\':
			escaped = true
		case r == '*':
			re.WriteString(".*")
		case r == '?':
			re.WriteString(".")
		case r == '#':
			re.WriteString("[0-9]+")
		default:
			re.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	if escaped {
		re.WriteString(regexp.QuoteMeta("\\")) // a \ at the end has nothing to escape
	}
	if optionalRest {
		re.WriteString("( .*)?")
	}
	re.WriteString("$")
	pattern, err := regexp.Compile(re.String())
	return glob{text: text, pattern: pattern}, err
}

func pathGlob(text, home string) (glob, error) {
	body := text
	if body == "~" || strings.HasPrefix(body, "~/") {
		body = home + strings.TrimPrefix(body, "~")
	}
	var re strings.Builder
	re.WriteString("^")
	if !strings.Contains(body, "/") {
		re.WriteString("(?:.*/)?") // a bare name matches in any folder
	}
	for i := 0; i < len(body); i++ {
		switch {
		case strings.HasPrefix(body[i:], "**/"):
			re.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(body[i:], "**"):
			re.WriteString(".*")
			i++
		case body[i] == '*':
			re.WriteString("[^/]*")
		case body[i] == '?':
			re.WriteString("[^/]")
		default:
			re.WriteString(regexp.QuoteMeta(body[i : i+1]))
		}
	}
	re.WriteString("$")
	pattern, err := regexp.Compile(re.String())
	return glob{text: text, pattern: pattern}, err
}
