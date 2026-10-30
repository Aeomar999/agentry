package events_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
)

// schemaOf is the schema of a type as its file holds it, decoded.
func schemaOf(t *testing.T, kind events.Type, spec events.Spec) map[string]any {
	t.Helper()
	encoded, err := events.SchemaJSON(kind, spec)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

// decode is a JSON value as the fixture test decodes a line of a log.
func decode(t *testing.T, line string) any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAnEventValidatesAgainstItsSchema(t *testing.T) {
	done := schemaOf(t, events.TaskDone, events.Spec{Fields: []string{"summary"}})
	started := schemaOf(t, events.TurnStarted, events.Spec{Fields: []string{"agent"}, Correlation: true})
	held := schemaOf(t, events.TaskHeld, events.Spec{})
	rejected := schemaOf(t, events.ProposalRejected, events.Spec{})
	const actor = `"actor":{"kind":"agent","id":"editor"}`
	const rejection = `"data":{"type":"task.done","by":"agent:editor","check":"shape","reason":"no stream"}`

	for _, c := range []struct {
		name, event string
		schema      map[string]any
		problem     string // "" when the event is valid
	}{
		{"a good event", `{"stream":"task-4",` + actor + `,"type":"task.done","data":{"summary":"Done."}}`, done, ""},
		{"a stored event", `{"position":7,"at":"2026-10-19T08:00:00.5Z","stream":"task-4",` + actor + `,"type":"task.done","data":{"summary":"Done."}}`, done, ""},
		{"a missing data field", `{"stream":"task-4",` + actor + `,"type":"task.done","data":{}}`, done, "data.summary: required field missing"},
		{"no data where fields are needed", `{"stream":"task-4",` + actor + `,"type":"task.done"}`, done, "data: required field missing"},
		{"a missing correlation", `{"stream":"task-4",` + actor + `,"type":"turn.started","data":{"agent":"editor"}}`, started, "correlation: required field missing"},
		{"an empty correlation", `{"stream":"task-4",` + actor + `,"type":"turn.started","correlation":"","data":{"agent":"editor"}}`, started, "correlation: want at least 1 characters"},
		{"a correlation", `{"stream":"task-4",` + actor + `,"type":"turn.started","correlation":"turn-1","data":{"agent":"editor"}}`, started, ""},
		{"no correlation where none is needed", `{"stream":"task-4",` + actor + `,"type":"task.done","data":{"summary":"Done."}}`, done, ""},
		{"an empty correlation where none is needed", `{"stream":"task-4",` + actor + `,"type":"task.done","correlation":"","data":{"summary":"Done."}}`, done, ""},
		{"the wrong type", `{"stream":"task-4",` + actor + `,"type":"task.held","data":{"summary":"Done."}}`, done, `type: want task.done, got "task.held"`},
		{"fields newer than the schema", `{"stream":"task-4",` + actor + `,"type":"task.done","origin":"web","data":{"summary":"Done.","toYou":"Look."}}`, done, ""},
		{"no data for a type with no fields", `{"stream":"task-4",` + actor + `,"type":"task.held"}`, held, ""},
		{"a rejection with no stream", `{"stream":"",` + actor + `,"type":"proposal.rejected",` + rejection + `}`, rejected, ""},
		{"an empty stream elsewhere", `{"stream":"",` + actor + `,"type":"task.done","data":{"summary":"Done."}}`, done, "stream: want at least 1 characters"},
		{"an empty actor id", `{"stream":"task-4","actor":{"kind":"agent","id":""},"type":"task.held"}`, held, "actor.id: want at least 1 characters"},
		{"a position that isn't whole", `{"position":1.5,"stream":"task-4",` + actor + `,"type":"task.held"}`, held, "position: want an integer"},
		{"a time that isn't one", `{"at":"yesterday","stream":"task-4",` + actor + `,"type":"task.held"}`, held, `at: want a date-time, got "yesterday"`},
		{"not an object", `["task.held"]`, held, "event: want an object, got a list"},
	} {
		err := events.Validate(c.schema, decode(t, c.event))
		switch {
		case c.problem == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.problem != "" && err == nil:
			t.Errorf("%s validates", c.name)
		case c.problem != "" && !strings.Contains(err.Error(), c.problem):
			t.Errorf("%s: got %q, want %q", c.name, err, c.problem)
		}
	}
}

// Validate takes Schema's output as it is, not only a decoded schema file.
func TestAnEventValidatesAgainstSchemasOwnOutput(t *testing.T) {
	schema := events.Schema(events.TurnStarted, events.Spec{Fields: []string{"agent"}, Correlation: true})
	good := decode(t, `{"stream":"task-4","actor":{"kind":"agent","id":"editor"},"type":"turn.started","correlation":"turn-1","data":{"agent":"editor"}}`)
	if err := events.Validate(schema, good); err != nil {
		t.Errorf("a good event: %v", err)
	}
	bad := decode(t, `{"stream":"task-4","actor":{"kind":"agent","id":""},"type":"turn.started","correlation":"turn-1","data":{}}`)
	err := events.Validate(schema, bad)
	for _, problem := range []string{"actor.id: want at least 1 characters", "data.agent: required field missing"} {
		if err == nil || !strings.Contains(err.Error(), problem) {
			t.Errorf("a bad event: got %v, want %q", err, problem)
		}
	}
}

// Validate refuses a keyword Schema doesn't write, even where the event
// lacks the field, so the two can't drift apart unseen.
func TestASchemaWithAnUnknownKeywordIsRefused(t *testing.T) {
	event := decode(t, `{"stream":"task-4","actor":{"kind":"agent","id":"editor"},"type":"task.held"}`)
	for name, change := range map[string]func(schema map[string]any){
		"a new keyword":      func(schema map[string]any) { schema["maxLength"] = 10 },
		"one inside a field": func(schema map[string]any) { field(schema, "at")["pattern"] = "^2" },
		"another format":     func(schema map[string]any) { field(schema, "at")["format"] = "date" },
		"another type":       func(schema map[string]any) { field(schema, "position")["type"] = "number" },
		"no more properties": func(schema map[string]any) { schema["additionalProperties"] = false },
	} {
		schema := schemaOf(t, events.TaskHeld, events.Spec{})
		change(schema)
		if err := events.Validate(schema, event); err == nil || !strings.Contains(err.Error(), "the schema") {
			t.Errorf("%s: got %v, want the schema refused", name, err)
		}
	}
}

func field(schema map[string]any, name string) map[string]any {
	return schema["properties"].(map[string]any)[name].(map[string]any)
}

// Every committed schema is one Validate can check: an event with just what
// its Spec needs passes it. An unknown type has neither a Spec nor a file.
func TestEveryCommittedSchemaChecksAnEvent(t *testing.T) {
	catalogue, err := events.NewCatalogue(extensions.Registration(ideas.Extension()))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range catalogue.Types() {
		spec, _ := catalogue.Spec(kind)
		event := map[string]any{"stream": "task-4", "actor": map[string]any{"kind": "agent", "id": "editor"}, "type": string(kind)}
		if spec.Correlation {
			event["correlation"] = "c-1"
		}
		if len(spec.Fields) > 0 {
			data := map[string]any{}
			for _, name := range spec.Fields {
				data[name] = ""
			}
			event["data"] = data
		}
		if err := events.Validate(schemaFile(t, filepath.Join(schemas, string(kind)+".json")), event); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}

	if _, known := catalogue.Spec("task.exploded"); known {
		t.Error("an unknown type has a spec")
	}
	if _, err := os.Stat(filepath.Join(schemas, "task.exploded.json")); !os.IsNotExist(err) {
		t.Errorf("an unknown type has a schema file: %v", err)
	}
}

// schemaFile is a committed schema, decoded.
func schemaFile(t *testing.T, path string) map[string]any {
	t.Helper()
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return schema
}
