package machines

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/platform/engines"
)

// Describe is what this machine tells the server about itself: what it
// is, and which engines and tools agents will find here.
func Describe() workers.Machine {
	host, _ := os.Hostname()
	m := workers.Machine{Host: host, OS: runtime.GOOS, Arch: runtime.GOARCH, Version: AgentryVersion(),
		Engines: []string{}, Missing: []string{}}
	for _, name := range engines.Names() {
		if version, ok := EngineVersion(name); ok {
			m.Engines = append(m.Engines, strings.TrimSpace(name+" "+version))
		}
	}
	for _, tool := range []string{"git"} {
		if _, err := exec.LookPath(tool); err != nil {
			m.Missing = append(m.Missing, tool)
		}
	}
	return m
}

// EngineVersion asks an engine's CLI for its version: "2.1.301" from
// "2.1.301 (Claude Code)". It reports false when there is no such program.
func EngineVersion(name string) (string, bool) {
	return versionOf(name, "--version")
}

// versionOf runs a program with the flag that prints its version, and
// picks the version out: the first word that starts with a digit.
func versionOf(name, flag string) (string, bool) {
	if _, err := exec.LookPath(name); err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, name, flag)
	command.WaitDelay = time.Second // a child left holding its output can't hold us too
	output, err := command.Output()
	if err != nil {
		return "", true
	}
	for _, word := range strings.Fields(string(output)) {
		if word[0] >= '0' && word[0] <= '9' {
			return word, true
		}
	}
	return "", true
}

// AgentryVersion is the commit this agentry was built from.
func AgentryVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && len(setting.Value) >= 7 {
			return setting.Value[:7]
		}
	}
	return info.Main.Version
}
