package exactgob

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"strings"
	"testing"
	"time"
)

type inner struct {
	List  []string
	Index map[string]int
}

type value struct {
	Name     string
	At       time.Time
	Nil      []string
	Empty    []string
	Full     []string
	NilMap   map[string]bool
	EmptyMap map[string]bool
	NoPtr    *inner
	ZeroPtr  *inner // a pointer to a zero value: gob writes nothing for it
	Ptr      *inner
	Maps     []map[string]int // gob reads a nil map in here back as empty
	Lists    map[string][]int // and an empty list in here back as nil
	Inners   []inner
	Fixed    [2][]int
	Deep     map[string]*inner
}

func every() value {
	return value{
		Name: "x", At: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC),
		Empty: []string{}, Full: []string{"a", ""}, EmptyMap: map[string]bool{},
		ZeroPtr: &inner{}, Ptr: &inner{List: []string{}, Index: map[string]int{"a": 1}},
		Maps:   []map[string]int{nil, {}, {"b": 2}},
		Lists:  map[string][]int{"nil": nil, "empty": {}, "full": {1}},
		Inners: []inner{{}, {List: []string{}}, {Index: map[string]int{}}},
		Fixed:  [2][]int{nil, {}},
		Deep:   map[string]*inner{"zero": {}, "empty": {List: []string{}, Index: map[string]int{}}},
	}
}

func TestAValueComesBackExactlyAsItWas(t *testing.T) {
	want := every()
	var written bytes.Buffer
	if err := Encode(&written, want); err != nil {
		t.Fatal(err)
	}
	var got value
	if err := Decode(&written, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read back differs:\n%#v\n%#v", got, want)
	}
}

// The case for the package: plain gob merges nil and empty, so the value
// above doesn't come back through it.
func TestPlainGobLosesNilAndEmpty(t *testing.T) {
	want := every()
	var written bytes.Buffer
	if err := gob.NewEncoder(&written).Encode(want); err != nil {
		t.Fatal(err)
	}
	var got value
	if err := gob.NewDecoder(&written).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(got, want) {
		t.Error("plain gob kept nil and empty apart: the shape isn't needed")
	}
}

// Decoding into a value that already holds something sets every slice, map
// and pointer as written, nil ones too.
func TestDecodingIntoAFilledValueSetsItsNils(t *testing.T) {
	var written bytes.Buffer
	if err := Encode(&written, value{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	got := every()
	got.Name = ""
	if err := Decode(&written, &got); err != nil {
		t.Fatal(err)
	}
	if got.Empty != nil || got.EmptyMap != nil || got.ZeroPtr != nil || got.Inners != nil {
		t.Errorf("nils left set: %#v", got)
	}
}

func TestAShapeThatDoesntFitFails(t *testing.T) {
	var written bytes.Buffer
	encoder := gob.NewEncoder(&written)
	if err := encoder.Encode(every()); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Encode(shape{Bits: []byte{0xff}, N: 3}); err != nil {
		t.Fatal(err)
	}
	var got value
	if err := Decode(&written, &got); err == nil || !strings.Contains(err.Error(), "shape") {
		t.Errorf("a short shape read as %v", err)
	}
}

// sameName is a key type whose keys all print the same.
type sameName int

func (sameName) String() string { return "same" }

func TestKeysThatPrintTheSameFail(t *testing.T) {
	var written bytes.Buffer
	err := Encode(&written, map[sameName][]int{1: nil, 2: {}})
	if err == nil || !strings.Contains(err.Error(), `print as "same"`) {
		t.Errorf("two keys that print the same wrote as %v", err)
	}
}

func TestAnInterfaceFieldFails(t *testing.T) {
	var written bytes.Buffer
	err := Encode(&written, struct{ Any any }{})
	if err == nil || !strings.Contains(err.Error(), "interface") {
		t.Errorf("an interface field wrote as %v", err)
	}
}
