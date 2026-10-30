package extensions

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// A job that panics logs its extension and stops; another job keeps running
// until ctx ends, and so does the process running them.
func TestAPanickingJobStopsAloneAndSaysWhose(t *testing.T) {
	logged := captureLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks, stopped := make(chan struct{}, 1), make(chan struct{})
	list := []Extension{
		{Name: "breaks", Jobs: func(context.Context, Core) { panic("a broken job") }},
		{Name: "quiet"}, // no jobs
		{Name: "steady", Jobs: func(ctx context.Context, c Core) {
			defer close(stopped)
			for {
				select {
				case <-ctx.Done():
					return
				case ticks <- struct{}{}:
				}
			}
		}},
	}
	StartJobs(ctx, list, func(Extension) Core { return Core{} })
	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(logged.String(), "feature=breaks"); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the panic wasn't logged:\n%s", logged.String())
		}
	}
	for range 3 { // after the panic, the other job goes on
		waitFor(t, ticks, "the steady job to tick")
	}
	cancel()
	waitFor(t, stopped, "the steady job to stop when ctx ends")
	said := logged.String()
	for _, want := range []string{"a broken job", "stack="} {
		if !strings.Contains(said, want) {
			t.Errorf("the log lacks %q:\n%s", want, said)
		}
	}
	if strings.Contains(said, "steady") {
		t.Errorf("the steady job is logged as failing:\n%s", said)
	}
}

// Each job is given the Core made for its own extension.
func TestAJobIsGivenItsOwnExtensionsCore(t *testing.T) {
	file := func() map[string]Table {
		return map[string]Table{"a": {Values: map[string]string{"who": "a"}}, "b": {Values: map[string]string{"who": "b"}}}
	}
	got := make(chan string, 2)
	job := func(_ context.Context, c Core) { who, _ := c.Settings.Setting("who"); got <- who }
	StartJobs(context.Background(), []Extension{{Name: "a", Jobs: job}, {Name: "b", Jobs: job}},
		func(f Extension) Core { return Core{Settings: SettingsIn(file, f.Name)} })
	seen := map[string]bool{}
	for range 2 {
		select {
		case who := <-got:
			seen[who] = true
		case <-time.After(5 * time.Second):
			t.Fatal("the jobs didn't run")
		}
	}
	if !seen["a"] || !seen["b"] {
		t.Errorf("the jobs saw %v, want a and b", seen)
	}
}

func waitFor[T any](t *testing.T, c <-chan T, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(5 * time.Second):
		t.Fatalf("waited 5s for %s", what)
	}
}

// captureLog sends slog's default logger to a buffer until the test ends.
func captureLog(t *testing.T) *syncBuffer {
	logged := &syncBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return logged
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
