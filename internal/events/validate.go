package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"time"
	"unicode/utf8"
)

// Validate checks a JSON value against a schema, both as encoding/json
// decodes them. It knows only the keywords Schema writes, and refuses a
// schema with any other, so Schema can't start writing one that goes
// unchecked. Each problem names where in the value it is, as
// "data.summary: required field missing".
func Validate(schema map[string]any, value any) error {
	problems, err := validate(schema, value, "")
	if err != nil {
		return err
	}
	return errors.Join(problems...)
}

// A check is one keyword's: the problems with the value at path, or what
// is wrong with the keyword's rule.
type check func(rule any, value any, path string) ([]error, error)

// checkOf is the check of each keyword Validate knows. $schema and title are
// notes for readers and check nothing.
func checkOf(keyword string) (check, bool) {
	switch keyword {
	case "$schema", "title":
		return func(any, any, string) ([]error, error) { return nil, nil }, true
	case "type":
		return checkType, true
	case "required":
		return checkRequired, true
	case "properties":
		return checkProperties, true
	case "additionalProperties":
		return checkAdditional, true
	case "const":
		return checkConst, true
	case "minLength":
		return checkMinLength, true
	case "minimum":
		return checkMinimum, true
	case "format":
		return checkFormat, true
	}
	return nil, false
}

// validate is every problem with the value at path, or the error with the
// schema: a keyword it doesn't know, or doesn't know in that form.
func validate(schema map[string]any, value any, path string) ([]error, error) {
	var problems []error
	for _, keyword := range slices.Sorted(maps.Keys(schema)) {
		check, known := checkOf(keyword)
		if !known {
			return nil, fmt.Errorf("%s: the schema has %q, which Validate doesn't know", where(path), keyword)
		}
		found, err := check(schema[keyword], value, path)
		if err != nil {
			return nil, fmt.Errorf("%s: the schema's %s: %w", where(path), keyword, err)
		}
		problems = append(problems, found...)
	}
	return problems, nil
}

func checkType(rule any, value any, path string) ([]error, error) {
	var fits bool
	var want string
	switch rule {
	case "object":
		_, fits = value.(map[string]any)
		want = "an object"
	case "string":
		_, fits = value.(string)
		want = "a string"
	case "integer":
		number, isNumber := asNumber(value)
		fits = isNumber && number == math.Trunc(number)
		want = "an integer"
	default:
		return nil, fmt.Errorf("type %v isn't one Validate knows", rule)
	}
	if !fits {
		return []error{fmt.Errorf("%s: want %s, got %s", where(path), want, describe(value))}, nil
	}
	return nil, nil
}

func checkRequired(rule any, value any, path string) ([]error, error) {
	fields, ok := rule.([]any)
	if !ok {
		return nil, fmt.Errorf("want a list of names, got %v", rule)
	}
	object, isObject := value.(map[string]any)
	var problems []error
	for _, field := range fields {
		name, ok := field.(string)
		if !ok {
			return nil, fmt.Errorf("want a list of names, got %v", rule)
		}
		if _, present := object[name]; isObject && !present {
			problems = append(problems, fmt.Errorf("%s: required field missing", join(path, name)))
		}
	}
	return problems, nil
}

func checkProperties(rule any, value any, path string) ([]error, error) {
	properties, ok := rule.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("want an object of schemas, got %v", rule)
	}
	object, isObject := value.(map[string]any)
	var problems []error
	for _, name := range slices.Sorted(maps.Keys(properties)) {
		schema, ok := properties[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s isn't a schema", name)
		}
		field, present := object[name]
		found, err := validate(schema, field, join(path, name))
		if err != nil {
			return nil, err
		}
		// A field the value lacks has its schema checked all the same, so
		// a keyword Validate doesn't know is found on any event.
		if isObject && present {
			problems = append(problems, found...)
		}
	}
	return problems, nil
}

// Schema allows more properties everywhere, so Validate knows no other form.
func checkAdditional(rule any, _ any, _ string) ([]error, error) {
	if rule != true {
		return nil, fmt.Errorf("only true is known, got %v", rule)
	}
	return nil, nil
}

func checkConst(rule any, value any, path string) ([]error, error) {
	if !reflect.DeepEqual(rule, value) {
		return []error{fmt.Errorf("%s: want %v, got %s", where(path), rule, describe(value))}, nil
	}
	return nil, nil
}

func checkMinLength(rule any, value any, path string) ([]error, error) {
	least, ok := asNumber(rule)
	if !ok {
		return nil, fmt.Errorf("want a number, got %v", rule)
	}
	text, isString := value.(string)
	if isString && float64(utf8.RuneCountInString(text)) < least {
		return []error{fmt.Errorf("%s: want at least %v characters, got %q", where(path), least, text)}, nil
	}
	return nil, nil
}

func checkMinimum(rule any, value any, path string) ([]error, error) {
	least, ok := asNumber(rule)
	if !ok {
		return nil, fmt.Errorf("want a number, got %v", rule)
	}
	if number, isNumber := asNumber(value); isNumber && number < least {
		return []error{fmt.Errorf("%s: want at least %v, got %v", where(path), least, number)}, nil
	}
	return nil, nil
}

func checkFormat(rule any, value any, path string) ([]error, error) {
	if rule != "date-time" {
		return nil, fmt.Errorf("format %v isn't one Validate knows", rule)
	}
	text, isString := value.(string)
	if !isString {
		return nil, nil
	}
	if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
		return []error{fmt.Errorf("%s: want a date-time, got %q", where(path), text)}, nil
	}
	return nil, nil
}

// asNumber is a JSON number as a float64, decoded either way encoding/json
// can.
func asNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	}
	return 0, false
}

// describe names what kind of JSON value this is.
func describe(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "an object"
	case []any:
		return "a list"
	case string:
		return fmt.Sprintf("%q", value)
	case bool:
		return "a boolean"
	}
	if _, isNumber := asNumber(value); isNumber {
		return fmt.Sprintf("the number %v", value)
	}
	return fmt.Sprintf("%v", value)
}

func join(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}

// where is the path, or "event" for the value itself.
func where(path string) string {
	if path == "" {
		return "event"
	}
	return path
}
