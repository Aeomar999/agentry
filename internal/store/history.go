package store

import (
	"cmp"
	"fmt"
	"io"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Opener opens a blob by its hash: sharedfiles.Disk.
type Opener interface {
	Open(hash string) (io.ReadCloser, error)
}

// Archived reads the events an archived stream's marker keeps in its blob,
// up to the marker's "through". FromBlobs reads them from the blob itself;
// a cache can stand in front of it.
type Archived func(marker events.Event) ([]events.Event, error)

// FromBlobs reads a marker's events from its blob in blobs.
func FromBlobs(blobs Opener) Archived {
	return func(marker events.Event) ([]events.Event, error) {
		blob, err := blobs.Open(marker.Text("archive"))
		if err != nil {
			return nil, err
		}
		defer blob.Close()
		kept, err := ReadArchive(blob)
		if err != nil {
			return nil, err
		}
		through := int64(marker.Number("through"))
		return slices.DeleteFunc(kept, func(e events.Event) bool { return e.Position > through }), nil
	}
}

// History is a stream's whole record, rejections included, as Log has it,
// archived rows too: the one read of a task's events, so no reader takes
// an archived task's live rows alone by mistake. A stream archived now
// (its last task.archived has no task.unarchived after it) is the rows
// archived reads for its marker, then its live rows: the marker, and any
// a crash left, in position order, each position once. Any other stream is
// its live rows.
func (s *Store) History(stream string, archived Archived) ([]events.Event, error) {
	live, err := s.Log(stream)
	if err != nil {
		return nil, err
	}
	marker, ok := MarkerOf(live)
	if !ok {
		return live, nil
	}
	kept, err := archived(marker)
	if err != nil {
		return nil, fmt.Errorf("reading %s's archived events: %w", stream, err)
	}
	all := append(slices.Clip(kept), live...)
	slices.SortStableFunc(all, func(a, b events.Event) int { return cmp.Compare(a.Position, b.Position) })
	return slices.CompactFunc(all, func(a, b events.Event) bool { return a.Position == b.Position }), nil
}

// MarkerOf is the marker of a stream archived now, from its live rows or
// its history: its last task.archived, unless a task.unarchived or a
// task.erased came after it. tasks.Archived finds the same marker: change the rule in both.
func MarkerOf(live []events.Event) (events.Event, bool) {
	for _, e := range slices.Backward(live) {
		switch e.Type {
		case events.TaskUnarchived, events.TaskErased:
			return events.Event{}, false
		case events.TaskArchived:
			return e, true
		}
	}
	return events.Event{}, false
}
