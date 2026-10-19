package features

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
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
	if route.Auth.DeskToo() || route.Auth.Verify() != nil || UI.DeskToo() || UI.Verify() != nil {
		t.Errorf("the zero Auth and UI should let in neither the desk nor a signature")
	}
	if !Desk.DeskToo() || Desk.Verify() != nil {
		t.Errorf("Desk should let in the desk, and check no signature")
	}
}

// A signed route is opened by its signature alone, never by the desk.
func TestSignedKeepsItsVerify(t *testing.T) {
	refused := errors.New("refused")
	auth := Signed(func(*http.Request, []byte) error { return refused })
	if auth.DeskToo() || auth.Verify() == nil || auth.Verify()(nil, nil) != refused {
		t.Errorf("Signed should keep its verify and not let in the desk")
	}
}

// Signed with no verify fails closed: it panics, so no route checks nothing.
func TestSignedWithNoVerifyPanics(t *testing.T) {
	defer func() {
		if message, _ := recover().(string); !strings.Contains(message, "needs a verify") {
			t.Errorf("Signed(nil) panicked with %q, want it to say it needs a verify", message)
		}
	}()
	Signed(nil)
}
