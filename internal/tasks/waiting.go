package tasks

import (
	"regexp"
	"strings"
)

// WaitingLine words a command held for you to approve: a push as the branch
// it goes to, anything else as the agent asking to run it.
func WaitingLine(agent, command string) string {
	push, ok := plainPush(command)
	switch {
	case !ok:
		return agent + " asks to run: " + command
	case push == "":
		return "Waiting for you to approve a push"
	}
	return "Waiting for you to approve the push to " + push
}

// remoteName is a remote named in the repo's config, not a URL or a path.
var remoteName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// plainPush reads the branch a lone `git push` goes to: "" when no single
// branch can be read, and not ok when the command is anything else. Only a
// push it fully understands is plain: any option, quote or remote it doesn't
// know could delete a branch or run code, so its whole text stays in view.
func plainPush(command string) (branch string, ok bool) {
	if strings.ContainsAny(command, ";&|<>`$\n'\"") {
		return "", false
	}
	words := strings.Fields(command)
	if len(words) == 0 || words[0] != "git" {
		return "", false
	}
	words = words[1:]
	for len(words) > 0 && words[0] == "-C" { // git -C dir
		if len(words) < 2 {
			return "", false
		}
		words = words[2:]
	}
	if len(words) == 0 || words[0] != "push" {
		return "", false
	}
	var places []string // the remote, then the refspecs
	for args := words[1:]; len(args) > 0; args = args[1:] {
		arg := args[0]
		switch {
		case arg == "-o" || arg == "--push-option":
			if len(args) < 2 {
				return "", false
			}
			args = args[1:] // the option's value
		case plainPushOption(arg):
		case strings.HasPrefix(arg, "-"):
			return "", false
		default:
			places = append(places, arg)
		}
	}
	if len(places) > 0 && !remoteName.MatchString(places[0]) {
		return "", false
	}
	if len(places) != 2 {
		return "", true
	}
	refspec := strings.TrimPrefix(places[1], "+")
	src, dst, mapped := strings.Cut(refspec, ":")
	if !mapped {
		dst = src
	}
	if mapped && src == "" { // ":branch" deletes it
		return "", false
	}
	if dst == "HEAD" || dst == "" || strings.Contains(dst, "*") {
		return "", true
	}
	return strings.TrimPrefix(dst, "refs/heads/"), true
}

// plainPushOption is a push option that only changes how a branch is pushed,
// never which refs or where. Git accepts bundled short flags and shortened
// long options, so anything not spelled exactly as here isn't plain.
func plainPushOption(arg string) bool {
	switch arg {
	case "-u", "--set-upstream", "-f", "--force", "--force-with-lease", "--force-if-includes",
		"--atomic", "--follow-tags", "-q", "-v", "-n", "--dry-run":
		return true
	}
	return strings.HasPrefix(arg, "--force-with-lease=") || strings.HasPrefix(arg, "--push-option=")
}
