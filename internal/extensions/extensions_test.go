package extensions

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// An extension whose Manifest panics is listed hidden and disabled, saying
// why, and logged once; the extensions around it keep their own lines.
func TestAPanickingManifestCostsOnlyItsOwnLine(t *testing.T) {
	var said bytes.Buffer
	before := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&said, nil)))
	t.Cleanup(func() { slog.SetDefault(before) })

	shown := Entry{Enabled: true, Visible: true, Why: "on", Badge: 2}
	list := []Extension{
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
	logged := strings.Split(strings.TrimSpace(said.String()), "\n")
	if len(logged) != 2 || !strings.Contains(logged[0], "extension=\"nil reader\"") || !strings.Contains(logged[1], "extension=panics panic=\"a bad manifest\"") {
		t.Errorf("want one line for each panicking manifest, got:\n%s", said.String())
	}
}

// StreamsOf lists the streams of the extensions that own some, in order.
func TestStreamsOfSkipsAnExtensionWithNone(t *testing.T) {
	a, b := &Streams{Prefix: "a-"}, &Streams{Prefix: "b-"}
	got := StreamsOf(Extension{Name: "a", Streams: a}, Extension{Name: "none"}, Extension{Name: "b", Streams: b})
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("StreamsOf = %v, want a's and b's", got)
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

// writable is a store: it reads and writes the log.
type writable struct{ appended int }

func (w *writable) Stream(string) ([]events.Event, error) { return nil, nil }
func (w *writable) Append(events.Event) error             { w.appended++; return nil }

// What ReadOnly returns gives an extension no way back to the store under it.
func TestReadOnlyHidesTheStore(t *testing.T) {
	log := ReadOnly(&writable{})
	if _, found := log.(*writable); found {
		t.Error("ReadOnly's value is the store")
	}
	if _, found := log.(interface{ Append(events.Event) error }); found {
		t.Error("ReadOnly's value can write")
	}
	value := reflect.ValueOf(log)
	for i := range value.NumField() {
		if field := value.Type().Field(i); field.IsExported() {
			t.Errorf("ReadOnly's value has the exported field %s", field.Name)
		}
	}
}
