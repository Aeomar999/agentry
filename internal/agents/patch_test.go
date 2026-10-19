package agents

import (
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// logged is a proposal as the log hands it back, through JSON: numbers come
// back as float64, and a string that isn't UTF-8 would come back changed.
func logged(t *testing.T, p events.Proposal) events.Event {
	t.Helper()
	written, err := json.Marshal(p.Data)
	if err != nil {
		t.Fatal(err)
	}
	p.Data = nil
	if err := json.Unmarshal(written, &p.Data); err != nil {
		t.Fatal(err)
	}
	return events.Event{Proposal: p}
}

// Characters of one to four bytes, so edits land beside and inside them.
var pieces = []string{"a", "b", " ", "\n", "é", "è", "ĩ", "日", "本", "🙂", "🙃"}

func randomText(r *rand.Rand, most int) string {
	var text strings.Builder
	for range r.IntN(most + 1) {
		text.WriteString(pieces[r.IntN(len(pieces))])
	}
	return text.String()
}

// randomEdit is new text made from old by one kind of edit, chosen at random.
func randomEdit(r *rand.Rand, old string) string {
	cut := func() int { // a byte offset, which may fall inside a character
		return r.IntN(len(old) + 1)
	}
	from, to := cut(), cut()
	if from > to {
		from, to = to, from
	}
	switch r.IntN(6) {
	case 0:
		return old // identical
	case 1:
		return "" // all gone
	case 2:
		return old[:from] + randomText(r, 8) + old[from:] // insert only
	case 3:
		return old[:from] + old[to:] // delete only
	case 4:
		return old[:from] + randomText(r, 8) + old[to:] // replace
	default:
		return randomText(r, 40) // unrelated
	}
}

// Diff never splits a UTF-8 character, so its middle survives the JSON the
// log is written in, and Apply gets every edit back byte for byte from the
// event the log returns. Edits cut at any byte, so some texts aren't UTF-8
// at all: JSON can't carry those whole, but Apply is byte-exact on them too.
func TestAPatchMakesTheNewTextFromTheOld(t *testing.T) {
	r := rand.New(rand.NewPCG(34, 1))
	old := ""
	for i := range 10_000 {
		if i%100 == 0 {
			old = randomText(r, 40) // sometimes empty
		}
		new := randomEdit(r, old)
		patch := Diff(old, new)
		if utf8.ValidString(new) {
			if !utf8.ValidString(patch.Middle) {
				t.Fatalf("Diff(%q, %q) splits a character: middle %q", old, new, patch.Middle)
			}
			read, err := PatchOf(logged(t, Agent{Name: "engineer"}.Patching(you, patch)).Proposal)
			if err != nil {
				t.Fatal(err)
			}
			patch = read
		}
		if got, err := patch.Apply(old); err != nil || got != new {
			t.Fatalf("Diff(%q, %q) = %+v, which applies as %q, %v", old, new, patch, got, err)
		}
		old = new
	}
}

func TestDiffKeepsAllItCan(t *testing.T) {
	for _, c := range []struct {
		old, new   string
		head, tail int
		middle     string
	}{
		{"", "", 0, 0, ""},
		{"", "new", 0, 0, "new"},
		{"old", "", 0, 0, ""},
		{"same", "same", 4, 0, ""},
		{"abc", "abXc", 2, 1, "X"}, // insert only
		{"abXc", "abc", 2, 1, ""},  // delete only
		{"aaa", "aa", 2, 0, ""},    // the prefix and suffix don't overlap
		{"aé", "aè", 1, 0, "è"},    // é and è share their first byte
		{"éa", "ĩa", 0, 1, "ĩ"},    // and é and ĩ their last
		{"日本", "日🙂本", 3, 3, "🙂"},   // whole characters around the insert
	} {
		patch := Diff(c.old, c.new)
		if patch.Head != c.head || patch.Tail != c.tail || patch.Middle != c.middle {
			t.Errorf("Diff(%q, %q) = %d, %d, %q; want %d, %d, %q", c.old, c.new, patch.Head, patch.Tail, patch.Middle, c.head, c.tail, c.middle)
		}
	}
}

func TestAPatchFitsOnlyItsOwnBase(t *testing.T) {
	patch := Diff("Run the tests.", "Run the slow tests.")
	for base, want := range map[string]string{
		"Run all the tests.": "another version",
		"Run.":               "of a text of 4",
	} {
		if _, err := patch.Verify(base); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("verified on %q: %v, want an error with %q", base, err, want)
		}
	}
	if _, err := (Patch{Head: -1, Tail: 2}).Apply("Run."); err == nil {
		t.Error("a negative head applied")
	}
}

