package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// The stand-ins for the programs: agentry-desktop names Agentry's desktop
// app, as its build info does, which is how uninstall.sh knows its own.
const (
	fakeApp    = "#!/bin/sh\n# gitlab.com/dev.chempa/agentry/cmd/agentry-desktop v1.2.3\n"
	fakeServer = "#!/bin/sh\n# agentry v1.2.3\n"
)

// unpackedLinuxDesktop makes the Linux .tar.gz around the stand-ins and
// unpacks it, as a user would, returning its Agentry folder.
func unpackedLinuxDesktop(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh here, as on Windows")
	}
	build := t.TempDir()
	writeFile(t, filepath.Join(build, "agentry-desktop"), fakeApp, 0o755)
	writeFile(t, filepath.Join(build, "agentry"), fakeServer, 0o755)
	name := filepath.Join(t.TempDir(), "agentry-desktop.tar.gz")
	t.Chdir("../..") // packLinuxDesktop reads the icon from the repo's root
	if err := packLinuxDesktop(name, build, "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	gz, err := gzip.NewReader(in)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	r := tar.NewReader(gz)
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, h.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, path, string(data), os.FileMode(h.Mode))
	}
	return filepath.Join(dir, "Agentry")
}

// runScript runs a script of the folder with only HOME, PATH and the env
// given, from another folder, and returns what it printed.
func runScript(t *testing.T, folder, script, home string, env ...string) string {
	t.Helper()
	cmd := exec.Command(filepath.Join(folder, script))
	cmd.Dir = t.TempDir()
	cmd.Env = append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", script, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, path, text string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil { // past the umask
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func checkFile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Error(err)
		return
	}
	if info.Mode() != mode {
		t.Errorf("%s has mode %v, want %v", path, info.Mode(), mode)
	}
	if got := readFile(t, path); got != want {
		t.Errorf("%s holds %q, want %q", path, got, want)
	}
}

func checkGone(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("%s is still there (%v)", path, err)
		}
	}
}

// installed is where install.sh puts each file, under the data folder.
type installed struct{ app, server, uninstall, icon, entry string }

func installedUnder(data string) installed {
	apps := filepath.Join(data, "agentry")
	return installed{
		app:       filepath.Join(apps, "agentry-desktop"),
		server:    filepath.Join(apps, "agentry"),
		uninstall: filepath.Join(apps, "uninstall.sh"),
		icon:      filepath.Join(data, "icons", "hicolor", "512x512", "apps", "agentry.png"),
		entry:     filepath.Join(data, "applications", "agentry.desktop"),
	}
}

func TestTheLinuxTarGzHoldsInstallAndUninstallScripts(t *testing.T) {
	folder := unpackedLinuxDesktop(t)
	checkFile(t, filepath.Join(folder, "install.sh"), desktopInstall, 0o755)
	checkFile(t, filepath.Join(folder, "uninstall.sh"), desktopUninstall, 0o755)
}

func TestInstallShPutsTheAppInTheMenuByFullPaths(t *testing.T) {
	folder := unpackedLinuxDesktop(t)
	home := filepath.Join(t.TempDir(), "my home")
	icon := readFile(t, filepath.Join(folder, "agentry.png"))
	for run := 1; run <= 2; run++ { // a second run updates it in place
		out := runScript(t, folder, "install.sh", home)
		at := installedUnder(filepath.Join(home, ".local", "share"))
		checkFile(t, at.app, fakeApp, 0o755)
		checkFile(t, at.server, fakeServer, 0o755)
		checkFile(t, at.uninstall, desktopUninstall, 0o755)
		checkFile(t, at.icon, icon, 0o644)
		want := strings.NewReplacer(
			"Exec=agentry-desktop", `Exec="`+at.app+`"`,
			"Icon=agentry", "Icon="+at.icon,
		).Replace(desktopEntry)
		checkFile(t, at.entry, want, 0o644)
		for _, path := range []string{at.app, at.server, at.uninstall, at.icon, at.entry} {
			checkGone(t, path+".new")
			if !strings.Contains(out, path) && !strings.Contains(out, filepath.Dir(path)) {
				t.Errorf("run %d: %s is not named in\n%s", run, path, out)
			}
		}
		if !strings.Contains(out, "'"+at.uninstall+"'") {
			t.Errorf("run %d: no uninstall command in\n%s", run, out)
		}
	}
}

