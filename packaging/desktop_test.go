package main

import (
	"strings"
	"testing"
)

func TestDesktopNotesFillEveryBlankForTheirSystem(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		text := desktopNotes(desktopReadme, goos, "v1.2.3")
		if strings.Contains(text, "{{") {
			t.Errorf("%s: a blank is left in\n%s", goos, text)
		}
		if !strings.Contains(text, "Version v1.2.3") {
			t.Errorf("%s: no version in\n%s", goos, text)
		}
		windows := goos == "windows"
		if strings.Contains(text, "agentry-desktop.exe") != windows {
			t.Errorf("%s: the wrong system's steps in\n%s", goos, text)
		}
		crlf := 0 // Windows ends every line with \r\n, Linux none
		if windows {
			crlf = strings.Count(text, "\n")
		}
		if got := strings.Count(text, "\r\n"); got != crlf {
			t.Errorf("%s: %d lines end in \\r\\n, want %d", goos, got, crlf)
		}
	}
}
