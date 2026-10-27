package main

import (
	"strings"
	"testing"
)

func TestDesktopNotesFillEveryBlankForTheirSystem(t *testing.T) {
	for _, goos := range []string{"linux", "windows", "darwin"} {
		text := desktopNotes(desktopReadme, goos, "v1.2.3")
		if strings.Contains(text, "{{") {
			t.Errorf("%s: a blank is left in\n%s", goos, text)
		}
		if !strings.Contains(text, "Version v1.2.3") {
			t.Errorf("%s: no version in\n%s", goos, text)
		}
		windows := goos == "windows"
		if strings.Contains(text, "agentry-desktop.exe") != windows || strings.Contains(text, "Agentry.app") != (goos == "darwin") {
			t.Errorf("%s: the wrong system's steps in\n%s", goos, text)
		}
		inApp := strings.Contains(text, "agentry inside Agentry.app")
		if inApp != (goos == "darwin") || strings.Contains(text, "agentry in this folder") == inApp {
			t.Errorf("%s: the wrong place for the server in\n%s", goos, text)
		}
		if goos == "darwin" && (!strings.Contains(text, "Open Anyway") || strings.Contains(text, "right-click")) {
			t.Errorf("darwin: not Open Anyway alone in\n%s", text)
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

func TestBundleVersionIsTheReleaseNumber(t *testing.T) {
	for version, want := range map[string]string{
		"v0.1.0":                   "0.1.0",
		"v0.1.0-5-g9e240bd7-dirty": "0.1.0",
		"9e240bd7":                 "0.0.0",
		"dev":                      "0.0.0",
	} {
		if got := bundleVersion(version); got != want {
			t.Errorf("bundleVersion(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestInfoPlistNamesTheAppItsProgramAndIcon(t *testing.T) {
	plist := infoPlist("v1.2.3")
	for _, want := range []string{
		"<key>CFBundleExecutable</key>\n\t<string>agentry-desktop</string>",
		"<key>CFBundleIconFile</key>\n\t<string>agentry</string>",
		"<key>CFBundleShortVersionString</key>\n\t<string>1.2.3</string>",
		"<key>CFBundleVersion</key>\n\t<string>1.2.3</string>",
		"<key>NSHighResolutionCapable</key>\n\t<true/>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("no %q in\n%s", want, plist)
		}
	}
}