// A real program, a copy of sh, stands in for the app here: Linux refuses
// to write over a program that is running ("Text file busy"), but not to
// rename another onto it.
func TestInstallShReplacesTheAppWhileItRuns(t *testing.T) {
	folder := unpackedLinuxDesktop(t)
	sh, _ := exec.LookPath("sh")
	program := readFile(t, sh)
	writeFile(t, filepath.Join(folder, "agentry-desktop"), program, 0o755)
	home := t.TempDir()
	runScript(t, folder, "install.sh", home)
	app := installedUnder(filepath.Join(home, ".local", "share")).app
	running := exec.Command(app, "-c", "read line") // until stdin closes
	stdin, err := running.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	defer running.Wait()
	defer stdin.Close()

	runScript(t, folder, "install.sh", home)
	checkFile(t, app, program, 0o755)
	if err := running.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("the app was not running through the second install: %v", err)
	}
}

// XDG_DATA_HOME moves what install.sh writes, and the menu entry's Exec
// quotes a path with characters the Desktop Entry spec reserves.
func TestInstallShQuotesExecAndFollowsXDGDataHome(t *testing.T) {
	folder := unpackedLinuxDesktop(t)
	home := t.TempDir()
	root := t.TempDir()
	data := filepath.Join(root, `a b"$c\d%e`)
	runScript(t, folder, "install.sh", home, "XDG_DATA_HOME="+data)
	at := installedUnder(data)
	checkFile(t, at.app, fakeApp, 0o755)
	entry := readFile(t, at.entry)
	exec := `Exec="` + root + `/a b\\"\\$c\\\\d%%e/agentry/agentry-desktop"`
	icon := `Icon=` + root + `/a b"$c\\d%e/icons/hicolor/512x512/apps/agentry.png`
	for _, want := range []string{exec, icon} {
		if !strings.Contains(entry, want+"\n") {
			t.Errorf("no line %s in\n%s", want, entry)
		}
	}
	checkGone(t, filepath.Join(home, ".local", "share", "agentry"))
}

func TestUninstallShRemovesWhatInstallShWroteAndKeepsTheRest(t *testing.T) {
	folder := unpackedLinuxDesktop(t)
	home := t.TempDir()
	runScript(t, folder, "install.sh", home)
	data := filepath.Join(home, ".agentry", "agentry.db")
	writeFile(t, data, "the log", 0o644)
	other := filepath.Join(home, ".local", "bin", "agentry") // as the worker install puts it
	writeFile(t, other, "#!/bin/sh\n# another agentry\n", 0o755)

	out := runScript(t, folder, "uninstall.sh", home)
	at := installedUnder(filepath.Join(home, ".local", "share"))
	checkGone(t, at.app, at.server, at.uninstall, at.icon, at.entry, filepath.Dir(at.app))
	checkFile(t, data, "the log", 0o644)
	checkFile(t, other, "#!/bin/sh\n# another agentry\n", 0o755)
	for _, want := range []string{"Removed " + at.app, "Removed " + at.entry, filepath.Join(home, ".agentry") + " stays"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}

	out = runScript(t, folder, "uninstall.sh", home) // nothing left: still exit 0
	if !strings.Contains(out, "nothing to remove") || strings.Contains(out, "Removed") {
		t.Errorf("a second uninstall says\n%s", out)
	}
	checkFile(t, other, "#!/bin/sh\n# another agentry\n", 0o755)
}

