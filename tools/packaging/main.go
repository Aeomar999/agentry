// Command packaging makes the downloads: one zip for each kind of computer,
// with everything it needs to run Agentry. In each, Start Agentry starts it
// in the background and opens it in the browser; Stop Agentry stops it.
// Agentry is pure Go, so every zip is built here, whatever this machine is.
//
//	go run ./tools/packaging           (make package)
//	go run ./tools/packaging desktop   (make desktop, see desktop.go)
//	go run ./tools/packaging check     (make release-check, see check.go)
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
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "desktop":
			command = runDesktop
		case "check":
			command = func() error { return runCheck(os.Args[2:]) }
		}
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
		if err := buildAgentry(filepath.Join(builds, workerName(p)), p.os, p.arch); err != nil {
			return err
		}
	}
	if err := os.MkdirAll("release", 0o755); err != nil {
		return err
	}
	for _, p := range platforms {
		name := filepath.Join("release", serverZip(version, p))
		if err := pack(name, p, builds, version); err != nil {
			return err
		}
		info, _ := os.Stat(name)
		fmt.Printf("Made %s (%.0f MB)\n", name, float64(info.Size())/(1<<20))
	}
	return nil
}

// checkRoot refuses to run anywhere but the repo's root, with the page built
// into the server from HEAD.
func checkRoot() error {
	if _, err := os.Stat(page + "/index.html"); err != nil {
		return fmt.Errorf("run it from the repo's root, with the page built (make ui)")
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		fmt.Println("Not a git checkout, so the page's commit isn't checked")
		return nil
	}
	if err := checkPageCommit(head); err != nil {
		return err
	}
	return checkUICommitted()
}

// page is where make ui builds the page the server embeds.
const page = "internal/web/static/dist"

// checkPageCommit refuses a page built from another commit than head, such
// as one left from before a pull, or copied in from another checkout.
func checkPageCommit(head string) error {
	builtFrom, err := os.ReadFile(page + "/built-from")
	if err != nil {
		return fmt.Errorf("the page doesn't say which commit it was built from (no %s/built-from), so it may not be HEAD (%s): run `make ui`, or copy in a page built from this commit", page, head)
	}
	if commit := strings.TrimSpace(string(builtFrom)); commit != head {
		return fmt.Errorf("the page was built from %s, not from HEAD (%s): run `make ui`, or copy in a page built from this commit", commit, head)
	}
	return nil
}

// checkUICommitted refuses changes to ui/ that aren't committed: the page
// may hold them, though built-from names HEAD, or may not, though the tree
// does.
func checkUICommitted() error {
	changes, err := git("status", "--porcelain", "--", "ui")
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if changes != "" {
		return fmt.Errorf("ui/ has changes that aren't committed, so the page can't be HEAD's:\n%s\ncommit them, then run `make ui`", changes)
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
	if err := addFile(z, selfName(p), filepath.Join(builds, workerName(p)), 0o755); err != nil {
		return err
	}
	for _, other := range platforms {
		if other != p {
			if err := addFile(z, workerName(other), filepath.Join(builds, workerName(other)), 0o755); err != nil {
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
		notes["{{FIRST}}"] = "The first time, macOS refuses it, since Agentry isn't signed yet. Click\nDone on the warning, open System Settings, Privacy & Security, then\nclick Open Anyway and confirm. After that it opens with a double-click."
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

// serverZip is the name of p's zip for version.
func serverZip(version string, p platform) string {
	return "agentry-" + version + "-" + p.label + ".zip"
}

// serverPrograms are the programs in p's zip: its own agentry, then the
// others' for its workers.
func serverPrograms(p platform) []string {
	programs := []string{selfName(p)}
	for _, other := range platforms {
		if other != p {
			programs = append(programs, workerName(other))
		}
	}
	return programs
}

// selfName is the name of p's own agentry in its zip.
func selfName(p platform) string {
	if p.os == "windows" {
		return "agentry.exe"
	}
	return "agentry"
}

// workerName is the name of p's agentry in the other systems' zips.
func workerName(p platform) string {
	return "agentry-" + p.os + "-" + p.arch
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
