package hooks

import (
	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
)

// HoldOrphanedAsks runs when the server starts. An action waiting for your
// decision is held by its hook call, and no hook call survives a restart: the
// hook failed closed and the agent was already told no. So each such action
// is held for you, as one whose hook gave up is: you can still approve it,
// and its coordinator hands it back.
// A task the state can't hand out, such as a broken or an archived one, is
// skipped: it can't take an event either, and it must not keep the server
// from starting.
func HoldOrphanedAsks(s *store.Store, state *current.State) error {
	for _, id := range state.TaskIDs() {
		task, err := state.Task(id)
		if err != nil || task == nil || task.IsDone {
			continue
		}
		for _, action := range task.HooksWaiting() {
			if _, _, err := s.Propose(events.Proposal{Stream: id, Actor: ruleset, Type: events.ActionHeld, Correlation: action.ID}); err != nil {
				return err
			}
		}
	}
	return nil
}
