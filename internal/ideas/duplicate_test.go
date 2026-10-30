package ideas_test

import (
	"errors"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
)

// taskList is a fake of the tasks an idea is checked against, in order; a nil
// one can't be read.
type taskList []*tasks.Task

func (b taskList) TaskIDs() []string {
	var ids []string
	for _, task := range b {
		if task == nil {
			ids = append(ids, "task-0") // which Task can't find
			continue
		}
		ids = append(ids, task.ID)
	}
	return ids
}

func (b taskList) Task(id string) (*tasks.Task, error) {
	for _, task := range b {
		if task != nil && task.ID == id {
			return task, nil
		}
	}
	return nil, errors.New("broken")
}

// The real repeats on the board: task-41 made task-38 again in other words,
// while task-39 is another feature that shares two of its words.
func TestAnIdeaLooksLikeAnOpenTaskWithACloseTitle(t *testing.T) {
	const (
		task38 = "Make a ship the user chose actually push, and count it shipped only after the push"
		task41 = "Make a ship the user chose push, and count it shipped only when it pushed"
		task39 = "Word a waiting push plainly, and grey out Shipping for teams that can't ship"
	)
	for _, c := range []struct {
		task, idea string
		like       bool
	}{
		{task38, task41, true},
		{task41, task38, true},
		{task38, task38, true},
		{"Search the board", "search the Board!", true},
		{"Search the board", "Search the whole board", true},
		{task41, task39, false},
		{task38, task39, false},
		{"Search the board", "Dark mode", false},
		{"Fix login", "Fix logout", false},
	} {
		open := taskList{{ID: "task-1", Title: c.task, Repo: "/repo"}}
		got := ideas.OpenTaskLike(open, &ideas.Idea{ID: "idea-1", Title: c.idea, Repo: "/repo"}) != nil
		if got != c.like {
			t.Errorf("%q looks like %q: %v, want %v", c.idea, c.task, got, c.like)
		}
	}
}

// Only an open task in the idea's repo counts, and not one made from the
// idea itself; of several, the closest, then the earliest made.
func TestOnlyAnOpenTaskInTheReposCounts(t *testing.T) {
	idea := &ideas.Idea{ID: "idea-1", Title: "Search the board", Repo: "/repo"}
	now := time.Now()
	for _, c := range []struct {
		name  string
		tasks taskList
		want  string
	}{
		{"done", taskList{{ID: "task-1", Title: "Search the board", Repo: "/repo", IsDone: true}}, ""},
		{"another repo", taskList{{ID: "task-1", Title: "Search the board", Repo: "/other"}}, ""},
		{"made from the idea", taskList{{ID: "task-1", Title: "Search the board", Repo: "/repo", MadeFrom: "idea-1"}}, ""},
		{"in the Backlog", taskList{{ID: "task-1", Title: "Search the board", Repo: "/repo", Parked: "backlog"}}, "task-1"},
		{"broken ones skipped", taskList{nil, {ID: "task-1", Title: "Search the board", Repo: "/repo"}}, "task-1"},
		{"the closest", taskList{{ID: "task-1", Title: "Search the whole board", Repo: "/repo"}, {ID: "task-2", Title: "Search the board", Repo: "/repo"}}, "task-2"},
		{"then the earliest", taskList{{ID: "task-2", Title: "Search the board", Repo: "/repo", CreatedAt: now},
			{ID: "task-1", Title: "search the board", Repo: "/repo", CreatedAt: now.Add(-time.Hour)}}, "task-1"},
	} {
		got := ""
		if like := ideas.OpenTaskLike(c.tasks, idea); like != nil {
			got = like.ID
		}
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
