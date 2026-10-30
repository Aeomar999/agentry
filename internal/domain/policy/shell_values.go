package policy

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

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

// partsValues is every string the parts of a word can expand to; quoted says
// they are inside double quotes.
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

// joinsDollar: a backslash and a newline sit between a $ and the rest of the
// word. mvdan keeps any $ that starts no expansion as its own literal (a$/b is
// a, $, /b) and drops a backslash-newline, but bash joins the lines first, so
// $\<newline>x is $x. Only the backslash-newline leaves a gap in the source
// between the $ and the next part; with no gap the $ is plain text.
func joinsDollar(parts []syntax.WordPart) bool {
	for i, part := range parts[:max(len(parts)-1, 0)] {
		lit, isLit := part.(*syntax.Lit)
		if isLit && lit.Value == "$" && parts[i+1].Pos().Offset() > lit.Pos().Offset()+1 {
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
	case *syntax.ExtGlob: // kept as written, for the paths it may match
		return []string{p.Op.String() + p.Pattern.Value + ")"}, true
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
