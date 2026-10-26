package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The desktop app, cmd/agentry-desktop, is packaged with the agentry server
// it starts: a .tar.gz for Linux and a zip for Windows, in release/. The
// Linux window is WebKitGTK through cgo, so it is built on Linux; Windows'
// WebView2 needs no cgo, so it is built here too. macOS is built on a Mac.

var (
	//go:embed files/desktop-README.txt
	desktopReadme string
	//go:embed files/agentry.desktop
	desktopEntry string
)

// desktopIcon is the app's icon, the one its window shows.
const desktopIcon = "cmd/agentry-desktop/build/appicon.png"

func runDesktop() error {
	if err := checkRoot(); err != nil {
		return err
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("the Linux desktop app is built on linux/amd64, not %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	version := gitVersion()
	builds, err := os.MkdirTemp("", "agentry-desktop-builds-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(builds)
	for _, goos := range []string{"linux", "windows"} {
		if err := buildDesktop(filepath.Join(builds, goos), goos); err != nil {
			return err
		}
	}
	if err := os.MkdirAll("release", 0o755); err != nil {
		return err
	}
	linux := filepath.Join("release", "agentry-desktop-"+version+"-linux-amd64.tar.gz")
	if err := packLinuxDesktop(linux, filepath.Join(builds, "linux"), version); err != nil {
		return err
	}
	windows := filepath.Join("release", "agentry-desktop-"+version+"-windows-amd64.zip")
	if err := packWindowsDesktop(windows, filepath.Join(builds, "windows"), version); err != nil {
		return err
	}
	for _, name := range []string{linux, windows} {
		info, _ := os.Stat(name)
		fmt.Printf("Made %s (%.0f MB)\n", name, float64(info.Size())/(1<<20))
	}
	return nil
}

// buildDesktop builds the app and the server for goos/amd64 into dir, as
// agentry-desktop and agentry (with .exe on Windows), side by side, where
// the app looks for the server.
func buildDesktop(dir, goos string) error {
	exe := ""
	if goos == "windows" {
		exe = ".exe"
	}
	if err := buildAgentry(filepath.Join(dir, "agentry"+exe), goos, "amd64"); err != nil {
		return err
	}
	fmt.Printf("Building agentry-desktop for %s/amd64\n", goos)
	tags, ldflags, cgo := "desktop,production,webkit2_41", "-s -w", "1"
	if goos == "windows" {
		// A GUI program, so Windows opens no console beside the window. Its
		// icon is the committed rsrc_windows_amd64.syso (go generate).
		tags, ldflags, cgo = "desktop,production", "-s -w -H=windowsgui", "0"
	}
	app, err := filepath.Abs(filepath.Join(dir, "agentry-desktop"+exe))
	if err != nil {
		return err
	}
	build := exec.Command("go", "build", "-trimpath", "-tags", tags, "-ldflags", ldflags, "-o", app, ".")
	build.Dir = filepath.Join("cmd", "agentry-desktop")
	build.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED="+cgo)
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	err = build.Run()
	if err != nil && goos == "linux" {
		return fmt.Errorf("%w: the Linux app needs libwebkit2gtk-4.1-dev and libgtk-3-dev (README.md, Make a release)", err)
	}
	return err
}

// packLinuxDesktop makes the Linux .tar.gz: the folder Agentry, with the
// app, the server, the menu entry, the icon and the README.
func packLinuxDesktop(name, build, version string) error {
	out, err := os.Create(name)
	if err != nil {
		return err
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	t := tar.NewWriter(gz)
	for _, program := range []string{"agentry-desktop", "agentry"} {
		if err := addTarFile(t, program, filepath.Join(build, program), 0o755); err != nil {
			return err
		}
	}
	if err := addTarFile(t, "agentry.png", desktopIcon, 0o644); err != nil {
		return err
	}
	if err := addTarText(t, "agentry.desktop", desktopEntry, 0o644); err != nil {
		return err
	}
	if err := addTarText(t, "README.txt", desktopNotes(desktopReadme, "linux", version), 0o644); err != nil {
		return err
	}
	if err := t.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return out.Close()
}

// packWindowsDesktop makes the Windows zip: the folder Agentry, with the
// app, the server and the README.
func packWindowsDesktop(name, build, version string) error {
	out, err := os.Create(name)
	if err != nil {
		return err
	}
	defer out.Close()
	z := zip.NewWriter(out)
	for _, program := range []string{"agentry-desktop.exe", "agentry.exe"} {
		if err := addFile(z, program, filepath.Join(build, program), 0o755); err != nil {
			return err
		}
	}
	if err := addText(z, "README.txt", desktopNotes(desktopReadme, "windows", version), 0o644); err != nil {
		return err
	}
	if err := z.Close(); err != nil {
		return err
	}
	return out.Close()
}

// desktopNotes is the desktop README for goos: its open and first-run steps
// and the version, with Windows line ends on Windows.
func desktopNotes(text, goos, version string) string {
	notes := map[string]string{"{{VERSION}}": version}
	if goos == "windows" {
		notes["{{OPEN}}"] = "Double-click agentry-desktop.exe."
		notes["{{FIRST}}"] = "The first time, Windows may say it protected your PC: click More info,\nthen Run anyway. Agentry isn't signed yet. The window needs Microsoft\nEdge WebView2, which Windows 10 and 11 already have."
	} else {
		notes["{{OPEN}}"] = "Run ./agentry-desktop. To have it in your applications menu, copy it\nand agentry to ~/.local/bin, agentry.png to\n~/.local/share/icons/hicolor/512x512/apps and agentry.desktop to\n~/.local/share/applications. If the menu's Agentry doesn't open, log out\nand in again, so ~/.local/bin is on your PATH."
		notes["{{FIRST}}"] = "The window needs WebKitGTK 4.1 and GTK 3, which most Linux desktops have\n(on Debian or Ubuntu, the package libwebkit2gtk-4.1-0 brings both)."
	}
	for key, value := range notes {
		text = strings.ReplaceAll(text, key, value)
	}
	if goos == "windows" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

func addTarFile(t *tar.Writer, name, path string, mode int64) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := t.WriteHeader(&tar.Header{Name: "Agentry/" + name, Mode: mode, Size: info.Size(), ModTime: built}); err != nil {
		return err
	}
	_, err = io.Copy(t, in)
	return err
}

func addTarText(t *tar.Writer, name, text string, mode int64) error {
	if err := t.WriteHeader(&tar.Header{Name: "Agentry/" + name, Mode: mode, Size: int64(len(text)), ModTime: built}); err != nil {
		return err
	}
	_, err := io.WriteString(t, text)
	return err
}
