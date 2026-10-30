package storetest

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FilesHolding are the files under dir whose bytes hold text, a gzipped
// file's, such as an archive's blob, read unzipped too.
func FilesHolding(t *testing.T, dir, text string) []string {
	t.Helper()
	var holding []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if unzipped, err := gzip.NewReader(bytes.NewReader(content)); err == nil {
			if content, err = io.ReadAll(unzipped); err != nil {
				return fmt.Errorf("unzipping %s: %w", path, err)
			}
		}
		if bytes.Contains(content, []byte(text)) {
			holding = append(holding, filepath.Base(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return holding
}

// writtenAs are the ways a file holds text: as it is, as a row's data
// writes it, and as a blob writes that data again.
func writtenAs(text string) []string {
	once := strings.Trim(asJSON(text), `"`)
	return []string{text, once, strings.Trim(asJSON(once), `"`)}
}

// HoldingAny are the files under dir holding text in any way it is
// written.
func HoldingAny(t *testing.T, dir, text string) []string {
	t.Helper()
	var holding []string
	for _, form := range writtenAs(text) {
		holding = append(holding, FilesHolding(t, dir, form)...)
	}
	return holding
}

func asJSON(text string) string {
	encoded, _ := json.Marshal(text) // a string always encodes
	return string(encoded)
}
