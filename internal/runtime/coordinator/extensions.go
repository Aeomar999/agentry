package coordinator

import (
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// An extension takes part in the loop through its Coordinator hook, for the
// coordinators its For says: its Brief adds sections to their briefs, its
// Decision says what their done may carry, its Check may refuse a done
// whose Field it can't read, and its OnDone acts on that Field. A done carrying a field no extension of its coordinator reads
// still goes through, and its summary says the field wasn't kept. Each hook
// runs under recover, so an extension's panic costs only its own part.

// hooksFor are the extensions whose coordinator hook is for coordinator, in
// state, in list's order. An extension whose For panics is left out.
func hooksFor(list []extensions.Extension, coordinator agents.Agent, state extensions.Reader) []extensions.Extension {
	var hooks []extensions.Extension
	for _, f := range list {
		if f.Coordinator == nil {
			continue
		}
		var matches bool
		if guarded(f, "For", func() { matches = f.Coordinator.For(coordinator, state) }) == "" && matches {
			hooks = append(hooks, f)
		}
	}
	return hooks
}

// briefSections are the sections hooks add to a brief. An extension whose
// Brief panics adds none.
func briefSections(hooks []extensions.Extension, state extensions.Reader, task *tasks.Task) []extensions.Section {
	var sections []extensions.Section
	for _, f := range hooks {
		if f.Coordinator.Brief == nil {
			continue
		}
		var more []extensions.Section
		if guarded(f, "Brief", func() { more = f.Coordinator.Brief(state, task) }) == "" {
			sections = append(sections, more...)
		}
	}
	return sections
}

// unreadable says why a done decision can't be applied: a field that a
// extension for the task's coordinator reads, and whose Check refuses. The
// coordinator is told, as of any decision it got wrong, and sends it again.
// It is "" for any other decision, and when every Check passes. A Check
// that panics passes, so one extension's bug can't hold up every done.
func (l *Loop) unreadable(task *tasks.Task, roster agents.Roster, decision Decision) string {
	if decision.Action != Finish {
		return ""
	}
	own, _ := roster.Get(task.Coordinator)
	for _, f := range hooksFor(l.Extensions, own, l.state) {
		hook := f.Coordinator
		field := decision.field(hook.Field)
		if hook.Check == nil || field == nil {
			continue
		}
		var err error
		if guarded(f, "Check", func() { err = hook.Check(field) }) == "" && err != nil {
			return fmt.Sprintf("the coordinator's reply had no usable decision: Agentry couldn't read its %q: %v", hook.Field, err)
		}
	}
	return ""
}

// onDone acts on the fields of a done decision that extensions read: each
// extension for the task's coordinator acts on its own, and a field only
// other coordinators' extensions read is reported, not kept. It returns the
// notes for the summary, "" for none. A panic in a hook becomes a note, and
// the done goes through.
func (l *Loop) onDone(task *tasks.Task, roster agents.Roster, decision Decision) string {
	own, _ := roster.Get(task.Coordinator)
	hooks := hooksFor(l.Extensions, own, l.state)
	var notes, read, unread []string
	for _, f := range hooks {
		hook := f.Coordinator
		field := decision.field(hook.Field)
		if hook.OnDone == nil || field == nil {
			continue
		}
		read = append(read, hook.Field)
		core := extensions.Core{Log: l.logOf(f), Events: l.events, Read: l.state, Defaults: l.Defaults, Settings: l.settingsOf(f)}
		var note string
		if failed := guarded(f, "OnDone", func() { note = hook.OnDone(core, task, field) }); failed != "" {
			note = fmt.Sprintf("Agentry couldn't keep %q: %s", hook.Field, failed)
		}
		if note != "" {
			notes = append(notes, note)
		}
	}
	for _, f := range l.Extensions {
		name := ""
		if f.Coordinator != nil {
			name = f.Coordinator.Field
		}
		if name != "" && decision.field(name) != nil && !slices.Contains(read, name) && !slices.Contains(unread, name) {
			unread = append(unread, name)
			notes = append(notes, fmt.Sprintf("Agentry couldn't keep %q, as no feature of this coordinator reads it.", name))
		}
	}
	return strings.Join(notes, "\n\n")
}

// logOf is f's checked proposer. An extension the loop was given none for
// writes nothing: everything it proposes is refused.
func (l *Loop) logOf(f extensions.Extension) extensions.Proposer {
	if log, found := l.Logs[f.Name]; found {
		return log
	}
	return extensions.Unwritable(f.Name)
}

// settingsOf is f's own settings, none when the loop was given no settings
// file.
func (l *Loop) settingsOf(f extensions.Extension) extensions.Settings {
	if l.Settings == nil {
		return extensions.Settings{}
	}
	return l.Settings(f.Name)
}

// guarded runs one of f's hooks, a panic in it turned into why it failed:
// "" when it didn't.
func guarded(f extensions.Extension, hook string, run func()) (failed string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("coordinator loop: an extension's hook panicked", "extension", f.Name, "hook", hook, "panic", r,
				"stack", string(debug.Stack()))
			failed = fmt.Sprintf("the %s feature's %s failed: %v", f.Name, hook, r)
		}
	}()
	run()
	return ""
}
