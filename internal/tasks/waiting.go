package tasks

import "strings"

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

// plainPush reads the branch a lone `git push` goes to: "" when no single
// branch can be read, and not ok when the command is anything else. A
// command that also does more (`&&`, a pipe, a redirect), deletes a branch
// or pushes many refs at once isn't plain: its whole text stays in view.
func plainPush(command string) (branch string, ok bool) {
	if strings.ContainsAny(command, ";&|<>`$\n") {
		return "", false
	}
	words := strings.Fields(command)
	if len(words) == 0 || words[0] != "git" {
		return "", false
	}
	words = words[1:]
	for len(words) > 0 && strings.HasPrefix(words[0], "-") { // git -C dir, -c key=value
		if words[0] == "-C" || words[0] == "-c" {
			words = words[1:]
		}
		words = words[1:]
	}
	if len(words) == 0 || words[0] != "push" {
		return "", false
	}
	var places []string // the remote, then the refspecs
	for args := words[1:]; len(args) > 0; args = args[1:] {
		switch arg := args[0]; {
		case arg == "-d" || arg == "--delete" || arg == "--mirror" || arg == "--all" || arg == "--branches" || arg == "--prune":
			return "", false
		case arg == "-o" || arg == "--push-option" || arg == "--repo" || arg == "--receive-pack" || arg == "--exec":
			if len(args) > 1 {
				args = args[1:] // the option's value
			}
		case !strings.HasPrefix(arg, "-"):
			places = append(places, arg)
		}
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
	if dst == "HEAD" || dst == "" {
		return "", true
	}
	return strings.TrimPrefix(dst, "refs/heads/"), true
}
