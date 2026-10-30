package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
)

// redacted is what redacting a secret answers: the stream and how many of
// its rows were rewritten, and, as for an erasure, what of the secret may
// be left on the disk for now.
type redacted struct {
	Stream   string    `json:"stream"`
	At       time.Time `json:"at"`
	Rows     int       `json:"rows"`
	Warnings []string  `json:"warnings"`
}

// stillRunning is the warning a redaction of the agents' knowledge gives
// while a turn that was running as it committed is still to end: the turn's
// end redacts the secret again from what the agent learned, but only this
// server remembers it, so a restart before then would let the turn write
// it back.
const stillRunning = "an agent whose turn is running wrote its knowledge file, secret and all, as the turn started: its end redacts it again, unless Agentry restarts first, so if it does, redact again after any turn that was running ends"

// previewRedaction is how many of a task's rows hold the text you marked,
// which the page shows you before you redact it (docs/design/log-growth.md
// section 5, "Removing a secret"); an archived task's read through its
// blob. It writes nothing. A refusal, such as no row holding the text, is
// answered 409 with its reason.
func (s *Server) previewRedaction(w http.ResponseWriter, r *http.Request) {
	asked, ok := secretOf(w, r)
	if !ok {
		return
	}
	id := r.PathValue("task")
	archived, ok := s.redactable(w, id)
	if !ok {
		return
	}
	rows, rejection, err := s.previewIn(id, asked.text, archived)
	answerPreview(w, rows, rejection, err)
}

// redact replaces the text you marked with [redacted] in every row of a
// task that holds it, an archived task's by unarchiving it, redacting it
// and archiving it again (redactIn). When the page sends the rows its
// preview counted, it is refused 409 unless that many hold the text as it
// commits. Only the page may ask: nothing undoes a redaction, so the desk
// can't. A new snapshot is written at once.
func (s *Server) redact(w http.ResponseWriter, r *http.Request) {
	asked, ok := secretOf(w, r)
	if !ok {
		return
	}
	id := r.PathValue("task")
	archived, ok := s.redactable(w, id)
	if !ok {
		return
	}
	s.answerRedaction(w, id, s.redactIn(id, asked, archived))
}

// previewAgentsRedaction is previewRedaction for the agents' stream: the
// rows of their roster and knowledge that hold the text you marked.
func (s *Server) previewAgentsRedaction(w http.ResponseWriter, r *http.Request) {
	asked, ok := secretOf(w, r)
	if !ok {
		return
	}
	rows, rejection, err := s.store.Preview(events.AgentsStream, asked.text)
	answerPreview(w, rows, rejection, err)
}

// redactAgents is redact for the agents' stream, their roster and
// knowledge (redactKnowledge).
func (s *Server) redactAgents(w http.ResponseWriter, r *http.Request) {
	asked, ok := secretOf(w, r)
	if !ok {
		return
	}
	s.answerRedaction(w, events.AgentsStream, s.redactIn(events.AgentsStream, asked, false))
}

// streamRedaction is what redacting a secret from one stream did: the
// redaction, once it commits, or its refusal or error, and the warning
// that a running turn may write the secret back.
type streamRedaction struct {
	redaction events.Event
	rejection *events.Rejection
	err       error
	warnings  []string
}

// previewIn is how many rows of stream hold text, an archived task's read
// through its blob, or why they can't be redacted.
func (s *Server) previewIn(stream, text string, archived bool) (int, *events.Rejection, error) {
	if archived {
		return s.store.PreviewArchived(stream, text, s.config.Blobs)
	}
	return s.store.Preview(stream, text)
}

// redactIn replaces the text asked with [redacted] in every row of stream
// that holds it, refused unless the rows asked hold it: an archived task's
// by unarchiving it, redacting it and archiving it again
// (Store.RedactArchivedCounted), and the agents' as redactKnowledge does.
// The cache of opened archives drops an archived task, as its old
// marker's entry still holds the text. The snapshot is the caller's to
// write.
func (s *Server) redactIn(stream string, asked redactionAsked, archived bool) streamRedaction {
	var done streamRedaction
	switch {
	case archived:
		done.redaction, done.rejection, done.err = s.store.RedactArchivedCounted(stream, asked.text, asked.rows, httpapi.You, asked.why, s.summarise(stream), s.config.Blobs)
		s.archives.forget(stream)
	case stream == events.AgentsStream:
		done = s.redactKnowledge(asked)
	default:
		done.redaction, done.rejection, done.err = s.store.RedactCounted(stream, asked.text, asked.rows, httpapi.You, asked.why)
	}
	return done
}

