package machines

import (
	"context"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
)

func TestTheFleetPicksWhereAnAgentWorks(t *testing.T) {
	here := &Local{Label: workers.Local}
	laptop, desktop := &Local{Label: "laptop"}, &Local{Label: "desktop"}
	fleet := NewFleet(here)
	fleet.Connect(laptop, []string{"claude 2.1.301"})
	fleet.Connect(desktop, []string{"codex 0.9"})

	cases := []struct {
		name    string
		workers []string
		current string // where the task's work is
		want    string // "" for none
	}{
		{"no workers: this machine", nil, "laptop", workers.Local},
		{"the first that is online", []string{"tablet", "laptop", workers.Local}, "desktop", "laptop"},
		{"where the work is already, if it may", []string{workers.Local, "laptop"}, "laptop", "laptop"},
		{"this machine when its workers are away", []string{"tablet", workers.Local}, "laptop", workers.Local},
		{"not one without its engine", []string{"desktop"}, workers.Local, ""},
		{"none online", []string{"tablet"}, workers.Local, ""},
	}
	for _, c := range cases {
		m, err := fleet.For(agents.Agent{Name: "engineer", Engine: "claude", Workers: c.workers}, c.current)
		got := ""
		if err == nil {
			got = m.Name()
		}
		if got != c.want {
			t.Errorf("%s: got %q (%v), want %q", c.name, got, err, c.want)
		}
	}

	_, err := fleet.For(agents.Agent{Name: "engineer", Engine: "claude", Workers: []string{"tablet", "desktop"}}, workers.Local)
	if err == nil || !strings.Contains(err.Error(), "tablet is offline, desktop has no claude") {
		t.Errorf("why it can't run: %v", err)
	}

	stale := &Local{Label: "laptop"}
	if fleet.Disconnect(stale) {
		t.Error("an old connection's end removed the one that replaced it")
	}
	if !fleet.Disconnect(laptop) || !fleet.Of("laptop").IsRunning("task-1") {
		t.Error("a worker that just went offline should be taken to run what it ran, while it dials back")
	}
	if (Offline{Worker: "laptop", Since: time.Now().Add(-OfflineGrace)}).IsRunning("task-1") {
		t.Error("a worker offline for longer than the grace still runs things")
	}
	if fleet.Of("tablet").IsRunning("task-1") {
		t.Error("a worker never seen runs things")
	}
}

// Just after a restart, a worker that may be running agents is waited for,
// not reported offline: it dials back in a second or two.
func TestTheFleetWaitsForAWorkerComingBack(t *testing.T) {
	fleet := NewFleet(&Local{Label: workers.Local})
	fleet.Expect("laptop")
	laptop := &Local{Label: "laptop"}
	go func() {
		time.Sleep(300 * time.Millisecond)
		fleet.Connect(laptop, []string{"claude 2.1.301"})
	}()
	engineer := agents.Agent{Name: "engineer", Engine: "claude", Workers: []string{"laptop"}}
	m, err := fleet.WaitFor(context.Background(), engineer, workers.Local)
	if err != nil || m != laptop {
		t.Fatalf("got %v %v, want the laptop once it is back", m, err)
	}
	if _, err := fleet.WaitFor(context.Background(), agents.Agent{Name: "x", Engine: "claude", Workers: []string{"tablet"}}, workers.Local); err == nil {
		t.Error("a worker never seen was waited for, or found")
	}
}
