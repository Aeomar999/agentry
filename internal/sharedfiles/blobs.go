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
	temporary.Close()
	switch {
	case err != nil:
		return "", 0, err
	case size > MaxSize:
		return "", 0, ErrTooBig
	}
	hash := hex.EncodeToString(sum.Sum(nil))
	if d.Has(hash) {
		return hash, size, nil // kept already: the same file, shared again
	}
	if err := os.MkdirAll(filepath.Dir(d.path(hash)), 0o700); err != nil {
		return "", 0, err
	}
	return hash, size, os.Rename(temporary.Name(), d.path(hash))
}

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
