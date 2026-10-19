package features

import (
	"reflect"
	"testing"
)

// A feature whose Manifest panics is listed hidden and disabled, saying
// why, and the features around it keep their own lines.
func TestAPanickingManifestCostsOnlyItsOwnLine(t *testing.T) {
	shown := Entry{Enabled: true, Visible: true, Why: "on", Badge: 2}
	list := []Feature{
		{Name: "before", Manifest: func(Reader) Entry { return shown }},
		{Name: "nil reader", Manifest: func(r Reader) Entry { r.Roster(); return shown }}, // a nil Reader: the call panics
		{Name: "plain"},
		{Name: "panics", Manifest: func(Reader) Entry { panic("a bad manifest") }},
		{Name: "last", Manifest: func(Reader) Entry { return shown }},
	}
	lines := Manifest(list, nil)
	want := []Line{
		{Name: "before", Entry: shown},
		{Name: "nil reader", Entry: Entry{Why: "its manifest failed: runtime error: invalid memory address or nil pointer dereference"}},
		{Name: "plain"},
		{Name: "panics", Entry: Entry{Why: "its manifest failed: a bad manifest"}},
		{Name: "last", Entry: shown},
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("lines:\n got %+v\nwant %+v", lines, want)
	}
}

// A Route that names no Auth is the page's only, never unguarded.
func TestTheZeroAuthIsUI(t *testing.T) {
	var route Route
	if route.Auth != UI || UI == Desk {
		t.Errorf("the zero Auth %+v should be UI %+v, and UI not Desk %+v", route.Auth, UI, Desk)
	}
}
