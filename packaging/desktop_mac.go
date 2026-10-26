package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// The Mac app is built on a Mac: its window is WKWebView through cgo, which
// needs Apple's SDK and clang. It is one universal Agentry.app, for Apple
// Silicon and Intel, zipped with its README in the folder Agentry.

// macMinimum is the oldest macOS the app runs on: Go 1.27 builds its own
// code for macOS 13, above Wails v2's 10.13.
const macMinimum = "13.0"

func runMacDesktop(version string) error {
	builds, err := os.MkdirTemp("", "agentry-desktop-builds-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(builds)
	folder := filepath.Join(builds, "Agentry")
	if err := makeMacApp(filepath.Join(folder, "Agentry.app"), filepath.Join(builds, "darwin"), version); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(folder, "README.txt"), []byte(desktopNotes(desktopReadme, "darwin", version)), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll("release", 0o755); err != nil {
		return err
	}
	name := filepath.Join("release", "agentry-desktop-"+version+"-macos-universal.zip")
	os.Remove(name) // ditto adds to nothing; an old zip would stay as it is
	// ditto keeps the programs' modes, so the zip opens to an app that runs.
	if err := runTool("ditto", "-c", "-k", "--keepParent", folder, name); err != nil {
		return err
	}
	info, _ := os.Stat(name)
	fmt.Printf("Made %s (%.0f MB)\n", name, float64(info.Size())/(1<<20))
	return nil
}

// makeMacApp makes the bundle app, building into build: the app and the
// server it starts, side by side in Contents/MacOS, where the app looks for
// the server, with the icon and Info.plist.
func makeMacApp(app, build, version string) error {
	programs := filepath.Join(app, "Contents", "MacOS")
	resources := filepath.Join(app, "Contents", "Resources")
	for _, dir := range []string{build, programs, resources} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	var desktops, servers []string
	for _, goarch := range []string{"arm64", "amd64"} {
		desktop := filepath.Join(build, "agentry-desktop-"+goarch)
		if err := buildMacDesktop(desktop, goarch); err != nil {
			return err
		}
		server := filepath.Join(build, "agentry-"+goarch)
		if err := buildAgentry(server, "darwin", goarch); err != nil {
			return err
		}
		desktops, servers = append(desktops, desktop), append(servers, server)
	}
	if err := lipo(filepath.Join(programs, "agentry-desktop"), desktops); err != nil {
		return err
	}
	if err := lipo(filepath.Join(programs, "agentry"), servers); err != nil {
		return err
	}
	if err := makeIcns(filepath.Join(resources, "agentry.icns"), build); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(infoPlist(version)), 0o644); err != nil {
		return err
	}
	return signAdHoc(app)
}

// signAdHoc signs the app with no identity: Apple Silicon runs no arm64
// code without a signature, and lipo's universal programs keep only the
// arm64 half's linker signature, which also names no bundle. The server is
// signed first, since signing the app seals what is inside it. It is still
// unsigned in Gatekeeper's sense: no developer, not notarised.
func signAdHoc(app string) error {
	server := filepath.Join(app, "Contents", "MacOS", "agentry")
	if err := runTool("codesign", "--force", "--sign", "-", "--identifier", "com.agent3labs.agentry.server", server); err != nil {
		return err
	}
	return runTool("codesign", "--force", "--sign", "-", app)
}

// buildMacDesktop builds the app for darwin/goarch to path. Go passes clang
// the arch, so one Mac builds both.
func buildMacDesktop(path, goarch string) error {
	fmt.Printf("Building agentry-desktop for darwin/%s\n", goarch)
	app, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	build := exec.Command("go", "build", "-trimpath", "-tags", "desktop,production", "-ldflags", "-s -w", "-o", app, ".")
	build.Dir = filepath.Join("cmd", "agentry-desktop")
	// The C part is built for macMinimum too, or it would need this Mac's
	// macOS; Wails' window uses UniformTypeIdentifiers, as wails build links.
	build.Env = append(os.Environ(), "GOOS=darwin", "GOARCH="+goarch, "CGO_ENABLED=1",
		"CGO_CFLAGS=-mmacosx-version-min="+macMinimum,
		"CGO_LDFLAGS=-framework UniformTypeIdentifiers -mmacosx-version-min="+macMinimum)
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("%w: the Mac app needs the Xcode command line tools (xcode-select --install)", err)
	}
	return nil
}

// lipo joins the programs, one for each arch, into one universal program.
func lipo(path string, programs []string) error {
	if err := runTool("lipo", append([]string{"-create", "-output", path}, programs...)...); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

// makeIcns makes the app's icon from desktopIcon, with sips and iconutil,
// which every Mac has, in each size macOS asks for; scratch holds the
// iconset.
func makeIcns(path, scratch string) error {
	set := filepath.Join(scratch, "agentry.iconset")
	if err := os.MkdirAll(set, 0o755); err != nil {
		return err
	}
	for _, size := range []int{16, 32, 128, 256, 512} {
		for scale := 1; scale <= 2; scale++ {
			name := fmt.Sprintf("icon_%dx%d.png", size, size)
			if scale == 2 {
				name = fmt.Sprintf("icon_%dx%d@2x.png", size, size)
			}
			pixels := fmt.Sprint(size * scale)
			if err := runTool("sips", "-z", pixels, pixels, desktopIcon, "--out", filepath.Join(set, name)); err != nil {
				return err
			}
		}
	}
	return runTool("iconutil", "-c", "icns", "-o", path, set)
}

// infoPlist is the app's Info.plist for version.
func infoPlist(version string) string {
	number := bundleVersion(version)
	keys := []struct{ key, value string }{
		{"CFBundleName", "Agentry"},
		{"CFBundleDisplayName", "Agentry"},
		{"CFBundleIdentifier", "com.agent3labs.agentry"},
		{"CFBundleExecutable", "agentry-desktop"},
		{"CFBundleIconFile", "agentry"},
		{"CFBundlePackageType", "APPL"},
		{"CFBundleShortVersionString", number},
		{"CFBundleVersion", number},
		{"LSMinimumSystemVersion", macMinimum},
	}
	var text strings.Builder
	text.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	for _, k := range keys {
		fmt.Fprintf(&text, "\t<key>%s</key>\n\t<string>%s</string>\n", k.key, escaped(k.value))
	}
	text.WriteString("\t<key>NSHighResolutionCapable</key>\n\t<true/>\n</dict>\n</plist>\n")
	return text.String()
}

// releaseNumber is the x.y.z a tagged git version starts with, after its v;
// an untagged one is a commit hash.
var releaseNumber = regexp.MustCompile(`^\d+\.\d+(\.\d+)?`)

// bundleVersion is the app's version from the git version: its release
// number, 0.0.0 for an untagged build. macOS takes only numbers there, so
// the rest of the git version (commits since the tag, -dirty) is left out;
// it is in the zip's name and README.
func bundleVersion(version string) string {
	if number := releaseNumber.FindString(strings.TrimPrefix(version, "v")); number != "" {
		return number
	}
	return "0.0.0"
}

func escaped(s string) string {
	var out strings.Builder
	xml.EscapeText(&out, []byte(s))
	return out.String()
}

// runTool runs a Mac tool, with its errors on our stderr; sips would print
// each file it makes on stdout.
func runTool(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
