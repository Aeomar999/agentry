package schedules

// Source is every schedule, as kept from the log (current.State), read
// through the state's generic reads of an extension's streams.
type Source interface {
	Owned(stream string) (any, error) // nil, nil when the stream holds nothing
	OwnedIDs(prefix string) []string  // in log order, broken ones too
}

// Get is a schedule's state: nil, nil when there is no such schedule, and
// the error when its stream is broken.
func Get(source Source, id string) (*Schedule, error) {
	held, err := source.Owned(id)
	if err != nil {
		return nil, err
	}
	schedule, _ := held.(*Schedule)
	return schedule, nil
}

// All is every schedule, oldest first, removed ones too, in a new slice the
// caller may sort. A broken schedule is left out, as the task list leaves
// out a broken task.
func All(source Source) []*Schedule {
	var all []*Schedule
	for _, id := range source.OwnedIDs(Prefix) {
		if schedule, err := Get(source, id); err == nil && schedule != nil {
			all = append(all, schedule)
		}
	}
	return all
}