// Apply checks only the bounds, so a fold never hashes: the rules verified
// the patch before it was written.
func TestApplyLeavesTheHashToVerify(t *testing.T) {
	patch := Diff("Run the tests.", "Run the slow tests.")
	if got, err := patch.Apply("Run all the tests."); err != nil || got != "Run all slow tests." {
		t.Errorf("Apply on another base = %q, %v", got, err)
	}
	if _, err := patch.Apply("Run."); err == nil {
		t.Error("Apply kept more of the text than there is")
	}
}

func TestOnlyAWellFormedPatchIsRead(t *testing.T) {
	good := map[string]any{"name": "engineer", "head": 1.0, "tail": 0.0, "middle": "x", "sha256": "ab"}
	if _, err := PatchOf(events.Proposal{Data: good}); err != nil {
		t.Fatal(err)
	}
	for field, value := range map[string]any{"head": -1.0, "tail": 1.5, "middle": 3.0, "sha256": nil} {
		data := map[string]any{}
		for k, v := range good {
			data[k] = v
		}
		data[field] = value
		if _, err := PatchOf(events.Proposal{Data: data}); err == nil {
			t.Errorf("a patch with %s %v was read", field, value)
		}
	}
}

// versions are knowledge texts an agent wrote one after another.
var versions = []string{"Use gofmt.", "Use gofmt.\nRun go vet.", "Use gofmt -s.\nRun go vet.", "", "Start again: 日本.", "Start again: 日本 🙂."}

// A log mixing full copies and patches folds to the roster the same
// versions give as full copies, and today's full copies replay unchanged.
func TestPatchesAndFullCopiesFoldToTheSameRoster(t *testing.T) {
	copies, mixed := seeded(), seeded()
	engineer, _ := FromEvents(seeded()).Get("engineer")
	for i, text := range versions {
		copies = append(copies, logged(t, engineer.Learned(you, text)))
		if i%3 == 0 {
			mixed = append(mixed, logged(t, engineer.Learned(you, text)))
		} else {
			mixed = append(mixed, logged(t, engineer.Patching(you, Diff(versions[i-1], text))))
		}
		got, want := FromEvents(mixed), FromEvents(copies)
		if got, _ := got.Get("engineer"); got.Knowledge != text {
			t.Fatalf("after version %d, the engineer knows %q", i, got.Knowledge)
		}
		for i := range got {
			got[i].patched = 0 // the one thing a copy and a patch leave different
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("after version %d the rosters differ:\n%+v\n%+v", i, got, want)
		}
	}
}

// Patched counts the middles since the last full copy, and a change of the
// agent's settings or name keeps it, as it keeps the knowledge.
func TestPatchedCountsSinceTheLastFullCopy(t *testing.T) {
	stream := seeded()
	engineer, _ := FromEvents(stream).Get("engineer")
	patched := func() int {
		agent, _ := FromEvents(stream).Get(engineer.Name)
		return agent.Patched()
	}
	stream = append(stream, logged(t, engineer.Learned(you, "a")),
		logged(t, engineer.Patching(you, Diff("a", "abc"))), logged(t, engineer.Patching(you, Diff("abc", "abcd"))))
	if got := patched(); got != 3 {
		t.Errorf("after middles of 2 and 1 bytes, Patched is %d", got)
	}
	engineer.Prompt = "Only write Go."
	stream = append(stream, asEvent(engineer.Proposal(you, true)), asEvent(engineer.Renaming(you, "builder")))
	engineer.Name = "builder"
	if got := patched(); got != 3 {
		t.Errorf("after a change and a rename, Patched is %d", got)
	}
	stream = append(stream, logged(t, engineer.Learned(you, "fresh")))
	if got := patched(); got != 0 {
		t.Errorf("after a full copy, Patched is %d", got)
	}
}

// The rules refuse a patch that doesn't fit, so one in the log that keeps
// more of the text than there is is a bug: applying it panics, which breaks
// the stream, and leaves the roster as it was.
func TestAPatchThatDoesNotFitPanics(t *testing.T) {
	stream := seeded()
	engineer, _ := FromEvents(stream).Get("engineer")
	roster := FromEvents(append(stream, logged(t, engineer.Learned(you, "Use gofmt."))))
	defer func() {
		if recover() == nil {
			t.Error("a patch on another base applied")
		}
		if got, _ := roster.Get("engineer"); got.Knowledge != "Use gofmt." {
			t.Errorf("the failed patch changed the knowledge to %q", got.Knowledge)
		}
	}()
	roster.Apply(logged(t, engineer.Patching(you, Diff("Use tabs for indentation.", "Use tabs for all indentation."))))
}
