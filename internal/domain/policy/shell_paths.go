package policy

import (
	"encoding/json"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

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
					files = append(files, s.resolveClean(item)...)
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

// addPath records a command argument that may name a file. URLs, empty words
// and options without a value don't; a word whose value can't be known makes
// the call opaque.
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
		s.call.named = append(s.call.named, s.knownNames(word)...)
		return
	}
	for _, value := range values {
		if !isTarget && strings.HasPrefix(value, "-") {
			value = optionPath(value)
		}
		if _, inStack := inFolderStack(value); inStack { // its folder can't be known; inspect judges what lies below it
			s.call.opaque = true
		}
		if value == "" || strings.Contains(value, "://") {
			continue
		}
		s.call.paths = append(s.call.paths, s.resolve(value)...)
	}
}

// knownNames is the names a word that can't be fully known still spells
// out: each piece of its known text that starts the word or follows a /, up
// to the next / or unknown part. "$(pwd)/../x/y" names .., x and y; a piece
// right after an unknown part, b in $(a)b, may be the end of any name, so it
// isn't one.
func (s *shellInspector) knownNames(word *syntax.Word) []string {
	var names []string
	text, atStart := "", true // the known text since the last unknown part
	takeNames := func() {
		pieces := strings.Split(text, "/")
		if !atStart {
			pieces = pieces[1:]
		}
		names = append(names, pieces...)
	}
	s.eachPart(word.Parts, false, func(value string, known bool) {
		if known {
			text += value
			return
		}
		takeNames()
		text, atStart = "", false
	})
	takeNames()
	return names
}

// eachPart hands visit each part of a word, inside double quotes too, with
// its value when it has exactly one that can be known.
func (s *shellInspector) eachPart(parts []syntax.WordPart, quoted bool, visit func(value string, known bool)) {
	for _, part := range parts {
		if inner, ok := part.(*syntax.DblQuoted); ok {
			s.eachPart(inner.Parts, true, visit)
			continue
		}
		values, ok := s.partValues(part, quoted)
		if ok && len(values) == 1 {
			visit(values[0], true)
		} else {
			visit("", false)
		}
	}
}

// optionPath is the part of an option that may name a file: the value of
// --opt=value, or of a short option joined to a value that holds a /
// (-C/srv, -o./out). Other options name none.
func optionPath(option string) string {
	if _, value, found := strings.Cut(option, "="); found {
		return value
	}
	if !strings.HasPrefix(option, "--") && len(option) > 2 && strings.Contains(option, "/") {
		return option[2:]
	}
	return ""
}
