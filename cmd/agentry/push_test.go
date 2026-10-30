package main

import (
	"context"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
)

const pushGuard = 5 * time.Second // only a failing test waits this long

func TestAnAppendWakesThePushSink(t *testing.T) {
	log := logWithAgents(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake := make(chan struct{}, 1)
	go followLog(ctx, log.NewReader(log.LastPosition()), wake)

	for _, id := range []string{"task-1", "task-2"} {
		createTask(t, log, id)
		select {
		case <-wake:
		case <-time.After(pushGuard):
			t.Fatalf("creating %s didn't wake the sink", id)
		}
	}
}

func TestWakesNotYetReadMergeIntoOne(t *testing.T) {
	wake := make(chan struct{}, 1)
	for range 3 {
		tap(wake) // never blocks
	}
	<-wake
	select {
	case <-wake:
		t.Fatal("three taps woke the sink twice")
	default:
	}
}

func TestFollowingTheLogStopsWithTheServer(t *testing.T) {
	log := logWithAgents(t)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		followLog(ctx, log.NewReader(log.LastPosition()), make(chan struct{}, 1))
		close(stopped)
	}()
	cancel()
	select {
	case <-stopped:
	case <-time.After(pushGuard):
		t.Fatal("followLog went on after ctx ended")
	}
}

func TestTheFallbackTickerStopsWithTheServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		tickEvery(ctx, time.Hour, make(chan struct{}, 1))
		close(stopped)
	}()
	cancel()
	select {
	case <-stopped:
	case <-time.After(pushGuard):
		t.Fatal("tickEvery went on after ctx ended")
	}
}

func logWithAgents(t *testing.T) *store.Store {
	t.Helper()
	log, state := openLog(t, t.TempDir())
	t.Cleanup(func() { log.Close() })
	if err := seedAgents(log, state); err != nil {
		t.Fatal(err)
	}
	return log
}

func createTask(t *testing.T, log *store.Store, id string) {
	t.Helper()
	p := events.Proposal{Stream: id, Actor: events.Actor{Kind: events.Human, ID: "you"}, Type: events.TaskCreated,
		Data: map[string]any{"title": "A task", "repo": "/r", "coordinator": "tech-lead"}}
	if _, rejection, err := log.Propose(p); rejection != nil || err != nil {
		t.Fatalf("creating %s: %v, %v", id, rejection, err)
	}
}