// redactKnowledge redacts the text asked from the agents' stream. A secret in an
// agent's knowledge is in every version after it was written, and
// Store.Redact rewrites each one that holds it as a full copy of its
// redacted text.
//
// An agent whose turn is running wrote its knowledge file as the turn
// started, secret and all, and the turn's end keeps what the file then
// holds as its new knowledge. So the secret is held, in memory only, for
// the turns running as the redaction commits, whose ends redact it again
// (hooks.Redactions); from before it commits, so a turn ending meanwhile
// is caught too.
func (s *Server) redactKnowledge(asked redactionAsked) streamRedaction {
	settle := s.config.Redactions.Hold(asked.text)
	var done streamRedaction
	done.redaction, done.rejection, done.err = s.store.RedactCounted(events.AgentsStream, asked.text, asked.rows, httpapi.You, asked.why)
	settle(done.redaction.Position != 0)
	if s.config.Redactions.Holds(asked.text) {
		done.warnings = append(done.warnings, stillRunning)
	}
	return done
}

// redactionAsked is what a redaction or its preview asks for: the text,
// kept as it is, as the secret is what you marked; the reason given; and
// the rows the preview counted, or store.AnyCount when none were sent.
type redactionAsked struct {
	text, why string
	rows      int
}

// secretOf is what a redaction or its preview asks for, or false once it
// has answered 400 for a body it can't read, a blank text or rows below
// one, as a redaction rewrites at least one.
func secretOf(w http.ResponseWriter, r *http.Request) (redactionAsked, bool) {
	var request struct {
		Text, Why string
		Rows      *int
	}
	if !httpapi.Decode(w, r, &request) {
		return redactionAsked{}, false
	}
	asked, ok := askedOf(w, request.Text, request.Why)
	if !ok {
		return redactionAsked{}, false
	}
	if request.Rows != nil {
		if *request.Rows < 1 {
			http.Error(w, "rows is how many rows the preview counted: 1 or more", http.StatusBadRequest)
			return redactionAsked{}, false
		}
		asked.rows = *request.Rows
	}
	return asked, true
}

// askedOf is a redaction of text, for why, of however many rows hold it,
// or false once it has answered 400 for a blank text.
func askedOf(w http.ResponseWriter, text, why string) (redactionAsked, bool) {
	if strings.TrimSpace(text) == "" {
		http.Error(w, "mark the text to redact", http.StatusBadRequest)
		return redactionAsked{}, false
	}
	return redactionAsked{text: text, why: strings.TrimSpace(why), rows: store.AnyCount}, true
}

// redactable is whether task id is archived, or false once it has answered
// that there is no such task (noTask) or that it can't be read.
func (s *Server) redactable(w http.ResponseWriter, id string) (archived, ok bool) {
	task, err := s.task(id)
	switch {
	case errors.Is(err, current.ErrArchived):
		return true, true
	case errors.Is(err, current.ErrErased), err == nil && task == nil:
		s.noTask(w, id)
		return false, false
	case err != nil:
		httpapi.Failed(w, err)
		return false, false
	}
	return false, true
}

// answerPreview answers a preview's count, its refusal 409 or its error
// 500.
func answerPreview(w http.ResponseWriter, rows int, rejection *events.Rejection, err error) {
	switch {
	case rejection != nil:
		http.Error(w, rejection.Reason, http.StatusConflict)
	case err != nil:
		httpapi.Failed(w, err)
	default:
		httpapi.Reply(w, map[string]int{"rows": rows})
	}
}

// answerRedaction answers a redaction of stream, as erase answers an
// erasure: a refusal 409 with its reason, and an error before the
// redaction commits 500. Once it commits, what fails after is a warning,
// with the redaction's own, and the snapshot is written again
// (writeOverOldText).
func (s *Server) answerRedaction(w http.ResponseWriter, stream string, done streamRedaction) {
	switch {
	case done.rejection != nil:
		http.Error(w, done.rejection.Reason, http.StatusConflict)
		return
	case done.redaction.Position == 0:
		httpapi.Failed(w, done.err)
		return
	}
	warnings := append([]string{}, done.warnings...)
	if done.err != nil {
		warnings = append(warnings, done.err.Error())
	}
	warnings = append(warnings, s.writeOverOldText()...)
	httpapi.Reply(w, redacted{Stream: stream, At: done.redaction.At, Rows: done.rows(), Warnings: warnings})
}

// rows is how many rows the redaction rewrote.
func (done streamRedaction) rows() int { return len(done.redaction.Numbers("positions")) }