// The README of v0.20.0 had the user copy the programs to ~/.local/bin and
// the icon and menu entry to ~/.local/share by hand.
func TestUninstallShRemovesTheHandCopiesOnlyWhenTheyAreAgentrys(t *testing.T) {
	folder := unpackedLinuxDesktop(t)
	icon := readFile(t, filepath.Join(folder, "agentry.png"))
	olderApp := strings.Replace(fakeApp, "v1.2.3", "v0.20.0", 1)
	notOurs := "#!/bin/sh\n# something else\n"
	for _, c := range []struct {
		name               string
		app, entry         string
		keepApp, keepEntry bool
	}{
		{name: "an older app", app: olderApp, entry: desktopEntry},
		{name: "not Agentry's app", app: notOurs, entry: desktopEntry, keepApp: true},
		{name: "another menu entry", app: fakeApp, entry: "[Desktop Entry]\nName=Agentry\n", keepEntry: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			bin := filepath.Join(home, ".local", "bin")
			at := installedUnder(filepath.Join(home, ".local", "share"))
			writeFile(t, filepath.Join(bin, "agentry-desktop"), c.app, 0o755)
			// The same as this release's, as the Claude Code and worker
			// installs put it there, so it always stays.
			writeFile(t, filepath.Join(bin, "agentry"), fakeServer, 0o755)
			writeFile(t, at.icon, icon, 0o644)
			writeFile(t, at.entry, c.entry, 0o644)

			out := runScript(t, folder, "uninstall.sh", home)
			for _, f := range []struct {
				path, text string
				keep       bool
			}{
				{filepath.Join(bin, "agentry-desktop"), c.app, c.keepApp},
				{filepath.Join(bin, "agentry"), fakeServer, true},
				{at.entry, c.entry, c.keepEntry},
				{at.icon, icon, false}, // the same as this release's
			} {
				if f.keep {
					if got := readFile(t, f.path); got != f.text {
						t.Errorf("%s holds %q, want it kept as %q", f.path, got, f.text)
					}
				} else {
					checkGone(t, f.path)
				}
			}
			if !strings.Contains(out, "Kept "+filepath.Join(bin, "agentry")+": Agentry's Claude Code and worker installs use it") {
				t.Errorf("no word on the agentry kept in\n%s", out)
			}
		})
	}
}

// The uninstall.sh install.sh put beside the app uninstalls that folder's
// install, with the download gone and XDG_DATA_HOME no longer set, and the
// hand copies too, comparing the icon with the installed one.
func TestTheInstalledUninstallShWorksWithoutTheDownload(t *testing.T) {
	folder := unpackedLinuxDesktop(t)
	icon := readFile(t, filepath.Join(folder, "agentry.png"))
	home := t.TempDir()
	at := installedUnder(filepath.Join(t.TempDir(), "data"))
	runScript(t, folder, "install.sh", home, "XDG_DATA_HOME="+filepath.Dir(filepath.Dir(at.app)))
	handApp := filepath.Join(home, ".local", "bin", "agentry-desktop")
	handIcon := installedUnder(filepath.Join(home, ".local", "share")).icon
	writeFile(t, handApp, fakeApp, 0o755)
	writeFile(t, handIcon, icon, 0o644)
	if err := os.RemoveAll(folder); err != nil {
		t.Fatal(err)
	}

	out := runScript(t, filepath.Dir(at.app), "uninstall.sh", home)
	checkGone(t, at.app, at.server, at.uninstall, at.icon, at.entry, filepath.Dir(at.app), handApp, handIcon)
	if !strings.Contains(out, "Removed "+at.uninstall) {
		t.Errorf("no word on removing itself in\n%s", out)
	}
}

// With CDPATH set, cd prints the folder it went to, which must not end up
// in the folder the scripts find their files in.
func TestTheScriptsWorkWithCDPATHSet(t *testing.T) {
	folder := unpackedLinuxDesktop(t)
	home := t.TempDir()
	at := installedUnder(filepath.Join(home, ".local", "share"))
	for _, script := range []string{"install.sh", "uninstall.sh"} {
		cmd := exec.Command("sh", filepath.Join("Agentry", script))
		cmd.Dir = filepath.Dir(folder)
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "CDPATH=" + cmd.Dir}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", script, err, out)
		}
		if script == "install.sh" {
			checkFile(t, at.app, fakeApp, 0o755)
		}
	}
	checkGone(t, at.app, at.server, at.uninstall, at.icon, at.entry, filepath.Dir(at.app))
}

