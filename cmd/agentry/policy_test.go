package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

func TestEachPolicyVersionIsRecordedWithItsText(t *testing.T) {
	dir := t.TempDir()
	log, _ := storetest.Open(t, filepath.Join(dir, "agentry.db"))
	live, err := openPolicy(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := log.Stream(events.PolicyStream)
	if len(stream) != 1 || stream[0].Text("change") != "first loaded" || stream[0].Text("text") != string(policy.Default()) ||
		stream[0].Data["onDisk"] != nil {
		t.Fatalf("the first start records the whole policy: %+v", stream)
	}

	edited := strings.Replace(string(policy.Default()), "\nuse_for_everyone =", "\n# use_for_everyone =", 1)
	os.WriteFile(live.Path(), []byte(edited), 0o600)
	loaded, refused := live.Reload()
	recordOnDisk(log, live, loaded, refused)
	stream, _ = log.Stream(events.PolicyStream)
	onDisk := stream[len(stream)-1]
	if onDisk.Actor != policy.Agentry || !onDisk.Flag("onDisk") || onDisk.Text("text") != edited ||
		!strings.Contains(onDisk.Text("diff"), "\n-use_for_everyone = \"allow-everything\"\n+# use_for_everyone = \"allow-everything\"\n") {
		t.Errorf("a change on disk is Agentry's, flagged, with its diff: %s %v\n%s", onDisk.Actor, onDisk.Data, onDisk.Text("diff"))
	}

	os.WriteFile(live.Path(), []byte("version = "), 0o600)
	loaded, refused = live.Reload()
	recordOnDisk(log, live, loaded, refused)
	if stream, _ = log.Stream(events.PolicyStream); stream[len(stream)-1].Type != events.PolicyRefused {
		t.Errorf("a broken file is recorded as refused: %s", stream[len(stream)-1].Type)
	}
}

// The HTTPS CA's key is one of Agentry's own files: no profile reaches it.
func TestTheCAKeyIsOffLimits(t *testing.T) {
	dir := t.TempDir()
	log, _ := storetest.Open(t, filepath.Join(dir, "agentry.db"))
	live, err := openPolicy(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(tlsca.Dir(dir), "ca.key")
	for _, call := range []policy.Call{
		{Tool: "Read", Input: key, Workspace: dir},
		{Tool: "Bash", Input: "cat " + key, Workspace: dir},
		{Tool: "Bash", Input: "tar czf /tmp/ca.tgz " + tlsca.Dir(dir), Workspace: dir},
	} {
		if got := live.Current().Decide("allow-everything", call); got.Decision != policy.Deny {
			t.Errorf("%s %q: %s (%s)", call.Tool, call.Input, got.Decision, got.Reason())
		}
	}
}

// A policy changed while Agentry was stopped is recorded at the next
// start, with its diff from the last version the log holds the text of.
func TestAPolicyChangedWhileStoppedIsRecordedAtStart(t *testing.T) {
	dir := t.TempDir()
	log, _ := storetest.Open(t, filepath.Join(dir, "agentry.db"))
	if _, err := openPolicy(dir, log); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, policyFile), append(policy.Default(), "# mine\n"...), 0o600)
	if _, err := openPolicy(dir, log); err != nil {
		t.Fatal(err)
	}
	stream, _ := log.Stream(events.PolicyStream)
	if last := stream[len(stream)-1]; len(stream) != 2 || !last.Flag("onDisk") || !strings.HasSuffix(last.Text("diff"), "+# mine\n") {
		t.Errorf("%d events, last %v:\n%s", len(stream), last.Data, last.Text("diff"))
	}
	if _, err := openPolicy(dir, log); err != nil {
		t.Fatal(err)
	}
	if again, _ := log.Stream(events.PolicyStream); len(again) != 2 {
		t.Errorf("an unchanged policy isn't recorded again: %d events", len(again))
	}
}
