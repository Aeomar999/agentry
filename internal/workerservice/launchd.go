package workerservice

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// launchd is a launchd agent: macOS starts it when you log in and keeps it
// running.
type launchd struct{}

const launchdLabel = "dev.agentry.worker"

func (launchd) Kind() string { return "launchd agent " + launchdLabel }

func (launchd) plist() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

// domains are where the agent can be loaded: your login session, or, where
// there is none (over SSH), your user's.
func (launchd) domains() []string {
	return []string{fmt.Sprintf("gui/%d", os.Getuid()), fmt.Sprintf("user/%d", os.Getuid())}
}

func (l launchd) Start(spec Spec) error {
	var arguments strings.Builder
	for _, word := range spec.command() {
		arguments.WriteString("\t\t<string>" + html.EscapeString(word) + "</string>\n")
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + launchdLabel + `</string>
	<key>ProgramArguments</key>
	<array>
` + arguments.String() + `	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>` + html.EscapeString(spec.Path) + `</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>` + html.EscapeString(spec.Log) + `</string>
	<key>StandardErrorPath</key>
	<string>` + html.EscapeString(spec.Log) + `</string>
</dict>
</plist>
`
	if err := writeFile(l.plist(), plist); err != nil {
		return err
	}
	// Unload whatever was loaded before, so the new definition is the one that runs.
	l.unload()
	var err error
	for _, domain := range l.domains() {
		if err = run("launchctl", "bootstrap", domain, l.plist()); err == nil {
			return nil
		}
	}
	return err
}

func (l launchd) unload() {
	for _, domain := range l.domains() {
		exec.Command("launchctl", "bootout", domain+"/"+launchdLabel).Run()
	}
}

func (l launchd) Stop() error {
	l.unload()
	if l.Running() {
		return fmt.Errorf("launchctl couldn't stop %s", launchdLabel)
	}
	return nil
}

func (l launchd) Running() bool {
	for _, domain := range l.domains() {
		output, err := exec.Command("launchctl", "print", domain+"/"+launchdLabel).Output()
		if err == nil && strings.Contains(string(output), "state = running") {
			return true
		}
	}
	return false
}

func (l launchd) Remove() error {
	l.unload()
	if err := os.Remove(l.plist()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
