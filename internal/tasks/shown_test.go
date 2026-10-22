package tasks

import "testing"

// An epic reads as epic-N; a story of one and a task on its own keep task-N.
func TestShownID(t *testing.T) {
	for _, c := range []struct {
		id   string
		kind Kind
		want string
	}{
		{"task-21", Epic, "epic-21"},
		{"task-22", Story, "task-22"}, // a story of task-21: its kind is Story too
		{"task-30", Story, "task-30"},
	} {
		if got := ShownID(c.id, c.kind); got != c.want {
			t.Errorf("ShownID(%q, %q) = %q, want %q", c.id, c.kind, got, c.want)
		}
	}
	for id, want := range map[string]string{"task-21": "epic-21", "": "", "epic-3": "epic-3"} { // shown already: kept
		if got := ShownEpic(id); got != want {
			t.Errorf("ShownEpic(%q) = %q, want %q", id, got, want)
		}
	}
}

// You may name an epic either way; the API always gets task-N.
func TestStoredID(t *testing.T) {
	for id, want := range map[string]string{"epic-21": "task-21", "task-21": "task-21", "": "", "idea-3": "idea-3",
		// Odd names change only their prefix, if that: the API answers no such task.
		"epic-": "task-", "epic-abc": "task-abc", "EPIC-3": "EPIC-3"} {
		if got := StoredID(id); got != want {
			t.Errorf("StoredID(%q) = %q, want %q", id, got, want)
		}
	}
}
