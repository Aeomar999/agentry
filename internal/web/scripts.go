package web

import (
	"net/http"
	"strings"
)

// The install commands the Workers and Integrations pages show: one for
// macOS and Linux (curl, piped to sh) and one for Windows (PowerShell). The
// script comes with this server's address and the code in it; anyone can
// fetch it, but it does nothing without a code that works.

// script is one install script, for sh or for PowerShell.
type script struct {
	template string
	quote    func(string) string
	kind     string // its Content-Type
	refuse   func(why string) string
}

func shScript(template string) script {
	return script{template: template, quote: shellQuoted, kind: "text/x-shellscript",
		refuse: func(why string) string { return "echo '" + why + "' >&2; exit 1" }}
}

func powerShellScript(template string) script {
	return script{template: template, quote: powerShellQuoted, kind: "text/plain; charset=utf-8",
		refuse: func(why string) string { return "Write-Error " + powerShellQuoted(why) + "; exit 1" }}
}

// serve sends the script for the request's code, if works says it is one
// that works, or the refusal, which the shell shows.
func (s *Server) serveScript(w http.ResponseWriter, r *http.Request, sc script, works func(code string) bool, refusal string) {
	code := r.URL.Query().Get("code")
	if !works(code) {
		http.Error(w, sc.refuse(refusal), http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", sc.kind)
	w.Write([]byte(strings.NewReplacer("{{SERVER}}", sc.quote(s.publicURL(r)), "{{CODE}}", sc.quote(code)).Replace(sc.template)))
}

// commands are the two ways to run the script at path?code=code: curl
// piped to sh, and in PowerShell, irm piped to iex (which runs it as text,
// so no execution policy stops it). The page shows the one for your system.
func commands(base, path, code string) map[string]any {
	sh := base + path + ".sh?code=" + code
	ps1 := base + path + ".ps1?code=" + code
	return map[string]any{
		"command":        "curl -fsSL '" + sh + "' | sh",
		"windowsCommand": "irm '" + ps1 + "' | iex", // in PowerShell, as Claude Code's own install is
	}
}

// powerShellQuoted quotes a value for a PowerShell script: inside single
// quotes, a quote is doubled.
func powerShellQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
