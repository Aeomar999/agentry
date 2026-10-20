package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
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
// while a running turn can write the secret back.
const stillRunning = "an agent whose turn is running wrote its knowledge file as the turn started, and may write the secret back as it ends: redact again after any turn that was running ends"

// previewRedaction is how many of a task's rows hold the text you marked,
// which the page shows you before you redact it (docs/design/log-growth.md
// section 5, "Removing a secret"); an archived task's read through its
// blob. It writes nothing. A refusal, such as no row holding the text, is
// answered 409 with its reason.
func (s *Server) previewRedaction(w http.ResponseWriter, r *http.Request) {
	text, _, ok := secretOf(w, r)
	if !ok {
		return
	}
	id := r.PathValue("task")
	archived, ok := s.redactable(w, id)
	if !ok {
		return
	}
	var rows int
	var rejection *events.Rejection
	var err error
	if archived {
		rows, rejection, err = s.store.PreviewArchived(id, text, s.config.Blobs)
	} else {
		rows, rejection, err = s.store.Preview(id, text)
	}
	answerPreview(w, rows, rejection, err)
}

// redact replaces the text you marked with [redacted] in every row of a
// task that holds it, an archived task's by unarchiving it, redacting it
// and archiving it again (Store.RedactArchived). Only the page may ask:
// nothing undoes a redaction, so the desk can't. The cache of opened
// archives drops the task, as its old marker's entry still holds the
// text, and a new snapshot is written at once.
func (s *Server) redact(w http.ResponseWriter, r *http.Request) {
	text, why, ok := secretOf(w, r)
	if !ok {
		return
	}
	id := r.PathValue("task")
	archived, ok := s.redactable(w, id)
	if !ok {
		return
	}
	var redaction events.Event
	var rejection *events.Rejection
	var err error
	if archived {
		redaction, rejection, err = s.store.RedactArchived(id, text, httpapi.You, why, s.summarise(id), s.config.Blobs)
		s.archives.forget(id)
	} else {
		redaction, rejection, err = s.store.Redact(id, text, httpapi.You, why)
	}
	s.answerRedaction(w, id, redaction, rejection, err)
}

// previewAgentsRedaction is previewRedaction for the agents' stream: the
// rows of their roster and knowledge that hold the text you marked.
func (s *Server) previewAgentsRedaction(w http.ResponseWriter, r *http.Request) {
	text, _, ok := secretOf(w, r)
	if !ok {
		return
	}
	rows, rejection, err := s.store.Preview(events.AgentsStream, text)
	answerPreview(w, rows, rejection, err)
}

// redactAgents is redact for the agents' stream. A secret in an agent's
// knowledge is in every version after it was written, and Store.Redact
// rewrites each one that holds it as a full copy of its redacted text.
//
// It can be undone for now, so the answer says so: an agent whose turn is
// running wrote its knowledge file as the turn started, secret and all,
// and the turn's end writes what the file then holds back as its new
// knowledge (step 10d2b2 of the design's plan stops that).
func (s *Server) redactAgents(w http.ResponseWriter, r *http.Request) {
	text, why, ok := secretOf(w, r)
	if !ok {
		return
	}
	redaction, rejection, err := s.store.Redact(events.AgentsStream, text, httpapi.You, why)
	s.answerRedaction(w, events.AgentsStream, redaction, rejection, err, stillRunning)
}

// secretOf is the text a redaction or its preview names and the reason
// given, or false once it has answered 400 for a body it can't read or a
// blank text. The text is kept as it is: the secret is what you marked.
func secretOf(w http.ResponseWriter, r *http.Request) (text, why string, ok bool) {
	var request struct{ Text, Why string }
	if !httpapi.Decode(w, r, &request) {
		return "", "", false
	}
	if strings.TrimSpace(request.Text) == "" {
		http.Error(w, "mark the text to redact", http.StatusBadRequest)
		return "", "", false
	}
	return request.Text, strings.TrimSpace(request.Why), true
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
// with those given, and the snapshot is written again (writeOverOldText).
func (s *Server) answerRedaction(w http.ResponseWriter, stream string, redaction events.Event, rejection *events.Rejection, err error, given ...string) {
	switch {
	case rejection != nil:
		http.Error(w, rejection.Reason, http.StatusConflict)
		return
	case redaction.Position == 0:
		httpapi.Failed(w, err)
		return
	}
	warnings := append([]string{}, given...)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	warnings = append(warnings, s.writeOverOldText()...)
	httpapi.Reply(w, redacted{Stream: stream, At: redaction.At, Rows: len(redaction.Numbers("positions")), Warnings: warnings})
}
