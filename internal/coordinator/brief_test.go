package coordinator

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A refused command cut for the brief keeps whole characters: a cut inside
// one backs off to its start.
func TestShortenCutsBetweenCharacters(t *testing.T) {
	text := "ab€cd" // € is 3 bytes, at bytes 2 to 4
	for limit, want := range map[int]string{2: "ab", 3: "ab", 4: "ab", 5: "ab€"} {
		got := shorten(text, limit)
		if !utf8.ValidString(got) || got != want+" [...]" {
			t.Errorf("shorten(%q, %d) = %q, want %q", text, limit, got, want+" [...]")
		}
	}
	if got := shorten(text, len(text)); got != text {
		t.Errorf("a text within the limit is kept whole, got %q", got)
	}
	if got := shorten(strings.Repeat("€", 100), 200); !utf8.ValidString(got) {
		t.Errorf("long text cut mid-character: %q", got)
	}
}
