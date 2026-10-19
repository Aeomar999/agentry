package sharedfiles

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// synced records what Put syncs, in order, and whether the blob was in
// its place at the time.
type synced struct {
	names   []string
	inPlace []bool
}

func watchSyncs(t *testing.T, blob func() string) *synced {
	t.Helper()
	seen := &synced{}
	syncFile = func(f *os.File) error {
		_, err := os.Stat(blob())
		seen.names = append(seen.names, f.Name())
		seen.inPlace = append(seen.inPlace, err == nil)
		return f.Sync()
	}
	t.Cleanup(func() { syncFile = (*os.File).Sync })
	return seen
}

// Put syncs the file before it is in its place, then the folder it went
// into and the folder holding that one, so a crash can't lose a blob an
// event already names. A blob kept already gets its folders synced again:
// the Put that kept it may have failed before it synced them.
func TestPutSyncsTheBlobThenItsFolders(t *testing.T) {
	blobs := Disk{Dir: t.TempDir()}
	hash, _, err := blobs.Put(strings.NewReader("rows"))
	if err != nil {
		t.Fatal(err)
	}
	blob := blobs.path(hash)
	for _, kept := range []bool{true, false} {
		if !kept {
			os.Remove(blob)
		}
		seen := watchSyncs(t, func() string { return blob })
		if _, _, err := blobs.Put(strings.NewReader("rows")); err != nil {
			t.Fatal(err)
		}
		if len(seen.names) != 3 || !strings.HasPrefix(filepath.Base(seen.names[0]), "incoming-") ||
			!slices.Equal(seen.names[1:], []string{filepath.Dir(blob), blobs.Dir}) || !slices.Equal(seen.inPlace, []bool{kept, true, true}) {
			t.Errorf("kept already %v: synced %v, the blob in place %v; want the incoming file, then %s and %s with the blob in place",
				kept, seen.names, seen.inPlace, filepath.Dir(blob), blobs.Dir)
		}
	}
}

// The same bytes make the same blob: a Put after a crash that left the
// blob from the round before gives the same name, and keeps one file.
func TestTheSameBytesMakeTheSameBlob(t *testing.T) {
	blobs := Disk{Dir: t.TempDir()}
	first, size, err := blobs.Put(strings.NewReader("the task's rows"))
	if err != nil {
		t.Fatal(err)
	}
	second, again, err := blobs.Put(strings.NewReader("the task's rows"))
	if err != nil || second != first || again != size {
		t.Fatalf("put again: %s %d %v, want %s %d", second, again, err, first, size)
	}
	var files []string
	filepath.WalkDir(blobs.Dir, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			files = append(files, filepath.Base(path))
		}
		return err
	})
	if !slices.Equal(files, []string{first}) {
		t.Errorf("files kept %v, want the one blob", files)
	}
}
