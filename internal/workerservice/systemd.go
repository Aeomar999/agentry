package workerservice

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// systemd is a systemd user service, which runs while you are logged in,
// or always once lingering is on (Start turns it on where it may).
type systemd struct{}

const systemdUnit = "agentry-worker.service"

func (systemd) Kind() string { return "systemd user service " + systemdUnit }

func (systemd) unitFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", systemdUnit)
}

func (s systemd) Start(spec Spec) error {
	unit := strings.Join([]string{
		"[Unit]",
		"Description=Agentry worker",
		"After=network-online.target",
		"",
		"[Service]",
		"Environment=PATH=" + spec.Path,
		"ExecStart=" + strings.Join(quoteAll(spec.command()), " "),
		"StandardOutput=append:" + spec.Log,
		"StandardError=append:" + spec.Log,
		"Restart=always",
		"RestartSec=5",
		"",
		"[Install]",
		"WantedBy=default.target",
		"",
	}, "\n")
	if err := writeFile(s.unitFile(), unit); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", systemdUnit}, {"restart", systemdUnit}} {
		if err := run("systemctl", append([]string{"--user"}, args...)...); err != nil {
			return err
		}
	}
	// Keep it running when you log out, where the machine lets us.
	if me, err := user.Current(); err == nil {
		exec.Command("loginctl", "enable-linger", me.Username).Run()
	}
	return nil
}

func (systemd) Stop() error { return run("systemctl", "--user", "stop", systemdUnit) }

func (systemd) Running() bool {
	return exec.Command("systemctl", "--user", "is-active", "--quiet", systemdUnit).Run() == nil
}

func (s systemd) Remove() error {
	exec.Command("systemctl", "--user", "disable", "--now", systemdUnit).Run()
	if err := os.Remove(s.unitFile()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return run("systemctl", "--user", "daemon-reload")
}

// quoteAll quotes each word for systemd's ExecStart, which splits on spaces.
func quoteAll(words []string) []string {
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = `"` + strings.ReplaceAll(word, `"`, `\"`) + `"`
	}
	return quoted
}
