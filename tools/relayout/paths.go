package main

import "strings"

// module is the module's path; an import names a package as module/old.
const module = "gitlab.com/dev.chempa/agentry"

// replacePath replaces old with new in text wherever old is a whole path
// (see isWholePath), never by a plain text replace: internal/store is the
// start of internal/storetest.
func replacePath(text, old, new string) string {
	var b strings.Builder
	done := 0
	for from := 0; ; {
		i := strings.Index(text[from:], old)
		if i < 0 {
			break
		}
		start, end := from+i, from+i+len(old)
		if isWholePath(text, start, end) {
			b.WriteString(text[done:start])
			b.WriteString(new)
			done = end
		}
		from = end
	}
	if done == 0 {
		return text
	}
	b.WriteString(text[done:])
	return b.String()
}

// isWholePath says whether text[start:end] is a path of its own, ending
// at a segment's end: the next character is not a letter, digit, _ or -.
// It must also start a path: at the start of a word (`internal/store`,
// "internal/store"), after the module (module/internal/store), or after
// ./ or ../. A path with one segment, such as packaging, is also a word,
// so as a word it must be followed by a slash: packaging/main.go is a path,
// "Command packaging makes" is not.
func isWholePath(text string, start, end int) bool {
	if end < len(text) && isNameByte(text[end]) {
		return false
	}
	switch {
	case afterPrefix(text[:start]):
		return true
	case startsWord(text[:start]):
		return strings.Contains(text[start:end], "/") || (end < len(text) && text[end] == '/')
	}
	return false
}

// afterPrefix says whether before ends with a path's start: module/, or
// ./ or ../ (as many ../ as it likes) at the start of a word, so neither
// x./internal/store nor ui/../internal/store is one.
func afterPrefix(before string) bool {
	if strings.HasSuffix(before, module+"/") {
		return true
	}
	rest, ok := strings.CutSuffix(before, "./")
	if !ok {
		return false
	}
	rest = strings.TrimSuffix(rest, ".")
	for strings.HasSuffix(rest, "../") {
		rest = strings.TrimSuffix(rest, "../")
	}
	return startsWord(rest)
}

// startsWord says whether a path can start right after before: at the
// start of the text, or after a character that is in no path.
func startsWord(before string) bool {
	if before == "" {
		return true
	}
	last := before[len(before)-1]
	return !isNameByte(last) && last != '.' && last != '/'
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// rewriteText rewrites every old path of rows in text to its new path.
func rewriteText(text string, rows []move) string {
	for _, m := range rows {
		text = replacePath(text, m.old, m.new)
	}
	return text
}
