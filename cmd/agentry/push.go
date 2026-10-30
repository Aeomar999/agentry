package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/webpush"
)

const (
	pushTimeout  = 30 * time.Second // a push service that never answers holds up every push after it
	pushFallback = 30 * time.Second // how often the sink looks again, in case the log reader missed a change
)

// startPush pushes what newly waits for you to the subscribed browsers,
// until ctx ends. Push works only at an https -public-url (the page offers
// it only then, and phones away from here reach nothing else), so without
// one nothing starts, even for browsers subscribed on an earlier run.
func startPush(ctx context.Context, log *store.Store, push *webpush.Store, publicURL string, waiting func() []extensions.Waiting) {
	if available, _ := webpush.Available(publicURL); !available {
		return
	}
	// The public URL is who sends: push services contact it about pushes that misbehave.
	sender := webpush.NewSender(push, &http.Client{Timeout: pushTimeout}, publicURL)
	wake := make(chan struct{}, 1)
	go followLog(ctx, log.NewReader(log.LastPosition()), wake) // what is appended from now on
	go tickEvery(ctx, pushFallback, wake)
	go webpush.NewSink(waiting, sender, wake).Run(ctx)
}

// followLog taps wake for each batch of events reader reads, until ctx
// ends, and closes reader. The read model is applied before the log tells
// its readers, so what waits has changed by the time wake fires. A log it
// can't read is left to the ticker.
func followLog(ctx context.Context, reader *store.Reader, wake chan<- struct{}) {
	defer reader.Close()
	for {
		if _, err := reader.Next(ctx); err != nil {
			if ctx.Err() == nil {
				slog.Warn("push: can't follow the log; looking every "+pushFallback.String(), "error", err)
			}
			return
		}
		tap(wake)
	}
}

// tickEvery taps wake every so often until ctx ends, a safety net for a
// change the log reader missed.
func tickEvery(ctx context.Context, every time.Duration, wake chan<- struct{}) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tap(wake)
		}
	}
}

// tap wakes whoever reads wake without waiting for them: wakes not yet
// read merge into one.
func tap(wake chan<- struct{}) {
	select {
	case wake <- struct{}{}:
	default:
	}
}
