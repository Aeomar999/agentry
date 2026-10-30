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
// that works, or the refusal, which the shell shows. A script for a server
// served with Agentry's own CA carries the CA, which its download trusts
// alone, and its fingerprint, which joining checks.
func (s *Server) serveScript(w http.ResponseWriter, r *http.Request, sc script, works func(code string) bool, refusal string) {
	code := r.URL.Query().Get("code")
	if !works(code) {
		http.Error(w, sc.refuse(refusal), http.StatusUnauthorized)
		return
	}
	server := s.publicURL(r)
	var ca, fingerprint string
	if s.servedWithOwnCA(r, server) {
		ca, fingerprint = strings.TrimSpace(s.config.CA), s.caFingerprint()
	}
	w.Header().Set("Content-Type", sc.kind)
	w.Write([]byte(strings.NewReplacer("{{SERVER}}", sc.quote(server), "{{CODE}}", sc.quote(code),
		"{{CA}}", sc.quote(ca), "{{CA_FINGERPRINT}}", sc.quote(fingerprint)).Replace(sc.template)))
}

// commands are the two ways to run the script at path?code=code: curl
// piped to sh, and in PowerShell, irm piped to iex (which runs it as text,
// so no execution policy stops it). The page shows the one for your system.
// At an https base served with Agentry's own CA, whose SHA-256 is
// fingerprint, they are pinnedCommands instead; fingerprint is "" for a
// base that needs no CA of its own.
func commands(base, path, code, fingerprint string) map[string]any {
	sh := base + path + ".sh?code=" + code
	ps1 := base + path + ".ps1?code=" + code
	if fingerprint != "" {
		return pinnedCommands(base, sh, ps1, fingerprint)
	}
	return map[string]any{
		"command":        "curl -fsSL '" + sh + "' | sh",
		"windowsCommand": "irm '" + ps1 + "' | iex", // in PowerShell, as Claude Code's own install is
	}
}

// pinnedCommands run the script, which carries the code, only from a
// server whose CA is the one in the command. Each gets the CA first, with
// nothing secret and without trusting the connection, checks its SHA-256,
// and gets the script trusting that CA alone, never the system's. Both
// stay one line to paste, as the plain commands are: the check can't be
// moved into a script fetched first, as nothing vouches for that script.
func pinnedCommands(base, sh, ps1, fingerprint string) map[string]any {
	hash := strings.ToLower(strings.ReplaceAll(fingerprint, ":", "")) // as sha256sum writes it
	return map[string]any{
		// In sh, so it runs the same from bash, zsh or fish.
		"command": "sh -c " + shellQuoted(pinnedSh) + " agentry " + shellQuoted(base) + " " + shellQuoted(hash) + " " + shellQuoted(sh),
		// curl.exe comes with Windows 10 and later; PowerShell has no
		// download that trusts one CA alone in both 5.1 and 7.
		"windowsCommand": "& { " + pinnedPowerShell + " } " + powerShellQuoted(base) + " " + powerShellQuoted(hash) + " " + powerShellQuoted(ps1),
	}
}

// pinnedSh is pinnedCommands' sh, given the base ($1), the CA's SHA-256 ($2)
// and the script ($3). curl's --cacert takes PEM, so the checked DER is
// written as PEM in the same file. The script is saved first, not piped,
// so a failed download fails the command. With no tool to hash the CA it
// can't be checked, so nothing is sent.
var pinnedSh = strings.Join([]string{
	`c=$(mktemp) && p=$(mktemp) &&`,
	`curl -fsSk "$1/trust/ca.crt" -o "$c" &&`,
	`h=$({ sha256sum || shasum -a 256; } <"$c" 2>/dev/null | cut -c1-64) &&`, // sha256sum on Linux, shasum on macOS
	`if [ -z "$h" ]; then echo "` + noHashTool + `" >&2; false;`,
	`elif [ "$h" = "$2" ]; then`,
	`b=$(base64 <"$c" | tr -d "\n" | fold -w 64) &&`,
	`printf "%s\n" "-----BEGIN CERTIFICATE-----" "$b" "-----END CERTIFICATE-----" >"$c" &&`,
	`curl -fsSL --cacert "$c" "$3" -o "$p" && sh "$p";`,
	`else echo "` + otherCA("$1") + `" >&2; false; fi;`,
	`s=$?; rm -f "$c" "$p"; exit $s`,
}, " ")

// pinnedPowerShell is pinnedCommands' PowerShell, a script block given the
// base, the CA's SHA-256 and the script. -ne compares in any case;
// Schannel's revocation check fails for a CA that publishes no list.
var pinnedPowerShell = strings.Join([]string{
	`param($server, $fingerprint, $script)`,
	`$ca = (New-TemporaryFile).FullName;`,
	`try {`,
	`curl.exe -fsSk "$server/trust/ca.crt" -o $ca; if ($LASTEXITCODE) { return };`,
	`if ((Get-FileHash $ca -Algorithm SHA256).Hash -ne $fingerprint) { Write-Error "` + otherCA("$server") + `"; return };`,
	`Set-Content $ca ('-----BEGIN CERTIFICATE-----', [Convert]::ToBase64String([IO.File]::ReadAllBytes($ca), 'InsertLineBreaks'), '-----END CERTIFICATE-----');`,
	`$text = curl.exe -fsSL --cacert $ca --ssl-no-revoke $script | Out-String;`,
	`if ($LASTEXITCODE) { Write-Error "Couldn't get the script from $server, so nothing was run."; return };`,
	`$text | iex`,
	`} finally { Remove-Item $ca }`,
}, " ")

// noHashTool is what the pinned sh command says when it can't hash the CA,
// with no single quote, as pinnedSh has none. PowerShell's Get-FileHash is
// built in, so its command never needs it.
const noHashTool = "Cannot check the CA of the server: this machine has no sha256sum or shasum, so nothing was sent."

// otherCA is what the pinned commands say, in double quotes, when the CA at
// server (a variable of theirs) isn't the one they name.
func otherCA(server string) string {
	return server + " has another CA than the one this command trusts, so nothing was sent to it. " +
		"Check its fingerprint with agentry ca on the server."
}

// powerShellQuoted quotes a value for a PowerShell script: inside single
// quotes, a quote is doubled.
func powerShellQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// shellQuoted quotes a value for the install script. Codes and addresses
// have no quotes in them, but a Host header could.
func shellQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
