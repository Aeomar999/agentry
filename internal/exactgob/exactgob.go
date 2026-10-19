// Package exactgob is gob that reads a value back exactly as it was
// written, nil and empty told apart. Gob writes an empty slice or map, and
// a pointer to a zero value, as nothing, and reads nothing back as nil; it
// reads a nil map inside a slice back as an empty one. reflect.DeepEqual
// tells those apart, and so does a snapshot's equivalence test.
//
// So beside the value it writes its shape: one bit for every slice, map and
// pointer in it, set when it isn't nil, in a fixed order (fields in order,
// elements in order, map entries by key). Reading sets each one back as its
// bit says. This is a presence flag per slice or map, as
// docs/design/log-growth.md asks for, made by reflection rather than kept by
// hand, so a field added later has its flag without anyone adding it.
//
// Like gob, it reads only exported fields, and fails on a nil pointer in a
// slice or map, which gob can't write.
package exactgob

import (
	"cmp"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sync"
)

// shape is the nil bits of a value, as written after it.
type shape struct {
	Bits []byte
	N    int // how many bits there are
}

// Encode writes v, then its shape, to w as two gob values.
func Encode(w io.Writer, v any) error {
	encoder := gob.NewEncoder(w)
	if err := encoder.Encode(v); err != nil {
		return err
	}
	var s shape
	walk(reflect.ValueOf(v), &shaping{record: &s})
	return encoder.Encode(s)
}

// Decode reads into v, a pointer, what Encode wrote.
func Decode(r io.Reader, v any) error {
	decoder := gob.NewDecoder(r)
	if err := decoder.Decode(v); err != nil {
		return err
	}
	var s shape
	if err := decoder.Decode(&s); err != nil {
		return fmt.Errorf("reading the shape: %w", err)
	}
	reshape := &shaping{replay: &s}
	walk(reflect.ValueOf(v).Elem(), reshape)
	if reshape.read != s.N {
		reshape.fail(fmt.Errorf("the shape has %d bits, the value %d", s.N, reshape.read))
	}
	return reshape.err
}

// shaping is one walk: recording a value's nils, or setting a decoded
// value's back as they were recorded.
type shaping struct {
	record *shape // set when recording
	replay *shape // set when setting them back
	read   int    // bits read so far
	err    error
}

// keeps records whether v, a slice, map or pointer, is nil, or makes it nil
// or not as recorded. It says whether v now holds anything to walk into.
func (s *shaping) keeps(v reflect.Value) bool {
	if s.record != nil {
		held := !v.IsNil()
		if s.record.N%8 == 0 {
			s.record.Bits = append(s.record.Bits, 0)
		}
		if held {
			s.record.Bits[s.record.N/8] |= 1 << (s.record.N % 8)
		}
		s.record.N++
		return held
	}
	if s.read >= s.replay.N {
		s.fail(errors.New("the shape ends before the value"))
		return false
	}
	held := s.replay.Bits[s.read/8]&(1<<(s.read%8)) != 0
	s.read++
	switch {
	case !held && !v.IsNil():
		v.SetZero()
	case held && v.IsNil():
		v.Set(empty(v.Type()))
	}
	return held
}

func (s *shaping) fail(err error) {
	if s.err == nil {
		s.err = err
	}
}

// empty is a non-nil slice, map or pointer that holds nothing: what gob
// read back as nil.
func empty(t reflect.Type) reflect.Value {
	switch t.Kind() {
	case reflect.Slice:
		return reflect.MakeSlice(t, 0, 0)
	case reflect.Map:
		return reflect.MakeMap(t)
	default:
		return reflect.New(t.Elem())
	}
}

// walk visits every slice, map and pointer in v, in the fixed order.
func walk(v reflect.Value, s *shaping) {
	if s.err != nil || !holdsNils(v.Type()) {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if s.keeps(v) {
			walk(v.Elem(), s)
		}
	case reflect.Slice:
		if s.keeps(v) {
			for i := range v.Len() {
				walk(v.Index(i), s)
			}
		}
	case reflect.Array:
		for i := range v.Len() {
			walk(v.Index(i), s)
		}
	case reflect.Map:
		if s.keeps(v) {
			walkMap(v, s)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				walk(v.Field(i), s)
			}
		}
	}
}

// walkMap walks each entry's value, by key. A map's values can't be set in
// place, so each is walked in a copy, put back when setting.
func walkMap(v reflect.Value, s *shaping) {
	if !holdsNils(v.Type().Elem()) {
		return
	}
	keys := v.MapKeys()
	slices.SortFunc(keys, func(a, b reflect.Value) int { return cmp.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
	for _, key := range keys {
		value := reflect.New(v.Type().Elem()).Elem()
		value.Set(v.MapIndex(key))
		walk(value, s)
		if s.replay != nil {
			v.SetMapIndex(key, value)
		}
	}
}

// holding caches holdsNils by type.
var holding sync.Map // reflect.Type -> bool

// holdsNils is whether a value of type t can hold a slice, map or pointer
// gob writes: one in itself, or in an exported field or element. Values of
// any other type are skipped whole.
func holdsNils(t reflect.Type) bool {
	if held, ok := holding.Load(t); ok {
		return held.(bool)
	}
	holding.Store(t, true) // a type that holds itself holds a pointer, slice or map
	held := false
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map:
		held = true
	case reflect.Array:
		held = holdsNils(t.Elem())
	case reflect.Struct:
		for i := range t.NumField() {
			if t.Field(i).IsExported() && holdsNils(t.Field(i).Type) {
				held = true
				break
			}
		}
	}
	holding.Store(t, held)
	return held
}
