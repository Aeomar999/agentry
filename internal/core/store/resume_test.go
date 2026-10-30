package store

import (
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// resuming is a model with a saved copy of itself at a position: at Resume
// it reads the log, then either goes on from its copy or empties itself.
type resuming struct {
	partWay
	keep   bool     // go on from the copy; otherwise empty itself
	read   []string // what Resume read
	hashes map[int64]string
}

func (r *resuming) Resume(log Past) error {
	r.hashes = map[int64]string{}
	for position := range log.Last() + 2 {
		hash, found, err := log.Hash(position)
		if err != nil {
			return err
		}
		if found {
			r.hashes[position] = hash
		}
	}
	accepted, err := log.Stream("task-1", r.since)
	if err != nil {
		return err
	}
	for _, e := range accepted {
		r.read = append(r.read, string(e.Type))
	}
	if !r.keep {
		r.since = 0
		return nil
	}
	if err := log.ApplyAfter(r, r.since); err != nil {
		return err
	}
	r.since = log.Last()
	return nil
}

// Open hands a Resumer the log before anything else. One that goes on from
// its copy applies the rest itself and is loaded no further; one that
// empties itself is loaded whole.
func TestOpenLetsAResumerCheckItsCopyFirst(t *testing.T) {
	path := seeded(t)
	kept := &resuming{partWay: partWay{since: 1}, keep: true}
	s, err := Open(path, events.Core(), kept, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if !slices.Equal(kept.read, []string{string(events.TaskCreated)}) || len(kept.calls) > 0 || !slices.Equal(kept.applied, []int64{2, 3}) {
		t.Errorf("going on from its copy: read %v, called %v, applied %v", kept.read, kept.calls, kept.applied)
	}
	emptied := &resuming{partWay: partWay{since: 1}}
	s, err = Open(path, events.Core(), emptied, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !slices.Equal(emptied.calls, []string{"Load task-1", "Load task-2", "Loaded"}) || len(emptied.applied) > 0 {
		t.Errorf("emptied: called %v, applied %v", emptied.calls, emptied.applied)
	}
	// Every row has a hash of its own, the same through the store's reads.
	if len(emptied.hashes) != 3 {
		t.Fatalf("hashes for %v, want positions 1 to 3 only", emptied.hashes)
	}
	seen := map[string]bool{}
	for position, hash := range emptied.hashes {
		if seen[hash] {
			t.Errorf("two rows share the hash %s", hash)
		}
		seen[hash] = true
		if read, found, err := s.Hash(position); err != nil || !found || read != hash {
			t.Errorf("Store.Hash(%d) is %q, %v, %v; Open read %q", position, read, found, err, hash)
		}
	}
	if _, found, err := s.Hash(4); found || err != nil {
		t.Errorf("Store.Hash past the end: found %v, %v", found, err)
	}
}
