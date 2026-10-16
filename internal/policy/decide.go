package policy

import (
	"path/filepath"
	"slices"
	"strings"
)

// Policy is a loaded, checked policy.toml.
type Policy struct {
	hash, text     string
	home           string
	protected      []string // Agentry's own files: no agent may touch them
	shellTools     []string
	defaultProfile string
	useForEveryone string
	profiles       map[string]*profile
}

type profile struct {
	name       string
	why        string
	otherwise  Decision
	unreadable Decision
	scratch    []glob // outside the workspace, but the agent's to use too
	rules      []rule
}

type rule struct {
	name, why string
	decision  Decision
	tools     []string
	runsAny   []glob
	runsOnly  []glob
	paths     []glob
	where     string
}

// Hash identifies this version of the policy in the log.
func (p *Policy) Hash() string { return p.hash }

// Text is the file as loaded.
func (p *Policy) Text() string { return p.text }

// ProfileNames lists the profiles, sorted.
func (p *Policy) ProfileNames() []string {
	names := make([]string, 0, len(p.profiles))
	for name := range p.profiles {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Decide judges a call made by an agent with the given profile ("" for the
// default). use_for_everyone, when set, wins over the agent's own profile.
func (p *Policy) Decide(agentProfile string, call Call) Verdict {
	verdict := p.decide(agentProfile, call)
	verdict.Policy = p.hash
	return verdict
}

func (p *Policy) decide(agentProfile string, call Call) Verdict {
	name := agentProfile
	if p.useForEveryone != "" {
		name = p.useForEveryone
	}
	if name == "" {
		name = p.defaultProfile
	}
	prof, ok := p.profiles[name]
	if !ok {
		return Verdict{Decision: Deny, Rule: "profile " + name + " isn't in policy.toml"}
	}

	facts := p.inspect(call)
	if p.touchesAgentry(call, facts) {
		return Verdict{Decision: Deny, Rule: "Agentry's own files are off limits",
			Why: "Built in: an agent that could edit the policy could allow itself anything."}
	}
	if facts.isShell && facts.shell.invalid {
		return Verdict{Decision: prof.unreadable, Profile: name, Rule: "Agentry couldn't read this shell command"}
	}
	for _, r := range prof.rules {
		if r.matches(call, facts, prof) {
			return Verdict{Decision: r.decision, Profile: name, Rule: r.name, Why: r.why}
		}
	}
	return Verdict{Decision: prof.otherwise, Profile: name, Rule: "otherwise", Why: prof.why}
}

// facts are what a call does, worked out once for all the rules.
type facts struct {
	isShell bool
	shell   shellCall
	paths   []string // absolute and cleaned
}

func (p *Policy) inspect(call Call) facts {
	f := facts{isShell: slices.Contains(p.shellTools, call.Tool)}
	var raw []string
	if f.isShell {
		f.shell = inspectShell(call.Input, call.Workspace, p.home)
		raw = f.shell.paths
	} else if call.Input != "" && !strings.Contains(call.Input, "://") {
		raw = []string{call.Input}
	}
	for _, path := range raw {
		f.paths = append(f.paths, p.absolute(path, call.Workspace))
	}
	return f
}

func (p *Policy) absolute(path, workspace string) string {
	switch {
	case path == "~" || strings.HasPrefix(path, "~/"):
		path = p.home + strings.TrimPrefix(path, "~")
	case !filepath.IsAbs(path):
		path = filepath.Join(workspace, path)
	}
	return filepath.Clean(path)
}

func (p *Policy) touchesAgentry(call Call, f facts) bool {
	if strings.Contains(call.Input, "AGENTRY_HOOK_TOKEN") {
		return true
	}
	for _, path := range f.paths {
		for _, protected := range p.protected {
			if path == protected || strings.HasPrefix(path, protected+"-") { // agentry.db-wal, agentry.db-shm
				return true
			}
		}
	}
	return false
}

// matches is true when every condition the rule has holds. A rule with no
// conditions matches every call.
func (r rule) matches(call Call, f facts, prof *profile) bool {
	if len(r.tools) > 0 && !slices.Contains(r.tools, call.Tool) {
		return false
	}
	if len(r.runsAny) > 0 && !(f.isShell && runsAny(r.runsAny, f.shell)) {
		return false
	}
	if len(r.runsOnly) > 0 && !(f.isShell && runsOnly(r.runsOnly, f.shell)) {
		return false
	}
	if len(r.paths) > 0 && !slices.ContainsFunc(f.paths, func(path string) bool { return anyMatches(r.paths, path) }) {
		return false
	}
	switch r.where {
	case "workspace":
		return staysInside(f, call.Workspace, prof.scratch)
	case "outside":
		return !staysInside(f, call.Workspace, prof.scratch)
	}
	return true
}

// runsAny: some command, or some pipeline, matches a pattern.
func runsAny(patterns []glob, s shellCall) bool {
	for _, text := range append(slices.Clone(s.commands), s.pipelines...) {
		if anyMatches(patterns, text) {
			return true
		}
	}
	return false
}

// runsOnly: every command matches a pattern, and nothing in it is unknown
// until it runs.
func runsOnly(patterns []glob, s shellCall) bool {
	if s.opaque || len(s.commands) == 0 {
		return false
	}
	for _, command := range s.commands {
		if !anyMatches(patterns, command) {
			return false
		}
	}
	return true
}

// staysInside: every path is in the workspace or a scratch place, and (for a
// shell command) nothing unknown could point elsewhere.
func staysInside(f facts, workspace string, scratch []glob) bool {
	if f.isShell && f.shell.opaque {
		return false
	}
	for _, path := range f.paths {
		inWorkspace := path == workspace || strings.HasPrefix(path, workspace+"/")
		if !inWorkspace && path != "/dev/null" && !anyMatches(scratch, path) {
			return false
		}
	}
	return true
}
