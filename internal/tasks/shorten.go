package tasks

import "unicode/utf8"

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
