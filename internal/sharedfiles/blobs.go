// Package sharedfiles is a task's shared files: what its agents, on any
// machine, and you hand each other that isn't in git, such as a dataset, a
// build or a document. Each file is kept once on the server, by the hash of
// what is in it (Disk); a machine reaches them there (Hub). In a worktree
// they are the folder .agentry/files (Folder), which Agentry fills before a
// turn and reads back after it, so the next agent has them wherever it runs.
package sharedfiles

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// MaxSize is the largest file a task can share.
const MaxSize = 2 << 30

// ErrTooBig is a file over MaxSize.
var ErrTooBig = fmt.Errorf("a shared file is at most %d GB", MaxSize>>30)

// Blobs keep files by the hash of what is in them.
type Blobs interface {
	Has(hash string) bool
	Open(hash string) (io.ReadCloser, error)
	// Put keeps what r gives and returns its hash and size.
	Put(r io.Reader) (hash string, size int64, err error)
}

// Disk is the server's blobs, in a folder.
type Disk struct{ Dir string }

var isHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (d Disk) path(hash string) string { return filepath.Join(d.Dir, hash[:2], hash) }

func (d Disk) Has(hash string) bool {
	if !isHash.MatchString(hash) {
		return false
	}
	_, err := os.Stat(d.path(hash))
	return err == nil
}

func (d Disk) Open(hash string) (io.ReadCloser, error) {
	if !isHash.MatchString(hash) {
		return nil, errors.New("not a file's hash")
	}
	return os.Open(d.path(hash))
}

// Put keeps a file once it is on disk for good: the file is synced before
// it is renamed into place, and its folder after, so an event that names
// it, committed once Put returns, never outlives it in a crash
// (docs/design/log-growth.md section 3). The same bytes make the same
// file, so a Put retried after a crash finds it kept.
func (d Disk) Put(r io.Reader) (string, int64, error) {
	if err := os.MkdirAll(d.Dir, 0o700); err != nil {
		return "", 0, err
	}
	temporary, err := os.CreateTemp(d.Dir, "incoming-*")
	if err != nil {
		return "", 0, err
	}
	defer os.Remove(temporary.Name())
	sum := sha256.New()
	size, err := io.Copy(io.MultiWriter(temporary, sum), io.LimitReader(r, MaxSize+1))
	if err == nil {
		err = syncFile(temporary)
	}
	temporary.Close()
	switch {
	case err != nil:
		return "", 0, err
	case size > MaxSize:
		return "", 0, ErrTooBig
	}
	hash := hex.EncodeToString(sum.Sum(nil))
	if !d.Has(hash) {
		if err := os.MkdirAll(filepath.Dir(d.path(hash)), 0o700); err != nil {
			return "", 0, err
		}
		if err := os.Rename(temporary.Name(), d.path(hash)); err != nil {
			return "", 0, err
		}
	}
	// Kept already, the same file shared again, may be a Put's that failed
	// before its folders were synced: sync them either way.
	return hash, size, d.syncFolders(hash)
}

// syncFolders makes the file's name last: its own folder, the one holding
// that folder, and the one holding d.Dir, as Put may have just made either.
func (d Disk) syncFolders(hash string) error {
	for _, folder := range []string{filepath.Dir(d.path(hash)), d.Dir, filepath.Dir(d.Dir)} {
		opened, err := os.Open(folder)
		if err != nil {
			return err
		}
		err = syncFile(opened)
		opened.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// syncFile is (*os.File).Sync; a test watches it.
var syncFile = (*os.File).Sync

// HashOf is the hash a file would be kept by.
func HashOf(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	sum := sha256.New()
	size, err := io.Copy(sum, file)
	return hex.EncodeToString(sum.Sum(nil)), size, err
}
