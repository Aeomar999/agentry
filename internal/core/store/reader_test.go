package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestReaderGetsEveryEventInOrderAtItsOwnPace(t *testing.T) {
	s := openTestStore(t)
	reader := s.NewReader(0)
	defer reader.Close()

	// The reader isn't reading while 1,200 events are written. Writes must not wait for it.
	started := time.Now()
	for i := range 1200 {
		if _, _, err := s.Propose(newTask(fmt.Sprintf("task-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("writes took %s with a reader not reading", elapsed)
	}

	var positions []int64
	for len(positions) < 1200 {
		batch, err := reader.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range batch {
			positions = append(positions, e.Position)
		}
	}
	for i, position := range positions {
		if position != int64(i+1) {
			t.Fatalf("event %d has position %d", i, position)
		}
	}
}

func TestReaderWaitsForTheNextEvent(t *testing.T) {
	s := openTestStore(t)
	s.Propose(newTask("task-1"))
	reader := s.NewReader(1)
	defer reader.Close()

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.Propose(newTask("task-2"))
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	batch, err := reader.Next(ctx)
	if err != nil || len(batch) != 1 || batch[0].Stream != "task-2" {
		t.Fatalf("got %+v, %v", batch, err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := reader.Next(ctx); err != context.DeadlineExceeded {
		t.Errorf("with nothing new, Next should wait until ctx ends, got %v", err)
	}
}
