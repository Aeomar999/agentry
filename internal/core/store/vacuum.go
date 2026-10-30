package store

import (
	"database/sql"
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// Archiving deletes rows, which frees pages inside the file but doesn't
// shrink it. A file in incremental auto_vacuum mode gives them back after
// each round; an older file switches to that mode only through one full
// VACUUM (docs/design/log-growth.md section 4, "VACUUM").

// incremental is PRAGMA auto_vacuum's number for INCREMENTAL.
const incremental = 2

// GiveBackSpace gives the file's free pages back to the disk, and empties
// the WAL into the file, so neither keeps the pages of the rows an
// archiving round or an erasure deleted, or the old text of rows a
// redaction rewrote. A reader in the middle of a read keeps the WAL from
// being emptied, so Erase and Redact, which run it themselves, may leave
// it full; then it is run again: by the archiver after a round that
// archived a task, by serve once at start, and by the erase and redact
// routes before they answer (step 10d2).
func (s *Store) GiveBackSpace() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.giveBackSpace()
}

// giveBackSpace is GiveBackSpace, under Store.mu.
func (s *Store) giveBackSpace() error {
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
	if archived, err := anyArchived(s.db); err != nil || !archived {
		return false, err
	}
	// The connection set auto_vacuum to INCREMENTAL as it opened; VACUUM
	// is what makes that the file's mode.
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return false, fmt.Errorf("vacuuming the log: %w", err)
	}
	return true, nil
}

// zeroedFrom is the first schema version whose builds delete with
// secure_delete on.
const zeroedFrom = 2

// clearFreedCells runs one full VACUUM on a file below zeroedFrom that
// has archived a task, before it is migrated. Its builds deleted the
// archived rows with secure_delete off, which left their bytes in freed
// cells on pages still in use: an erasure, zeroing only its own rows, and
// incremental_vacuum, freeing only whole pages, never reach them. VACUUM
// copies only the live rows. It runs before the migration, which can't
// hold it in its transaction, so a VACUUM that fails leaves the version
// as it was, and the next Open tries again. Being full, it also switches
// the file to incremental mode, so VacuumOnce has nothing left to do.
func clearFreedCells(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version >= zeroedFrom {
		return nil
	}
	if archived, err := anyArchived(db); err != nil || !archived {
		return err
	}
	if _, err := db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuuming the log: %w", err)
	}
	return nil
}

// anyArchived says whether the log holds a task.archived marker.
func anyArchived(db *sql.DB) (bool, error) {
	var archived bool
	err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM events WHERE type = ?)`, events.TaskArchived).Scan(&archived)
	return archived, err
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
