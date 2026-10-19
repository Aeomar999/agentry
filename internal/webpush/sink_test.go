package webpush

import (
	"context"
	"crypto/ecdh"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/features"
)

// inbox is a fake push service that keeps every push by its path,
// answers the paths in gone with 410 and those in broken with 500.
type inbox struct {
	*httptest.Server
	mu       sync.Mutex
	gone     map[string]bool
	broken   map[string]bool
	received map[string][][]byte
	arrived  chan string
}

func newInbox(t *testing.T, gone ...string) *inbox {
	in := &inbox{gone: map[string]bool{}, broken: map[string]bool{}, received: map[string][][]byte{}, arrived: make(chan string, 100)}
	for _, path := range gone {
		in.gone[path] = true
	}
	in.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		in.mu.Lock()
		in.received[r.URL.Path] = append(in.received[r.URL.Path], body)
		in.mu.Unlock()
		switch {
		case in.gone[r.URL.Path]:
			w.WriteHeader(http.StatusGone)
		case in.broken[r.URL.Path]:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusCreated)
		}
		in.arrived <- r.URL.Path
	}))
	t.Cleanup(in.Close)
	return in
}

func (in *inbox) bodies(path string) [][]byte {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.received[path]
}

// device is a browser subscribed at the inbox's path, with the key that
// reads its pushes.
type device struct {
	sub Subscription
	key *ecdh.PrivateKey
}

// waitingList is a list the test changes between checks.
type waitingList struct {
	mu      sync.Mutex
	entries []features.Waiting
}

func (l *waitingList) set(entries ...features.Waiting) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = entries
}

func (l *waitingList) get() []features.Waiting {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.entries
}

// newSink is a Sink over list whose pushes reach in, with a device
// subscribed at each of paths.
func newSink(t *testing.T, in *inbox, list *waitingList, wake <-chan struct{}, paths ...string) (*Sink, *Store, map[string]device) {
	t.Helper()
	store := open(t, filepath.Join(t.TempDir(), File))
	devices := map[string]device{}
	for _, path := range paths {
		sub, key := receiver(t, in.URL+path)
		if err := store.Subscribe(sub, now); err != nil {
			t.Fatal(err)
		}
		devices[path] = device{sub, key}
	}
	return NewSink(list.get, NewSender(store, in.Client(), "https://agentry.example"), wake), store, devices
}

// pushed is what d was sent, decrypted.
func pushed(t *testing.T, in *inbox, path string, d device) []message {
	t.Helper()
	var messages []message
	for _, body := range in.bodies(path) {
		var m message
		if err := json.Unmarshal([]byte(decrypt(t, body, d.sub, d.key)), &m); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, m)
	}
	return messages
}

var (
	approval = features.Waiting{Key: "task-4/approval/a1", Task: "task-4", Line: "task-4: Waiting for you to approve the push to dev", Kind: "needs you"}
	question = features.Waiting{Key: "task-9/question/open", Task: "task-9", Line: "task-9: tech-lead asks which store to use", Kind: "needs you"}
	ideas    = features.Waiting{Key: "ideas/3", Line: "3 open ideas from the Visionaries", Kind: "ideas"}
)

func TestWhatWaitsAtStartIsNotPushed(t *testing.T) {
	in := newInbox(t)
	list := &waitingList{}
	list.set(approval)
	sink, _, _ := newSink(t, in, list, nil, "/phone")
	sink.start()
	sink.check(context.Background())
	if got := in.bodies("/phone"); len(got) != 0 {
		t.Errorf("want no push for what already waited, got %d", len(got))
	}
}

func TestANewEntryIsPushedOnceToEachDevice(t *testing.T) {
	in := newInbox(t)
	list := &waitingList{}
	sink, _, devices := newSink(t, in, list, nil, "/phone", "/laptop")
	sink.start()
	list.set(approval)
	sink.check(context.Background())
	sink.check(context.Background()) // nothing new

	want := message{Title: "Agentry", Body: approval.Line, Task: "task-4", URL: "/tasks/task-4", Tag: tagOf(approval.Key)}
	for path, d := range devices {
		if got := pushed(t, in, path, d); len(got) != 1 || got[0] != want {
			t.Errorf("%s: got %+v, want one %+v", path, got, want)
		}
	}
}

func TestTwoNewEntriesAreTwoPushes(t *testing.T) {
	in := newInbox(t)
	list := &waitingList{}
	sink, _, devices := newSink(t, in, list, nil, "/phone")
	sink.start()
	list.set(approval, ideas)
	sink.check(context.Background())

	got := pushed(t, in, "/phone", devices["/phone"])
	if len(got) != 2 || got[0].Tag != tagOf(approval.Key) || got[1].Tag != tagOf(ideas.Key) || got[1].URL != "/" || got[1].Task != "" {
		t.Errorf("want the approval, then the ideas opening /, got %+v", got)
	}
}

