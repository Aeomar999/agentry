package connectionsapi

import (
	_ "embed"
	"net/http"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/connections"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
)

// A machine connecting its Claude Code with an install command's code,
// which it shows instead of a token: the install scripts, agentry for its
// system, and connect. Each route checks the code itself.

//go:embed claude-code.sh
var claudeCodeTemplate string

//go:embed claude-code.ps1
var claudeCodeWindowsTemplate string

const unknownInstallCode = "That install command is unknown, used or expired. Make a new one on the Integrations page."

// claudeCodeScript is the install script: for sh, or for PowerShell on Windows.
func (a API) claudeCodeScript(w http.ResponseWriter, r *http.Request) {
	a.ServeScript(w, r, "claude-code.sh", claudeCodeTemplate, a.codeWorks, unknownInstallCode)
}

func (a API) claudeCodeWindowsScript(w http.ResponseWriter, r *http.Request) {
	a.ServeScript(w, r, "claude-code.ps1", claudeCodeWindowsTemplate, a.codeWorks, unknownInstallCode)
}

// downloadBinary sends agentry for the machine, for a code that works.
func (a API) downloadBinary(w http.ResponseWriter, r *http.Request) {
	if !a.codeWorks(r.URL.Query().Get("code")) {
		http.Error(w, "that install command is unknown, used or expired", http.StatusUnauthorized)
		return
	}
	a.ServeBinary(w, r)
}

// connect swaps a code for the machine's credential, once.
func (a API) connect(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Code string
		connections.Machine
	}
	if !httpapi.Decode(w, r, &request) {
		return
	}
	credential, made := connections.Connect(request.Code, request.Machine)
	if !httpapi.ProposeOrRefuse(w, a.Log, made) {
		return
	}
	httpapi.Reply(w, map[string]string{"credential": credential, "page": a.PublicURL(r)})
}

func (a API) codeWorks(code string) bool {
	roster, err := a.State.Connections(), a.State.Broken(events.ConnectionsStream)
	return err == nil && code != "" && roster.CanConnect(workers.Hash(code), time.Now())
}
