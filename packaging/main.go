// Command packaging makes the downloads: one zip for each kind of computer,
// with everything it needs to run Agentry. In each, Start Agentry starts it
// in the background and opens it in the browser; Stop Agentry stops it.
// Agentry is pure Go, so every zip is built here, whatever this machine is.
//
//	go run ./packaging           (make package)
//	go run ./packaging desktop   (make desktop, see desktop.go)
//
// The zips go in release/.
package main

import (
	"archive/zip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type platform struct{ os, arch, label string }

var platforms = []platform{
	{"darwin", "arm64", "mac-apple-silicon"},
	{"darwin", "amd64", "mac-intel"},
	{"linux", "amd64", "linux-x64"},
	{"linux", "arm64", "linux-arm64"},
	{"windows", "amd64", "windows-x64"},
	{"windows", "arm64", "windows-arm64"},
}

var (
	//go:embed files/start-unix.sh
	startUnix string
	//go:embed files/stop-unix.sh
	stopUnix string
	//go:embed files/start-windows.cmd
	startWindows string
	//go:embed files/stop-windows.cmd
	stopWindows string
	//go:embed files/README.txt
	readme string
)

func main() {
	command := run
	if len(os.Args) > 1 && os.Args[1] == "desktop" {
		command = runDesktop
	}
	if err := command(); err != nil {
		fmt.Fprintln(os.Stderr, "packaging:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := checkRoot(); err != nil {
		return err
	}
	version := gitVersion()
	builds, err := os.MkdirTemp("", "agentry-builds-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(builds)
	for _, p := range platforms {
		if err := buildAgentry(filepath.Join(builds, "agentry-"+p.os+"-"+p.arch), p.os, p.arch); err != nil {
			return err
		}
	}
	if err := os.MkdirAll("release", 0o755); err != nil {
		return err
	}
	for _, p := range platforms {
		name := filepath.Join("release", "agentry-"+version+"-"+p.label+".zip")
		if err := pack(name, p, builds, version); err != nil {
			return err
		}
		info, _ := os.Stat(name)
		fmt.Printf("Made %s (%.0f MB)\n", name, float64(info.Size())/(1<<20))
	}
	return nil
}

// checkRoot refuses to run anywhere but the repo's root, with the page built
// into the server.
func checkRoot() error {
	if _, err := os.Stat("internal/web/dist/index.html"); err != nil {
		return fmt.Errorf("run it from the repo's root, with the page built (make ui)")
	}
	return nil
}

// buildAgentry builds the server for goos/goarch to path, without cgo, so
// any machine can build it for every other.
func buildAgentry(path, goos, goarch string) error {
	fmt.Printf("Building agentry for %s/%s\n", goos, goarch)
	build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", path, "./cmd/agentry")
	build.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	return build.Run()
}

// pack makes one zip: the folder Agentry, with this computer's agentry,
// the others' for its workers, Start and Stop Agentry and the README.
func pack(name string, p platform, builds, version string) error {
	out, err := os.Create(name)
	if err != nil {
		return err
	}
	defer out.Close()
	z := zip.NewWriter(out)
	defer z.Close()

	windows := p.os == "windows"
	self := "agentry"
	if windows {
		self = "agentry.exe"
	}
	if err := addFile(z, self, filepath.Join(builds, "agentry-"+p.os+"-"+p.arch), 0o755); err != nil {
		return err
	}
	for _, other := range platforms {
		if other != p {
			if err := addFile(z, "agentry-"+other.os+"-"+other.arch, filepath.Join(builds, "agentry-"+other.os+"-"+other.arch), 0o755); err != nil {
				return err
			}
		}
	}

	notes := map[string]string{"{{VERSION}}": version}
	switch {
	case windows:
		addText(z, "Start Agentry.cmd", startWindows, 0o644)
		addText(z, "Stop Agentry.cmd", stopWindows, 0o644)
		notes["{{START}}"] = "Double-click Start Agentry.cmd."
		notes["{{STOP}}"] = "Double-click Stop Agentry.cmd."
		notes["{{FIRST}}"] = "The first time, Windows may say it protected your PC: click More info,\r\nthen Run anyway. Agentry isn't signed yet."
	case p.os == "darwin":
		addText(z, "Start Agentry.command", startUnix, 0o755)
		addText(z, "Stop Agentry.command", stopUnix, 0o755)
		notes["{{START}}"] = "Double-click Start Agentry.command."
		notes["{{STOP}}"] = "Double-click Stop Agentry.command."
		notes["{{FIRST}}"] = "The first time, macOS may say it can't check the developer: right-click\nStart Agentry.command, choose Open, then Open again. Agentry isn't signed\nyet; after that it opens with a double-click."
	default:
		addText(z, "start-agentry.sh", startUnix, 0o755)
		addText(z, "stop-agentry.sh", stopUnix, 0o755)
		notes["{{START}}"] = "Run ./start-agentry.sh (or double-click it, if your file manager runs scripts)."
		notes["{{STOP}}"] = "Run ./stop-agentry.sh."
		notes["{{FIRST}}"] = "Unzip it with a tool that keeps permissions (unzip does); if start-agentry.sh\nwon't run, chmod +x agentry start-agentry.sh stop-agentry.sh."
	}
	text := readme
	for key, value := range notes {
		text = strings.ReplaceAll(text, key, value)
	}
	if windows {
		text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
	}
	return addText(z, "README.txt", text, 0o644)
}

func addFile(z *zip.Writer, name, path string, mode os.FileMode) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	header := &zip.FileHeader{Name: "Agentry/" + name, Method: zip.Deflate, Modified: built}
	header.SetMode(mode)
	w, err := z.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, in)
	return err
}

func addText(z *zip.Writer, name, text string, mode os.FileMode) error {
	header := &zip.FileHeader{Name: "Agentry/" + name, Method: zip.Deflate, Modified: built}
	header.SetMode(mode)
	w, err := z.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, text)
	return err
}

// built is when the zips were made: their files' date.
var built = time.Now()

func gitVersion() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(out))
}