func TestAnEntryThatLeavesAndComesBackIsNewAgain(t *testing.T) {
	in := newInbox(t)
	list := &waitingList{}
	list.set(approval)
	sink, _, _ := newSink(t, in, list, nil, "/phone")
	sink.start()
	list.set(question)
	sink.check(context.Background())
	list.set(question, approval)
	sink.check(context.Background())

	if got := len(in.bodies("/phone")); got != 2 {
		t.Errorf("want the question and the returning approval, got %d pushes", got)
	}
}

func TestAGoneDeviceIsDroppedAndTheOthersStillGetTheirs(t *testing.T) {
	in := newInbox(t, "/old-phone")
	list := &waitingList{}
	sink, store, devices := newSink(t, in, list, nil, "/old-phone", "/phone")
	sink.start()
	list.set(approval)
	sink.check(context.Background())
	list.set(approval, question)
	sink.check(context.Background())

	if got := pushed(t, in, "/phone", devices["/phone"]); len(got) != 2 {
		t.Errorf("the phone: want 2 pushes, got %+v", got)
	}
	if got := len(in.bodies("/old-phone")); got != 1 {
		t.Errorf("the gone phone: want 1 push before it was dropped, got %d", got)
	}
	if got := store.Subscriptions(); len(got) != 1 || got[0].Endpoint != devices["/phone"].sub.Endpoint {
		t.Errorf("want only the phone left, got %+v", got)
	}
}

func TestADeviceThatFailsDoesNotStopTheOthers(t *testing.T) {
	in := newInbox(t)
	in.broken["/broken-phone"] = true
	list := &waitingList{}
	sink, store, devices := newSink(t, in, list, nil, "/broken-phone", "/phone")
	sink.start()
	list.set(approval)
	sink.check(context.Background())

	if got := pushed(t, in, "/phone", devices["/phone"]); len(got) != 1 {
		t.Errorf("the phone: want 1 push, got %+v", got)
	}
	if got := len(store.Subscriptions()); got != 2 {
		t.Errorf("a device that failed once is kept: want 2 subscriptions, got %d", got)
	}
}

func TestAnEntryWithALongKeyIsStillPushed(t *testing.T) {
	in := newInbox(t)
	list := &waitingList{}
	sink, _, devices := newSink(t, in, list, nil, "/phone")
	sink.start()
	text := strings.Repeat(`which "store" <db> to use? `, 200)
	long := features.Waiting{Key: "task-9/question/open/" + text, Task: "task-9", Line: "task-9: tech-lead asks " + text, Kind: "needs you"}
	list.set(long)
	sink.check(context.Background())

	want := message{Title: "Agentry", Body: shortened(long.Line, lineShown), Task: "task-9", URL: "/tasks/task-9", Tag: tagOf(long.Key)}
	if got := pushed(t, in, "/phone", devices["/phone"]); len(got) != 1 || got[0] != want {
		t.Errorf("got %+v, want one %+v", got, want)
	}
}

func TestATagIsShortAndTheSameForTheSameKey(t *testing.T) {
	tag := tagOf(approval.Key)
	if len(tag) != 32 || tag != tagOf(approval.Key) || tag == tagOf(question.Key) {
		t.Errorf("want 32 hex digits, the same for one key and different for another, got %q", tag)
	}
}

func TestALongLineIsCutToFitAPush(t *testing.T) {
	long := strings.Repeat("<é>", 2000)
	got := messageOf(features.Waiting{Key: "k", Task: "task-1", Line: long})
	if len(got.Body) > lineShown || !strings.HasSuffix(got.Body, "…") || !strings.HasPrefix(long, strings.TrimSuffix(got.Body, "…")) {
		t.Errorf("want at most %d bytes of the line and an ellipsis, got %d bytes: %q", lineShown, len(got.Body), got.Body)
	}
	data, _ := json.Marshal(got)
	if len(data) > maxMessage {
		t.Errorf("the message is %d bytes, over a push's %d", len(data), maxMessage)
	}
}

func TestRunPushesOnAWakeAndStopsWithItsContext(t *testing.T) {
	in := newInbox(t)
	list := &waitingList{}
	list.set(approval)
	wake := make(chan struct{})
	sink, _, _ := newSink(t, in, list, wake, "/phone")
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		sink.Run(ctx)
		close(stopped)
	}()

	wake <- struct{}{} // Run has started, so the approval is already seen
	list.set(approval, question)
	wake <- struct{}{}
	if path := <-in.arrived; path != "/phone" {
		t.Errorf("got a push to %s", path)
	}
	cancel()
	<-stopped
	if got := len(in.bodies("/phone")); got != 1 {
		t.Errorf("want one push, for the question, got %d", got)
	}
}

func TestRunStopsWhenWakeIsClosed(t *testing.T) {
	in := newInbox(t)
	list := &waitingList{}
	wake := make(chan struct{})
	sink, _, _ := newSink(t, in, list, wake, "/phone")
	stopped := make(chan struct{})
	go func() {
		sink.Run(context.Background())
		close(stopped)
	}()

	close(wake)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Run is still running after wake was closed")
	}
}
