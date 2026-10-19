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
	name, why       string
	decision        Decision
	tools           []string
	runsAny         []glob
	runsOnly        []glob
	paths           []glob
	where           string
	redirectsToFile bool   // a redirect writes a file other than /dev/null and scratch
	runsOutside     []glob // runners that ask when they may load code from outside the workspace
}

// Hash identifies this version of the policy in the log.
func (p *Policy) Hash() string { return p.hash }

// Text is the file as loaded.
func (p *Policy) Text() string { return p.text }

// DefaultProfile is the profile of an agent that doesn't name one.
func (p *Policy) DefaultProfile() string { return p.defaultProfile }

// UseForEveryone is the profile every agent uses whatever its own, or "".
func (p *Policy) UseForEveryone() string { return p.useForEveryone }

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
	return p.DecideAs(p.EffectiveProfile(agentProfile).Profile, call)
}

// DecideAs judges a call as the named profile would, whatever
// use_for_everyone says: what an agent with it would get once that is off.
func (p *Policy) DecideAs(profile string, call Call) Verdict {
	verdict := p.decide(profile, call)
	verdict.Policy = p.hash
	return verdict
}

func (p *Policy) decide(name string, call Call) Verdict {
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
	for i, r := range prof.rules {
		if r.matches(call, facts, prof) {
			return Verdict{Decision: r.decision, Profile: name, Rule: r.name, Why: r.why, Order: i + 1}
		}
	}
	return Verdict{Decision: prof.otherwise, Profile: name, Rule: "otherwise", Why: prof.why}
}

// facts are what a call does, worked out once for all the rules.
type facts struct {
	isShell bool
	shell   shellCall
	paths   []string // absolute and cleaned
	writes  []string // redirect targets, absolute and cleaned
	// some path leads back into the file system through a process's
	// working directory, root or open files
	loopsBack bool
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
		if below, inStack := inFolderStack(path); inStack {
			f.loopsBack = f.loopsBack || below
			continue
		}
		expanded := p.expanded(path, call.Workspace)
		f.loopsBack = f.loopsBack || loopsBack(expanded)
		f.paths = append(f.paths, filepath.Clean(expanded))
	}
	for _, path := range f.shell.writes {
		f.writes = append(f.writes, filepath.Clean(p.expanded(path, call.Workspace)))
	}
	return f
}

// expanded is path made absolute, its ~ read as the shell reads it, but not
// cleaned: through a process's working directory, ".." leaves that
// directory. Folders of the shell's stack (~-) are left to inFolderStack.
func (p *Policy) expanded(path, workspace string) string {
	switch {
	case path == "~" || strings.HasPrefix(path, "~/"):
		return p.home + strings.TrimPrefix(path, "~")
	case path == "~+" || strings.HasPrefix(path, "~+/"): // the working directory
		return workspace + strings.TrimPrefix(path, "~+")
	case strings.HasPrefix(path, "~"): // ~dev/x is dev's home, beside this one
		return filepath.Dir(p.home) + "/" + strings.TrimPrefix(path, "~")
	case !filepath.IsAbs(path):
		return workspace + "/" + path
	}
	return path
}

// inFolderStack: path starts at a folder of the shell's stack, which can't
// be known: ~- (the folder it was in before), ~N, ~+N or ~-N. below says it
// goes on past that folder, so it may lead anywhere, as /proc/self/cwd does.
func inFolderStack(path string) (below, inStack bool) {
	first, rest, _ := strings.Cut(path, "/")
	name, isTilde := strings.CutPrefix(first, "~")
	if !isTilde {
		return false, false
	}
	number := name
	if strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") {
		number = name[1:]
	}
	if name != "-" && (number == "" || strings.Trim(number, "0123456789") != "") {
		return false, false
	}
	return rest != "", true
}

func (p *Policy) touchesAgentry(call Call, f facts) bool {
	if strings.Contains(call.Input, "AGENTRY_HOOK_TOKEN") || f.loopsBack {
		return true
	}
	if f.isShell && slices.ContainsFunc(f.shell.named, p.isProtectedName) {
		return true
	}
	// A program that may read folders whole reaches Agentry's files through a
	// folder that holds them. It counts for the whole call, as a pipe can
	// hand one program's paths to another: echo ~ | xargs tar c.
	recursive := call.Tool == "Grep" || f.isShell && f.shell.recursive
	for _, path := range f.paths {
		for _, protected := range p.protected {
			if mayReach(path, protected, recursive) {
				return true
			}
		}
	}
	return false
}

// patternCharacters removes the characters that make a name a pattern.
var patternCharacters = strings.NewReplacer("*", "", "?", "", "[", "", "]", "")

// isProtectedName: name, spelt out by a path that can't be fully known, may
// be one of Agentry's files or the folder that holds them, without case. It
// is a best-effort rule: such a path can still reach them without spelling a
// name out ($X alone), which only the rules that ask about unknown commands
// cover. A name with at most one character besides * ? [ ] (*, c*, .*) isn't
// spelt out: after a folder that can't be known, it may be anything there,
// just as $X alone may, and refusing it would refuse ls $d/c*. One with more
// text (tl*, ca.ke?, agentry.d[b]) is matched as a pattern.
func (p *Policy) isProtectedName(name string) bool {
	if len(patternCharacters.Replace(name)) <= 1 {
		return false
	}
	for _, protected := range p.protected {
		if nameMayBe(name, filepath.Base(protected), true) || nameMayBe(name, filepath.Base(filepath.Dir(protected)), true) {
			return true
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
	if len(r.runsOutside) > 0 && !(f.isShell && runsOutside(r.runsOutside, f.shell, call.Workspace)) {
		return false
	}
	if r.redirectsToFile && !slices.ContainsFunc(f.writes, func(path string) bool { return writesAFile(path, prof.scratch) }) {
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

// runsOutside: a command matching a pattern may start outside the workspace,
// in a folder that can't be known, or names a file outside it. The scratch
// places count as outside here: an agent may write a test or a config there,
// and a runner started there, or pointed at it, would run it.
func runsOutside(patterns []glob, s shellCall, workspace string) bool {
	for _, run := range s.runs {
		if !slices.ContainsFunc(run.texts, func(text string) bool { return anyMatches(patterns, text) }) {
			continue
		}
		if run.dirs == nil || slices.ContainsFunc(slices.Concat(run.dirs, run.files), func(path string) bool {
			return !inWorkspace(path, workspace)
		}) {
			return true
		}
	}
	return false
}

func inWorkspace(path, workspace string) bool {
	return path == workspace || strings.HasPrefix(path, workspace+"/")
}

// writesAFile: writing to path changes a file someone keeps. /dev/null keeps
// nothing, and the profile's scratch places are the agent's own.
func writesAFile(path string, scratch []glob) bool {
	return path != "/dev/null" && !anyMatches(scratch, path)
}

// staysInside: every path is in the workspace or a scratch place, and (for a
// shell command) nothing unknown could point elsewhere.
func staysInside(f facts, workspace string, scratch []glob) bool {
	if f.isShell && f.shell.opaque {
		return false
	}
	for _, path := range f.paths {
		if !inWorkspace(path, workspace) && path != "/dev/null" && !anyMatches(scratch, path) {
			return false
		}
	}
	return true
}
