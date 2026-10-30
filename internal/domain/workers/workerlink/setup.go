package workerlink

import (
	"os"
	"path/filepath"

	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/platform/pinned"
)

// Setup is what a worker that joined a server works with: `agentry worker
// run` makes its Worker from it.
type Setup struct {
	Server, Name, Credential string
	CA                       string // the server's own CA, PEM encoded, when it is served over https with one
	Dir                      string // where the worker keeps its repos, worktrees and the CA for git
	HookCommand              string // how the agents' hooks run agentry
}

// CAFile is where a worker in dir keeps the server's CA for git, which
// reads it from a file.
func CAFile(dir string) string { return filepath.Join(dir, "server-ca.pem") }

// Worker is the worker's end of the link, with this machine to do the
// work. The link, git, shared files and the agents' hooks all trust only
// the server's CA when it has one.
func (s Setup) Worker() (*Worker, error) {
	client := pinned.Client([]byte(s.CA), 0)
	hub := &machines.Hub{URL: HubURL(s.Server), Credential: s.Credential, MirrorRoot: filepath.Join(s.Dir, "repos"), HTTP: client}
	if s.CA != "" {
		if err := os.WriteFile(CAFile(s.Dir), []byte(s.CA), 0o600); err != nil {
			return nil, err
		}
		hub.CAFile = CAFile(s.Dir)
	}
	return &Worker{Server: s.Server, Credential: s.Credential, HTTP: client, About: machines.Describe(),
		Machine: &machines.Local{Label: s.Name, ServerURL: s.Server, ServerCA: s.CA, HookCommand: s.HookCommand,
			Blobs:        sharedfiles.Hub{Server: s.Server, Credential: s.Credential, HTTP: client},
			WorktreeRoot: filepath.Join(s.Dir, "worktrees"), Hub: hub}}, nil
}
