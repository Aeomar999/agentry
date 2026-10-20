package web

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// erased is what erasing a task answers: when, and what of it may be left
// on the disk for now, as the steps after the erasure itself can fail.
type erased struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	Warnings []string  `json:"warnings"`
}

// erase erases a task for good, at your request, with what Agentry keeps
// of its words elsewhere (docs/design/log-growth.md section 5, "Removing a
// whole task"). Only the page may ask: nothing undoes an erasure, so the
// desk, which acts on what Claude makes of your words, can't.
//
// A live task's session is stopped first, while the state still says
// where it runs: an idle one too, as a turn's session lives on after its
// turn.ended until the loop stops it, and the loop leaves an erased task
// alone. A turn that starts between the stop and the erasure's read of the
// rows is still orphaned: the log refuses what it writes, but its process
// and worktree go on. An archived task is erased as it is, from its blob. A refusal is
// answered 409 with its reason, and an error before the erasure commits
// 500; neither has written anything. Once it commits the task is erased,
// so what fails after that is a warning in the answer: a blob that won't
// delete, a copy of its words that can't be redacted, or a snapshot that
// can't be written.
func (s *Server) erase(w http.ResponseWriter, r *http.Request) {
	var request struct{ Why string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	id := r.PathValue("task")
	task, err := s.task(id)
	if err == nil && task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return
	}
	if refused, err := s.leaveEpic(id); refused != "" {
		http.Error(w, refused, http.StatusConflict)
		return
	} else if err != nil {
		httpapi.Failed(w, err)
		return
	}
	if task != nil && s.config.StopSession != nil {
		s.config.StopSession(id)
	}
	shared, unread := s.sharedFilesOf(id)
	erasure, rejection, err := s.store.Erase(id, httpapi.You, strings.TrimSpace(request.Why), s.config.Blobs)
	switch {
	case rejection != nil:
		http.Error(w, rejection.Reason, http.StatusConflict)
		return
	case erasure.Position == 0:
		httpapi.Failed(w, err)
		return
	}
	s.archives.forget(id)
	warnings := []string{}
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	if unread != nil {
		warnings = append(warnings, unread.Error())
	}
	elsewhere, err := s.elsewhereOf(id)
	if err != nil {
		warnings = append(warnings, "the other tasks can't all be read, so its shared files are kept and its words may stay in them: "+err.Error())
	} else {
		warnings = append(warnings, s.removeSharedFiles(shared, elsewhere.files)...)
	}
	warnings = append(warnings, s.redactCopies(id, elsewhere.copies)...)
	warnings = append(warnings, s.writeOverOldText()...)
	httpapi.Reply(w, erased{ID: id, At: erasure.At, Warnings: warnings})
}

