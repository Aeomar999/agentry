package sharedfiles

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFilesGoRoundBetweenWorktrees(t *testing.T) {
	blobs := Disk{Dir: t.TempDir()}
	first, second := t.TempDir(), t.TempDir()
	write(t, filepath.Join(Dir(first), "data", "sales.csv"), "a,b\n1,2\n")
	write(t, filepath.Join(Dir(first), "notes.md"), "# notes")
	write(t, filepath.Join(Dir(first), EpicFolder, "brief.md"), "the epic's")

	found, err := Manifest(first)
	if err != nil || len(found) != 2 {
		t.Fatalf("manifest: %v %+v", err, found)
	}
	sent, err := Upload(first, []string{"data/sales.csv", "notes.md"}, blobs)
	if err != nil || sent[0].Hash != found[0].Hash || !blobs.Has(sent[1].Hash) {
		t.Fatalf("upload: %v %+v", err, sent)
	}
	if again, _, _ := blobs.Put(strings.NewReader("# notes")); again != sent[1].Hash {
		t.Error("the same content should be kept once, by its hash")
	}

	write(t, filepath.Join(Dir(second), "stale.txt"), "left from before")
	if err := Deliver(second, sent, blobs); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(Dir(second), "data", "sales.csv")); string(got) != "a,b\n1,2\n" {
		t.Errorf("delivered: %q", got)
	}
	if _, err := os.Stat(filepath.Join(Dir(second), "stale.txt")); err == nil {
		t.Error("a file not shared any more should go")
	}
}

func TestFetchAsksOutsideTheBoundary(t *testing.T) {
	blobs := Disk{Dir: t.TempDir()}
	inside, outside := t.TempDir(), t.TempDir()
	write(t, filepath.Join(inside, "build", "app.bin"), "binary")
	write(t, filepath.Join(outside, "secret.txt"), "private")
	if _, err := Fetch(filepath.Join(outside, "secret.txt"), inside, "", blobs); !errors.Is(err, ErrOutside) {
		t.Errorf("outside the boundary: %v", err)
	}
	got, err := Fetch(filepath.Join(inside, "build"), inside, "out", blobs)
	if err != nil || len(got) != 1 || got[0].Name != "out/app.bin" {
		t.Errorf("a folder inside: %v %+v", err, got)
	}
	if got, err := Fetch(filepath.Join(outside, "secret.txt"), "", "", blobs); err != nil || got[0].Name != "secret.txt" {
		t.Errorf("approved, from anywhere: %v %+v", err, got)
	}
}

func TestNamesStayInsideTheFolder(t *testing.T) {
	for _, bad := range []string{"../escape", "/etc/passwd", "..", ""} {
		if _, err := CleanName(bad); err == nil {
			t.Errorf("%q was taken as a name", bad)
		}
	}
	if name, _ := CleanName(`data\sales.csv`); name != "data/sales.csv" {
		t.Errorf("a Windows name: %q", name)
	}
}
