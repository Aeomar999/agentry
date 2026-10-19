package store

import (
	"database/sql"
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Archiving deletes rows, which frees pages inside the file but doesn't
// shrink it. A file in incremental auto_vacuum mode gives them back after
// each round; an older file switches to that mode only through one full
// VACUUM (docs/design/log-growth.md section 4, "VACUUM").

// incremental is PRAGMA auto_vacuum's number for INCREMENTAL.
const incremental = 2

// GiveBackSpace gives the file's free pages back to the disk, and empties
// the WAL into the file, so neither keeps the pages of the rows an
// archiving round deleted. A reader in the middle of a read keeps the WAL
// from being emptied: the next round empties it.
func (s *Store) GiveBackSpace() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := incrementalVacuum(s.db); err != nil {
		return fmt.Errorf("giving back free pages: %w", err)
	}
	var busy, walPages, checkpointed int
	if err := s.db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &walPages, &checkpointed); err != nil {
		return fmt.Errorf("emptying the WAL: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("emptying the WAL: a reader held it, %d of %d pages copied", checkpointed, walPages)
	}
	return nil
}

// VacuumOnce runs the one full VACUUM a file made before incremental mode
// needs to switch to it, once a task has been archived, when the copy it
// makes holds only the live rows. It says whether it ran: not for a file
// already incremental, which every file made since is, nor before the
// first marker. It holds the lock throughout, so no proposal is written
// while it copies, and needs free disk as big as the copy. A failure,
// such as a full disk, leaves the file as it was.
func (s *Store) VacuumOnce() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var mode int
	if err := s.db.QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		return false, err
	}
	if mode == incremental {
		return false, nil
	}
	var archived bool
	if err := s.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM events WHERE type = ?)`, events.TaskArchived).Scan(&archived); err != nil {
		return false, err
	}
	if !archived {
		return false, nil
	}
	// The connection set auto_vacuum to INCREMENTAL as it opened; VACUUM
	// is what makes that the file's mode.
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return false, fmt.Errorf("vacuuming the log: %w", err)
	}
	return true, nil
}

// incrementalVacuum runs PRAGMA incremental_vacuum to the end. SQLite
// gives the pages back as the statement is stepped: through Exec it gave
// back none (TestGiveBackSpaceShrinksTheFileAndTheWAL).
func incrementalVacuum(db *sql.DB) error {
	freed, err := db.Query(`PRAGMA incremental_vacuum`)
	if err != nil {
		return err
	}
	defer freed.Close()
	for freed.Next() {
	}
	return freed.Err()
}
