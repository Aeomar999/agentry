package store

import (
	"context"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Reader follows the log from a position, at its own pace. The store never
// pushes events at readers: a write only taps each reader's wake channel,
// which never blocks, so a slow reader can't stall writes (proof 05).
type Reader struct {
	store    *Store
	position int64
	wake     chan struct{}
}

const batchSize = 500

// NewReader starts reading after position (0 reads the whole log).
func (s *Store) NewReader(position int64) *Reader {
	r := &Reader{store: s, position: position, wake: make(chan struct{}, 1)}
	s.changes.add(r.wake)
	return r
}

// Next returns the next events after the reader's position, waiting until
// there are some or ctx ends.
func (r *Reader) Next(ctx context.Context) ([]events.Event, error) {
	for {
		found, err := r.store.After(r.position, batchSize)
		if err != nil {
			return nil, err
		}
		if len(found) > 0 {
			r.position = found[len(found)-1].Position
			return found, nil
		}
		select {
		case <-r.wake:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (r *Reader) Position() int64 { return r.position }

func (r *Reader) Close() { r.store.changes.remove(r.wake) }

// changes is the set of readers' wake channels.
type changes struct {
	mu    sync.Mutex
	wakes map[chan struct{}]bool
}

func newChanges() *changes { return &changes{wakes: map[chan struct{}]bool{}} }

func (c *changes) add(wake chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wakes[wake] = true
}

func (c *changes) remove(wake chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.wakes, wake)
}

// notify taps every reader. A reader already tapped stays tapped; nobody waits.
func (c *changes) notify() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for wake := range c.wakes {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}
