package web

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
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

// streamFailed is a stream a redaction across the log left holding the
// text: why, and whether counting its rows again and confirming that
// count would redact it.
type streamFailed struct {
	Stream  string `json:"stream"`
	Reason  string `json:"reason"`
	Recount bool   `json:"recount"`
}

// logRedacted is what a redaction across the log answers: the rows
// rewritten, in each stream; the streams left holding the text, and why;
// and what may be left on the disk for now, those streams among it.
type logRedacted struct {
	Rows     int              `json:"rows"`
	Streams  []streamRedacted `json:"streams"`
	Failures []streamFailed   `json:"failures"`
	Warnings []string         `json:"warnings"`
}

// previewLogRedaction is previewRedaction across the whole log: each
// stream that holds the text you marked, and how many of its rows, or
// why it can't be redacted. It writes nothing. No row anywhere holding it
// is answered as answerPreview answers one stream: 500 when a stream
// failed, 409 otherwise.
func (s *Server) previewLogRedaction(w http.ResponseWriter, r *http.Request) {
	asked, ok := secretOf(w, r)
	if !ok {
		return
	}
	preview := s.previewLog(asked.text)
	if preview.Rows == 0 {
		http.Error(w, preview.noneHeld(), preview.noneHeldStatus())
		return
	}
	httpapi.Reply(w, preview)
}

// redactLog redacts the text you marked from each stream its preview
// lists, as redactIn redacts one. When the page sends the rows its preview
// counted in each stream, a stream is refused unless that many of its rows
// hold the text as it commits, and one it didn't count isn't redacted, as
// you didn't see its rows; with no counts, each is redacted of however
// many rows hold the text. A stream refused or failing is a failure, and
// a warning, and the rest are redacted still; only when none is, it is
// answered as answerRedaction answers one stream. The snapshot is written
// once, at the end. Only the page may ask, as for redact.
func (s *Server) redactLog(w http.ResponseWriter, r *http.Request) {
	asked, counts, ok := countedSecretOf(w, r)
	if !ok {
		return
	}
	preview := s.previewLog(asked.text)
	if preview.Rows == 0 {
		http.Error(w, preview.noneHeld(), preview.noneHeldStatus())
		return
	}
	answer := logRedacted{Streams: []streamRedacted{}, Failures: []streamFailed{}, Warnings: []string{}}
	failed := false // a stream failed, rather than was refused
	for _, listed := range preview.Streams {
		done := streamRedaction{rejection: listed.rejection, err: listed.err}
		if listed.Error == "" {
			done = s.redactCountedIn(listed, asked, counts)
		}
		switch {
		case done.redaction.Position != 0:
			answer.Rows += done.rows()
			answer.Streams = append(answer.Streams, streamRedacted{Stream: listed.Stream, At: done.redaction.At, Rows: done.rows()})
			if done.err != nil {
				answer.Warnings = append(answer.Warnings, listed.Stream+": "+done.err.Error())
			}
		case done.rejection != nil:
			answer.fail(listed.Stream, done.rejection.Reason, store.Miscounted(done.rejection))
		default:
			failed = true
			answer.fail(listed.Stream, done.err.Error(), false)
		}
		answer.Warnings = append(answer.Warnings, done.warnings...)
	}
	for _, stream := range countedGone(counts, preview) {
		answer.fail(stream, fmt.Sprintf("no row holds this text now, not %d: count them again", counts[stream]), true)
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

// fail names stream as left holding the text, for reason, among the
// failures and the warnings.
func (answer *logRedacted) fail(stream, reason string, recount bool) {
	answer.Failures = append(answer.Failures, streamFailed{Stream: stream, Reason: reason, Recount: recount})
	answer.Warnings = append(answer.Warnings, stream+": "+reason)
}

// countedSecretOf is what a redaction across the log asks for, as
// secretOf, and the rows its preview counted in each stream, nil when the
// page sent none; or false once it has answered 400 for a body it can't
// read, a blank text or a count below one, as a redaction rewrites at
// least one row.
func countedSecretOf(w http.ResponseWriter, r *http.Request) (redactionAsked, map[string]int, bool) {
	var request struct {
		Text, Why string
		Counts    map[string]int
	}
	if !httpapi.Decode(w, r, &request) {
		return redactionAsked{}, nil, false
	}
	asked, ok := askedOf(w, request.Text, request.Why)
	if !ok {
		return redactionAsked{}, nil, false
	}
	for stream, rows := range request.Counts {
		if rows < 1 {
			http.Error(w, "counts are how many rows of each stream the preview counted: 1 or more, not "+strconv.Itoa(rows)+" for "+stream, http.StatusBadRequest)
			return redactionAsked{}, nil, false
		}
	}
	return asked, request.Counts, true
}

// redactCountedIn is redactIn of a stream the preview listed, refused
// unless as many of its rows hold the text as counts holds for it. With
// no counts at all, it redacts however many rows hold it; with counts but
// none for this stream, it is refused, as you didn't see its rows.
func (s *Server) redactCountedIn(listed streamPreview, asked redactionAsked, counts map[string]int) streamRedaction {
	if counts != nil {
		rows, counted := counts[listed.Stream]
		if !counted {
			return streamRedaction{rejection: &events.Rejection{Check: store.CountCheck, Reason: store.RowsHolding(listed.Rows) + " this text, which the preview you confirmed didn't count: count them again"}}
		}
		asked.rows = rows
	}
	return s.redactIn(listed.Stream, asked, listed.Archived)
}

// countedGone is each stream counts holds that the preview no longer
// lists, as no row of it holds the text now, by name.
func countedGone(counts map[string]int, preview logPreview) []string {
	var gone []string
	for stream := range counts {
		if !slices.ContainsFunc(preview.Streams, func(listed streamPreview) bool { return listed.Stream == stream }) {
			gone = append(gone, stream)
		}
	}
	slices.Sort(gone)
	return gone
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

// noneHeldStatus is how a preview with no row is refused: 500 when a
// stream's preview failed, rather than was refused, 409 otherwise.
func (p logPreview) noneHeldStatus() int {
	if p.failed() {
		return http.StatusInternalServerError
	}
	return http.StatusConflict
}

// failed is whether a listed stream's preview failed, rather than was
// refused.
func (p logPreview) failed() bool {
	return slices.ContainsFunc(p.Streams, func(listed streamPreview) bool { return listed.err != nil })
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
