package rules

import (
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Archiving moves a done task's rows out of the live log into a blob and
// back (docs/design/log-growth.md section 4). Only the store writes the two
// events, and the catalogue lets no one else. What these rules can't see
// is left to the store methods that write them: the task's stream
// unchanged since the blob and the blob present (Store.Archive), and the
// marker undone being the stream's last (Store.Unarchive). The age is the
// archiver's to choose by: a task reopened since changes its stream.

// doneToArchive: only a done task is archived.
func doneToArchive(t *tasks.Task, _ events.Proposal) string {
	if !t.IsDone {
		return "the task isn't done, so it can't be archived"
	}
	return ""
}

// noTurnRunning: a task whose turn is still open waits for the turn to end.
// Its end would land on an archived task, refused, and a redaction can't
// hold a turn the state no longer shows, so what the turn learned would
// bring a redacted secret back.
func noTurnRunning(t *tasks.Task, _ events.Proposal) string {
	if t.OpenTurn() != nil {
		return "the task has a turn running, so it waits until the turn ends"
	}
	return ""
}

// aMarker says where the task's rows went: the blob, and the positions it
// covers, from the task's task.created at "created" to "through"; and what
// the task keeps, a summary the state can read, or its stream would break
// for good at the marker.
func aMarker(_ *tasks.Task, p events.Proposal) string {
	if _, err := tasks.ReadSummary(p.Data["summary"]); err != nil {
		return "an archive keeps the task's summary: " + err.Error()
	}
	switch {
	case !isSHA256(p.Text("archive")):
		return "not an archive's hash"
	case p.Number("created") < 1 || p.Number("through") < p.Number("created"):
		return "an archive covers the task's rows from its task.created to its \"through\""
	case p.Number("count") < 1:
		return "an archive holds the task's rows, its task.created at least"
	}
	return ""
}

// anArchivedTask: a task the state holds whole isn't archived.
func anArchivedTask(t *tasks.Task, _ events.Proposal) string {
	return "task " + t.ID + " isn't archived"
}

// anUnarchiving names the marker it undoes and that marker's blob.
func anUnarchiving(p events.Proposal) string {
	switch {
	case !isSHA256(p.Text("archive")):
		return "not an archive's hash"
	case p.Number("marker") < 1:
		return "an unarchiving names the position of the marker it undoes"
	}
	return ""
}
