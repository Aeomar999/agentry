package tasks

import (
	"strings"
	"unicode/utf8"
)

// Shorten cuts text to at most limit bytes, backing off to the start of a
// character so none is split. A negative limit counts as 0.
func Shorten(text string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + " [...]"
}

// OneLine is text on one line, its runs of white space each one space, cut
// to limit characters with an ellipsis when it is longer.
func OneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) > limit {
		text = string([]rune(text)[:limit-1]) + "…"
	}
	return text
}