// leaveEpic takes a story out of its epic before it is erased, with the
// epic's task.story_removed, which names only its id: once erased, the
// story can't be loaded for the rule itsStory, and the epic would wait on
// it for ever. A live story writes its task.epic_left too, as fileUnder
// does, so if the erasure then fails it is out of any epic, and you may
// file it under one again; an archived one still names its epic, which
// fileUnder lets go of once it is unarchived. An archived epic is done
// and is left as it is: unarchiving it for the removal would leave its
// blob holding the story's words, which redactCopies takes out only of an
// epic still archived. It returns why the removal was refused, or the
// error that stopped it.
func (s *Server) leaveEpic(id string) (string, error) {
	summary, found := s.state.Summary(id)
	if !found || summary.Epic == "" {
		return "", nil
	}
	epic, err := s.task(summary.Epic)
	switch {
	case errors.Is(err, current.ErrArchived), errors.Is(err, current.ErrErased):
		return "", nil
	case err != nil:
		return "", err
	case epic == nil || !slices.Contains(epic.Stories, id):
		return "", nil
	}
	leaving := []events.Proposal{{Stream: epic.ID, Actor: httpapi.You, Type: events.StoryRemoved, Data: map[string]any{"story": id}}}
	if story, err := s.task(id); err == nil && story != nil {
		leaving = append(leaving, events.Proposal{Stream: id, Actor: httpapi.You, Type: events.EpicLeft, Data: map[string]any{"epic": epic.ID}})
	}
	for _, p := range leaving {
		if _, rejection, err := s.store.Propose(p); rejection != nil {
			return rejection.Reason, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", nil
}

// sharedFilesOf is the blobs of the files task id shared, read before it
// is erased, an archived task's from its blob.
func (s *Server) sharedFilesOf(id string) ([]string, error) {
	history, err := s.store.History(id, store.FromBlobs(s.config.Blobs))
	if err != nil {
		return nil, fmt.Errorf("its shared files can't be found, so their blobs are kept: %w", err)
	}
	var hashes []string
	for _, e := range history {
		for _, hash := range tasks.FileHashes(e) {
			if !slices.Contains(hashes, hash) {
				hashes = append(hashes, hash)
			}
		}
	}
	return hashes, nil
}

// elsewhere is what the other tasks hold of an erased one.
type elsewhere struct {
	files  map[string]bool     // the blobs of shared files their events name
	copies map[string][]copied // its words, by the stream of each task that keeps a copy
}

// copied is words of an erased task that another task keeps, and the
// rows of it that keep them: only those are redacted, as the same words,
// such as a short title, may be the other task's own in its other rows.
type copied struct {
	text string
	rows []int64
}

// keep adds text, kept by the row at position, to copies.
func keep(copies []copied, text string, position int64) []copied {
	i := slices.IndexFunc(copies, func(c copied) bool { return c.text == text })
	if i < 0 {
		return append(copies, copied{text: text, rows: []int64{position}})
	}
	if !slices.Contains(copies[i].rows, position) {
		copies[i].rows = append(copies[i].rows, position)
	}
	return copies
}

// elsewhereOf reads every other task's whole record, an archived task's
// from its blob, for what it holds of task id: the shared files it names,
// which may be id's too, and the copies of id's words (copiesOf), with
// the rows that keep them. Every
// task is read, not just its epic and asker as they are now, as a story
// taken out of its epic leaves the copies behind.
func (s *Server) elsewhereOf(id string) (elsewhere, error) {
	found := elsewhere{files: map[string]bool{}, copies: map[string][]copied{}}
	for _, other := range s.state.TaskIDs() {
		if other == id {
			continue
		}
		history, err := s.store.History(other, store.FromBlobs(s.config.Blobs))
		if err != nil {
			return found, err
		}
		for _, e := range history {
			for _, hash := range tasks.FileHashes(e) {
				found.files[hash] = true
			}
			for _, text := range copiesOf(id, e) {
				if text != "" {
					found.copies[other] = keep(found.copies[other], text, e.Position)
				}
			}
		}
	}
	return found, nil
}

// copiesOf are the words of task id an event of another task keeps: its
// summary in its epic's task.story_done and its asker's
// task.request_answered, and its title and summary in the task.story_added
// of an epic you added it to. The epic's task.split and the asker's
// task.requested are left: the epic and the asker wrote those words first.
func copiesOf(id string, e events.Event) []string {
	switch {
	case e.Type == events.StoryDone && e.Text("story") == id, e.Type == events.RequestAnswered && e.Text("child") == id:
		return []string{e.Text("summary")}
	case e.Type == events.StoryAdded && e.Text("story") == id:
		return []string{e.Text("title"), e.Text("summary")}
	}
	return nil
}

// removeSharedFiles deletes the blobs of an erased task's shared files
// that no other task's event names, and says which won't delete. Only
// erasing deletes a shared file's blob (sharedfiles.Disk.Remove).
func (s *Server) removeSharedFiles(shared []string, named map[string]bool) []string {
	var warnings []string
	for _, hash := range shared {
		if named[hash] {
			continue
		}
		if err := s.config.Blobs.Remove(hash); err != nil {
			warnings = append(warnings, "the shared file "+hash+" can't be deleted yet: "+err.Error())
		}
	}
	return warnings
}

// redactCopies redacts each copy of an erased task's words in the rows of
// the task that keep it, an archived one through its blob, and says which
// can't be: the erasure stands all the same.
func (s *Server) redactCopies(id string, copies map[string][]copied) []string {
	var warnings []string
	why := "The copy of erased " + id + "."
	for _, stream := range slices.Sorted(maps.Keys(copies)) {
		for _, kept := range longestFirst(copies[stream]) {
			var rejection *events.Rejection
			var err error
			if _, archived := s.task(stream); errors.Is(archived, current.ErrArchived) {
				_, rejection, err = s.store.RedactArchivedRows(stream, kept.text, kept.rows, httpapi.You, why, s.summarise(stream), s.config.Blobs)
			} else {
				_, rejection, err = s.store.RedactRows(stream, kept.text, kept.rows, httpapi.You, why)
			}
			switch {
			case rejection != nil:
				warnings = append(warnings, "its words in "+stream+" can't be redacted: "+rejection.Reason)
			case err != nil:
				warnings = append(warnings, "its words in "+stream+": "+err.Error())
			}
		}
	}
	return warnings
}

// longestFirst orders copies longest first, the same lengths by their
// text: a copy that holds another, such as a summary that starts with the
// title, is redacted whole before the shorter one could leave part of it.
func longestFirst(copies []copied) []copied {
	return slices.SortedFunc(slices.Values(copies), func(a, b copied) int {
		return cmp.Or(cmp.Compare(len(b.text), len(a.text)), strings.Compare(a.text, b.text))
	})
}

// summarise is the summary an archived task unarchived to redact it is
// archived again with: the list's, or nil when the state can't hand it
// out, which the rules refuse.
func (s *Server) summarise(id string) func() any {
	return func() any {
		if summary, found := s.state.Summary(id); found {
			return summary
		}
		return nil
	}
}

// writeOverOldText gives the database's free pages back and empties its
// WAL again, in case a reader held them while a step ran, then writes a
// new snapshot at once: the one on disk still holds the erased task.
func (s *Server) writeOverOldText() []string {
	var warnings []string
	if err := s.store.GiveBackSpace(); err != nil {
		warnings = append(warnings, "the old text may stay in the database's free space for now: "+err.Error())
	}
	if s.config.Snapshots != nil {
		if err := s.config.Snapshots.WriteNow(); err != nil {
			warnings = append(warnings, "the snapshot can't be written, so the old one may still hold it: "+err.Error())
		}
	}
	return warnings
}

// gone answers that an erased task is gone for good, and when it went,
// rather than that there was never such a task. It is false for a task
// that isn't erased.
func (s *Server) gone(w http.ResponseWriter, id string) bool {
	erasure, erased := s.state.Erased(id)
	if erased {
		http.Error(w, id+" was erased on "+erasure.At.Format("2 January 2006"), http.StatusGone)
	}
	return erased
}
