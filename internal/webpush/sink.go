package webpush

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"unicode/utf8"

	"gitlab.com/dev.chempa/agentry/internal/features"
)

const lineShown = 500 // how much of an entry's line a push carries; JSON escaping can make it six times longer

// Sink pushes each thing that newly waits for you to every browser in the
// Store, once. What already waits when it starts is not news.
type Sink struct {
	waiting func() []features.Waiting
	sender  *Sender
	wake    <-chan struct{}
	seen    map[string]bool
}

// NewSink pushes what waiting lists, with sender, rechecking it each time
// wake fires. waiting is the server's Waiting(""), which reads the read
// model, so wake should fire when the log changes.
func NewSink(waiting func() []features.Waiting, sender *Sender, wake <-chan struct{}) *Sink {
	return &Sink{waiting: waiting, sender: sender, wake: wake}
}

// Run pushes what newly waits until ctx ends.
func (s *Sink) Run(ctx context.Context) {
	s.start()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
			s.check(ctx)
		}
	}
}

// start takes what waits now as already seen.
func (s *Sink) start() { s.seen = keys(s.waiting()) }

// check pushes every entry not waiting at the last check. Then the seen
// set is exactly what waits now, so an entry that leaves and comes back is
// news again.
func (s *Sink) check(ctx context.Context) {
	waiting := s.waiting()
	for _, entry := range waiting {
		if !s.seen[entry.Key] {
			s.push(ctx, entry)
		}
	}
	s.seen = keys(waiting)
}

// push sends entry to every subscription. One that fails doesn't stop the
// others; its error goes to the server's log, never the event log, since
// it can name the device's address.
func (s *Sink) push(ctx context.Context, entry features.Waiting) {
	message, err := json.Marshal(messageOf(entry))
	if err != nil {
		slog.Warn("push: the message can't be made", "key", entry.Key, "error", err)
		return
	}
	for _, sub := range s.sender.store.Subscriptions() {
		err := s.sender.Send(ctx, sub, message)
		if err != nil && !errors.Is(err, ErrGone) {
			slog.Warn("push: not sent", "key", entry.Key, "error", err)
		}
	}
}

// message is what a push tells the service worker: what to show, and the
// page to open when it is tapped.
type message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Task  string `json:"task"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

func messageOf(entry features.Waiting) message {
	url := "/"
	if entry.Task != "" {
		url = "/tasks/" + entry.Task
	}
	return message{Title: "Agentry", Body: shortened(entry.Line, lineShown), Task: entry.Task, URL: url, Tag: entry.Key}
}

// shortened is text cut to at most limit bytes, on a character boundary,
// with an ellipsis when cut.
func shortened(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit - len("…")
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

func keys(waiting []features.Waiting) map[string]bool {
	seen := map[string]bool{}
	for _, entry := range waiting {
		seen[entry.Key] = true
	}
	return seen
}
