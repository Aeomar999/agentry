package web

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Redacting a secret across the whole log, for one pasted widely
// (docs/design/log-growth.md section 5, "Removing a secret"): each stream
// that holds it is redacted as its own route redacts it, one
// event.redacted each.

// namedStreams are the streams that aren't a task's or a feature's, in the order
// a redaction across the log lists them.
var namedStreams = []string{events.AgentsStream, events.PolicyStream, events.WorkersStream, events.ConnectionsStream, events.SettingsStream, events.TemplatesStream}

// loggedStream is a stream a redaction across the log reads, and whether
// it is an archived task's.
type loggedStream struct {
	name     string
	archived bool
}

// streamPreview is how many rows of a stream hold the text, as the
// preview across the log lists it, or why that can't be known or they
// can't be redacted.
type streamPreview struct {
	Stream   string `json:"stream"`
	Rows     int    `json:"rows"`
	Archived bool   `json:"archived"`
	Error    string `json:"error,omitempty"`

	rejection *events.Rejection
	err       error
}

// logPreview is the preview of a redaction across the log: the rows that
// hold the text, and the streams they are in.
type logPreview struct {
	Rows    int             `json:"rows"`
	Streams []streamPreview `json:"streams"`
}

// streamRedacted is a stream a redaction across the log rewrote.
type streamRedacted struct {
	Stream string    `json:"stream"`
	At     time.Time `json:"at"`
	Rows   int       `json:"rows"`
}

// logRedacted is what a redaction across the log answers: the rows
// rewritten, in each stream, and what was refused or may be left on the
// disk for now.
type logRedacted struct {
	Rows     int              `json:"rows"`
	Streams  []streamRedacted `json:"streams"`
	Warnings []string         `json:"warnings"`
}

// previewLogRedaction is previewRedaction across the whole log: each
// stream that holds the text you marked, and how many of its rows, or
// why it can't be redacted. It writes nothing. No row anywhere holding it
// is answered 409.
func (s *Server) previewLogRedaction(w http.ResponseWriter, r *http.Request) {
	text, _, ok := secretOf(w, r)
	if !ok {
		return
	}
	preview := s.previewLog(text)
	if preview.Rows == 0 {
		http.Error(w, preview.noneHeld(), http.StatusConflict)
		return
	}
	httpapi.Reply(w, preview)
}

// redactLog redacts the text you marked from each stream its preview
// lists, as redactIn redacts one. A stream refused or failing is a
// warning, and the rest are redacted still; only when none is, it is
// answered as answerRedaction answers one stream. The snapshot is
// written once, at the end. Only the page may ask, as for redact.
func (s *Server) redactLog(w http.ResponseWriter, r *http.Request) {
	text, why, ok := secretOf(w, r)
	if !ok {
		return
	}
	preview := s.previewLog(text)
	if preview.Rows == 0 {
		http.Error(w, preview.noneHeld(), http.StatusConflict)
		return
	}
	answer := logRedacted{Streams: []streamRedacted{}, Warnings: []string{}}
	failed := false // a stream failed, rather than was refused
	for _, listed := range preview.Streams {
		done := streamRedaction{rejection: listed.rejection, err: listed.err}
		if listed.Error == "" {
			done = s.redactIn(listed.Stream, text, why, listed.Archived)
		}
		switch {
		case done.redaction.Position != 0:
			answer.Rows += done.rows()
			answer.Streams = append(answer.Streams, streamRedacted{Stream: listed.Stream, At: done.redaction.At, Rows: done.rows()})
			if done.err != nil {
				answer.Warnings = append(answer.Warnings, listed.Stream+": "+done.err.Error())
			}
		case done.rejection != nil:
			answer.Warnings = append(answer.Warnings, listed.Stream+": "+done.rejection.Reason)
		default:
			failed = true
			answer.Warnings = append(answer.Warnings, listed.Stream+": "+done.err.Error())
		}
		answer.Warnings = append(answer.Warnings, done.warnings...)
	}
	switch {
	case len(answer.Streams) == 0 && failed:
		http.Error(w, strings.Join(answer.Warnings, "; "), http.StatusInternalServerError)
		return
	case len(answer.Streams) == 0:
		http.Error(w, strings.Join(answer.Warnings, "; "), http.StatusConflict)
		return
	}
	answer.Warnings = append(answer.Warnings, s.writeOverOldText()...)
	httpapi.Reply(w, answer)
}

// previewLog previews the redaction of text from every stream in the log
// (streamsLogged), listing those that hold it and those whose preview is
// refused or fails, but not those that don't hold it.
func (s *Server) previewLog(text string) logPreview {
	preview := logPreview{Streams: []streamPreview{}}
	for _, stream := range s.streamsLogged() {
		rows, rejection, err := s.previewIn(stream.name, text, stream.archived)
		if store.HoldsNone(stream.name, rejection) {
			continue
		}
		listed := streamPreview{Stream: stream.name, Rows: rows, Archived: stream.archived, rejection: rejection, err: err}
		switch {
		case rejection != nil:
			listed.Error = rejection.Reason
		case err != nil:
			listed.Error = err.Error()
		}
		preview.Rows += rows
		preview.Streams = append(preview.Streams, listed)
	}
	return preview
}

// noneHeld is why a preview with no row is refused: no row holds the
// text, or, when some stream couldn't be read or redacted, why not, each
// reason once.
func (p logPreview) noneHeld() string {
	var reasons []string
	for _, listed := range p.Streams {
		if !slices.Contains(reasons, listed.Error) {
			reasons = append(reasons, listed.Error)
		}
	}
	if len(reasons) == 0 {
		return "no row of the log holds the secret"
	}
	return strings.Join(reasons, "; ")
}

// streamsLogged is every stream a redaction across the log reads, in the
// order it lists them: each task's, by id, but an erased one's, of which
// nothing is left; then the named streams; then each feature's, in log
// order.
func (s *Server) streamsLogged() []loggedStream {
	var streams []loggedStream
	for _, id := range s.state.TaskIDs() {
		if _, erased := s.state.Erased(id); erased {
			continue
		}
		_, err := s.task(id)
		streams = append(streams, loggedStream{name: id, archived: errors.Is(err, current.ErrArchived)})
	}
	for _, name := range namedStreams {
		streams = append(streams, loggedStream{name: name})
	}
	for _, feature := range s.config.Features {
		if feature.Streams == nil {
			continue
		}
		for _, id := range s.state.OwnedIDs(feature.Streams.Prefix) {
			streams = append(streams, loggedStream{name: id})
		}
	}
	return streams
}