// runDash runs uninstall.sh under dash, the sh of Debian and Ubuntu, from
// the folder dir, by the args given, or piped in when there are none.
func runDash(t *testing.T, dir, home string, args []string, env ...string) string {
	t.Helper()
	if _, err := exec.LookPath("dash"); err != nil {
		t.Skip("no dash here")
	}
	cmd := exec.Command("dash", args...)
	if len(args) == 0 {
		cmd.Stdin = strings.NewReader(desktopUninstall)
	}
	cmd.Dir = dir
	cmd.Env = append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("uninstall.sh: %v\n%s", err, out)
	}
	return string(out)
}

// Piped in, uninstall.sh has no folder of its own, so the folder it runs
// in, here the home folder, is not taken for the install.
func TestUninstallShPipedInRemovesTheInstallAndNotTheFolderItRunsIn(t *testing.T) {
	folder := unpackedLinuxDesktop(t)
	home := t.TempDir()
	runScript(t, folder, "install.sh", home)
	mine := filepath.Join(home, "agentry")
	writeFile(t, mine, "my own notes\n", 0o644)

	out := runDash(t, home, home, nil)
	at := installedUnder(filepath.Join(home, ".local", "share"))
	checkGone(t, at.app, at.server, at.uninstall, at.icon, at.entry, filepath.Dir(at.app))
	checkFile(t, mine, "my own notes\n", 0o644)
	if strings.Contains(out, "Removed "+mine+"\n") {
		t.Errorf("it says it removed %s in\n%s", mine, out)
	}
}

// With install.sh deleted from the unpacked folder, that folder is still
// not the install: it is not named agentry.
func TestUninstallShWithoutInstallShBesideItKeepsTheDownload(t *testing.T) {
	folder := unpackedLinuxDesktop(t)
	home := t.TempDir()
	data := filepath.Join(t.TempDir(), "data")
	runScript(t, folder, "install.sh", home, "XDG_DATA_HOME="+data)
	if err := os.Remove(filepath.Join(folder, "install.sh")); err != nil {
		t.Fatal(err)
	}

	runDash(t, folder, home, []string{"./uninstall.sh"}, "XDG_DATA_HOME="+data)
	at := installedUnder(data)
	checkGone(t, at.app, at.server, at.uninstall, at.icon, at.entry, filepath.Dir(at.app))
	checkFile(t, filepath.Join(folder, "agentry-desktop"), fakeApp, 0o755)
	checkFile(t, filepath.Join(folder, "agentry"), fakeServer, 0o755)
	checkFile(t, filepath.Join(folder, "uninstall.sh"), desktopUninstall, 0o755)
}

// A folder that looks like the install, but whose agentry-desktop is not
// Agentry's, keeps every file in it.
func TestUninstallShKeepsAFolderWhoseAppIsNotAgentrys(t *testing.T) {
	home := t.TempDir()
	apps := filepath.Join(t.TempDir(), "agentry")
	notOurs := "#!/bin/sh\n# something else\n"
	writeFile(t, filepath.Join(apps, "uninstall.sh"), desktopUninstall, 0o755)
	writeFile(t, filepath.Join(apps, "agentry-desktop"), notOurs, 0o755)
	writeFile(t, filepath.Join(apps, "agentry"), notOurs, 0o755)

	runDash(t, apps, home, []string{"./uninstall.sh"})
	checkFile(t, filepath.Join(apps, "uninstall.sh"), desktopUninstall, 0o755)
	checkFile(t, filepath.Join(apps, "agentry-desktop"), notOurs, 0o755)
	checkFile(t, filepath.Join(apps, "agentry"), notOurs, 0o755)
}
