package events_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// module is the root of the module, from this package's folder.
const module = "../.."

// Every event in every fixture log, any .jsonl or .ndjson file under a
// testdata folder of the module, validates against the committed schema
// file of its type. A new fixture log is checked without being listed here.
func TestEveryFixtureEventFitsItsSchema(t *testing.T) {
	logs := fixtureLogs(t)
	if len(logs) == 0 {
		t.Fatalf("no fixture logs under %s", module)
	}
	for _, path := range logs {
		checkFixtureLog(t, path)
	}
}

// fixtureLogs are the event logs under the module's testdata folders.
// Folders whose names start with a dot, and node_modules, hold no fixtures.
func fixtureLogs(t *testing.T) []string {
	t.Helper()
	var logs []string
	err := filepath.WalkDir(module, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != module && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		extension := filepath.Ext(name)
		inTestdata := strings.Contains(filepath.ToSlash(path), "/testdata/")
		if inTestdata && (extension == ".jsonl" || extension == ".ndjson") {
			logs = append(logs, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return logs
}

// checkFixtureLog validates each line of a log, naming the file and line of
// any event that doesn't fit.
func checkFixtureLog(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := filepath.Rel(module, path)
	lines := bufio.NewScanner(bytes.NewReader(content))
	lines.Buffer(nil, len(content)+1)
	for number := 1; lines.Scan(); number++ {
		line := lines.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.UseNumber()
		var event map[string]any
		if err := decoder.Decode(&event); err != nil {
			t.Errorf("%s:%d: %v", name, number, err)
			continue
		}
		kind, _ := event["type"].(string)
		file := filepath.Join(schemas, kind+".json")
		if _, err := os.Stat(file); kind == "" || err != nil {
			t.Errorf("%s:%d: type %q has no schema file in %s", name, number, kind, schemas)
			continue
		}
		if err := events.Validate(schemaFile(t, file), event); err != nil {
			t.Errorf("%s:%d: %s: %v", name, number, kind, err)
		}
	}
	if err := lines.Err(); err != nil {
		t.Errorf("%s: %v", name, err)
	}
}
