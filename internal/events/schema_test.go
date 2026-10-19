package events_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
)

var update = flag.Bool("update", false, "rewrite the schema files from the catalogue as it is now")

// schemas is where the schema of each type serve registers is kept, one
// file per type.
const schemas = "schemas"

func TestTheCatalogueListsItsTypesAndSpecs(t *testing.T) {
	catalogue, err := events.NewCatalogue(ideas.Registration())
	if err != nil {
		t.Fatal(err)
	}
	types := catalogue.Types()
	if !slices.IsSorted(types) {
		t.Errorf("types aren't sorted: %v", types)
	}
	for _, kind := range []events.Type{events.TaskCreated, ideas.IdeaSuggested} {
		if !slices.Contains(types, kind) {
			t.Errorf("%s isn't among the types", kind)
		}
	}
	if slices.Contains(events.Core().Types(), ideas.IdeaSuggested) {
		t.Errorf("the core alone lists %s", ideas.IdeaSuggested)
	}
	spec, known := catalogue.Spec(events.TurnStarted)
	if !known || !spec.Correlation || !slices.Equal(spec.Fields, []string{"agent", "hash"}) {
		t.Errorf("turn.started: %+v %v", spec, known)
	}
	if _, known := catalogue.Spec("task.exploded"); known {
		t.Error("an unknown type has a spec")
	}
}

func TestASchemaHoldsWhatTheSpecSays(t *testing.T) {
	required := func(schema map[string]any) []string { return schema["required"].([]string) }
	data := func(schema map[string]any) map[string]any {
		return schema["properties"].(map[string]any)["data"].(map[string]any)
	}

	created := events.Schema(events.TaskCreated, events.Spec{Fields: []string{"title", "repo"}})
	if got := required(created); !slices.Equal(got, []string{"stream", "actor", "type", "data"}) {
		t.Errorf("task.created requires %v", got)
	}
	if got := required(data(created)); !slices.Equal(got, []string{"title", "repo"}) {
		t.Errorf("task.created's data requires %v", got)
	}

	started := events.Schema(events.TurnStarted, events.Spec{Fields: []string{"agent"}, Correlation: true})
	if !slices.Contains(required(started), "correlation") {
		t.Errorf("turn.started doesn't require its correlation: %v", required(started))
	}

	held := events.Schema(events.TaskHeld, events.Spec{})
	if got := required(held); !slices.Equal(got, []string{"stream", "actor", "type"}) {
		t.Errorf("task.held, with no fields, requires %v", got)
	}
	if _, has := data(held)["required"]; has {
		t.Errorf("task.held's data requires %v", data(held)["required"])
	}
}

// The committed schemas are what the catalogue serve opens the log with
// gives now: one file per type, none missing and none left over. Run with
// -update to write them again after a Spec changes.
func TestTheSchemaFilesAreUpToDate(t *testing.T) {
	catalogue, err := events.NewCatalogue(ideas.Registration())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]byte{}
	for _, kind := range catalogue.Types() {
		spec, _ := catalogue.Spec(kind)
		if want[string(kind)+".json"], err = events.SchemaJSON(kind, spec); err != nil {
			t.Fatal(err)
		}
	}
	if *update {
		writeSchemas(t, want)
	}

	files, err := os.ReadDir(schemas)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if _, kept := want[file.Name()]; !kept {
			t.Errorf("%s has no type in the catalogue", filepath.Join(schemas, file.Name()))
		}
	}
	for name, schema := range want {
		path := filepath.Join(schemas, name)
		got, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			t.Errorf("%s is missing", path)
		case err != nil:
			t.Error(err)
		case !bytes.Equal(got, schema):
			t.Errorf("%s differs from what the catalogue gives:\n%s", path, schema)
		}
	}
}

// writeSchemas makes the schemas folder hold these files and no others.
func writeSchemas(t *testing.T, want map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(schemas, 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(schemas)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if _, kept := want[file.Name()]; !kept && strings.HasSuffix(file.Name(), ".json") {
			if err := os.Remove(filepath.Join(schemas, file.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, schema := range want {
		if err := os.WriteFile(filepath.Join(schemas, name), schema, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
