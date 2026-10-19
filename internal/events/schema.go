// Package events is what the log holds: the event types, the catalogue of
// what each one must carry, and a JSON Schema of each.
//
// The schemas live in internal/events/schemas, one <type>.json per type
// the catalogue serve opens the log with knows. They come from the Specs, by
// Schema, and aren't edited by hand: after a Spec changes, write them again
// with
//
//	go test ./internal/events -run TestTheSchemaFilesAreUpToDate -update
//
// and that test fails until they are. TestEveryFixtureEventFitsItsSchema
// checks every event of every .jsonl and .ndjson file under a testdata
// folder of the module against the committed file of its type, by Validate.
package events

import (
	"encoding/json"
	"slices"
)

// SchemaDialect is the JSON Schema draft every schema here is written in.
const SchemaDialect = "https://json-schema.org/draft/2020-12/schema"

// Schema is the JSON Schema of a stored event of one type, as Event
// marshals it. It holds only what the Spec says, and every object in it
// allows more properties: a type changes only by adding optional fields, so
// an event of any age, older or newer than this build, still validates.
// Writers are left out, as writer lists may narrow.
//
// Position and at are allowed but not required, because a fixture log
// holds proposals, which lack them. Data is required only when the type has
// fields, because a proposal with no data marshals without it; correlation
// likewise only when the type needs one.
func Schema(kind Type, spec Spec) map[string]any {
	required := []string{"stream", "actor", "type"}
	correlation := map[string]any{"type": "string"}
	if spec.Correlation {
		required = append(required, "correlation")
		correlation["minLength"] = 1
	}
	data := object(nil)
	if len(spec.Fields) > 0 {
		required = append(required, "data")
		data["required"] = slices.Clone(spec.Fields)
	}
	schema := object(map[string]any{
		"position":    map[string]any{"type": "integer", "minimum": 1},
		"at":          map[string]any{"type": "string", "format": "date-time"},
		"stream":      text(),
		"actor":       object(map[string]any{"kind": text(), "id": text()}, "kind", "id"),
		"type":        map[string]any{"const": kind},
		"correlation": correlation,
		"data":        data,
	}, required...)
	schema["$schema"] = SchemaDialect
	schema["title"] = kind
	return schema
}

// SchemaJSON is the Schema as the file it is kept in: indented, keys
// sorted, ending in a newline.
func SchemaJSON(kind Type, spec Spec) ([]byte, error) {
	encoded, err := json.MarshalIndent(Schema(kind, spec), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// object is the schema of a JSON object with these properties, the
// required ones among them, and room for any others.
func object(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "additionalProperties": true}
	if properties != nil {
		schema["properties"] = properties
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// text is the schema of a string that isn't empty, as CheckShape wants a
// stream and an actor.
func text() map[string]any { return map[string]any{"type": "string", "minLength": 1} }
