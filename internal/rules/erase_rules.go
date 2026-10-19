package rules

import "gitlab.com/dev.chempa/agentry/internal/events"

// Erasing deletes a task's rows for good (docs/design/log-growth.md section
// 5). Only the store writes task.erased, for you: the catalogue lets no
// other kind of actor write it. It goes onto any task the log has, whole,
// archived or broken, but one erased already, and Check refuses every
// event after it, task.unarchived and task.reopened too: nothing reverses
// it. What these rules can't see is left to Store.Erase: the stream
// unchanged since its rows were read, so "through" and "count" are its
// rows, and the blobs named its archives.

// anErasure says what it deletes: the task's rows from its task.created at
// "created" to "through", and the blobs that held them when it was
// archived.
func anErasure(p events.Proposal) string {
	switch {
	case p.Number("created") < 1 || p.Number("through") < p.Number("created"):
		return "an erasure covers the task's rows from its task.created to its \"through\""
	case p.Number("count") < 1:
		return "an erasure deletes the task's rows, its task.created at least"
	}
	for _, hash := range p.List("archives") {
		if !isSHA256(hash) {
			return "not an archive's hash: " + hash
		}
	}
	return ""
}

// ErasedRefusal is why an erased task takes no event, for the rules and
// for a route that refuses one before proposing anything.
func ErasedRefusal(stream string) string {
	return stream + " is erased: an erased task takes no events"
}
