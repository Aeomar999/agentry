package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

const jobSecret = "s3cr3t-never-logged-41d9"

const jobSettings = `
[probe]
host = "first.example"

[probe.secrets]
token = "` + jobSecret + `"
`

// A feature's job reads its secret and sees an edit to the file with no
// restart. The secret, through a load, a reload and a broken version whose
// bad line holds it, reaches neither the server's log nor the event log, nor
// a start of the server with the file broken or missing.
func TestASecretReachesTheJobAndNeitherLog(t *testing.T) {
	dataDir := t.TempDir()
	settings := filepath.Join(dataDir, settingsFile)
	writeSettings(t, settings, jobSettings)
	var logged syncBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	defer slog.SetDefault(previous)

	state := current.Restore(filepath.Join(dataDir, snapshotFile), "test", storetest.WithFeatures(), features.StreamsOf(installedFeatures()...)...)
	log, err := store.Open(filepath.Join(dataDir, "agentry.db"), storetest.WithFeatures(), state, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedAgents(log, state); err != nil {
		t.Fatal(err)
	}
	type seen struct{ host, token string }
	reads := make(chan seen)
	probe := features.Feature{Name: "probe", Jobs: func(ctx context.Context, c features.Core) {
		for {
			host, _ := c.Settings.Setting("host")
			token, _ := c.Settings.Secret("token")
			select {
			case <-ctx.Done():
				return
			case reads <- seen{host, token}:
			}
		}
	}}
	logs := map[string]features.Proposer{probe.Name: features.Checked(probe, events.Registration{}, log)}
	ctx, cancel := context.WithCancel(context.Background())
	startJobs(ctx, openSettings(ctx, settings, 5*time.Millisecond).For, []features.Feature{probe}, logs, log, state, tasks.Limits{})
	waitForRead := func(want seen) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); ; {
			select {
			case got := <-reads:
				if got == want {
					return
				}
			case <-time.After(time.Until(deadline)):
				t.Fatalf("the job never read %+v", want)
			}
		}
	}
	waitForRead(seen{"first.example", jobSecret})
	writeSettings(t, settings, strings.Replace(jobSettings, "first.example", "second.example", 1))
	waitForRead(seen{"second.example", jobSecret})
	writeSettings(t, settings, jobSettings+"\n["+jobSecret+"\n")
	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(logged.String(), "doesn't load"); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the broken file wasn't reported:\n%s", logged.String())
		}
	}
	waitForRead(seen{"second.example", jobSecret}) // the last good version stays in force
	cancel()
	log.Close()

	said := logged.String() + servedOnce(t, dataDir) // a start with the file broken
	os.Remove(settings)
	said += servedOnce(t, dataDir) // and with none
	if strings.Contains(said, jobSecret) {
		t.Errorf("the secret is in the server's log:\n%s", said)
	}
	for _, name := range []string{"agentry.db", "agentry.db-wal", snapshotFile} {
		kept, err := os.ReadFile(filepath.Join(dataDir, name))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if strings.Contains(string(kept), jobSecret) {
			t.Errorf("the secret is in %s", name)
		}
	}
}

// No agent may touch the settings file, whatever the policy says.
func TestTheSettingsFileIsProtected(t *testing.T) {
	dataDir := t.TempDir()
	log, err := store.Open(filepath.Join(dataDir, "agentry.db"), storetest.WithFeatures(), storetest.NewState(storetest.WithFeatures()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	live, err := openPolicy(dataDir, log)
	if err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(dataDir, settingsFile)
	for _, call := range []policy.Call{
		{Tool: "Read", Input: settings},
		{Tool: "Bash", Input: "cat " + settings},
	} {
		if verdict := live.Decide("developer", call); verdict.Decision != policy.Deny {
			t.Errorf("%s of the settings file: %+v, want deny", call.Tool, verdict)
		}
	}
}

func writeSettings(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
