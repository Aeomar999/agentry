// Package deeptest checks copies of state by reflection: it fills every
// field of a value, exported or not, and finds memory two values share. A
// field added to a type later is checked without anyone adding it to a test.
package deeptest

import (
	"reflect"
	"testing"
	"time"
	"unsafe"
)

// Fill gives every field, exported or not, a value: every pointer, map and
// slice is non-empty, and slices have spare capacity. Times are in UTC, as
// the log's are, so they come back from JSON as they were.
func Fill(t testing.TB, v reflect.Value) {
	t.Helper()
	if v.Type() == reflect.TypeFor[time.Time]() {
		v.Set(reflect.ValueOf(time.Unix(1, 0).UTC()))
		return
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	case reflect.Struct:
		for _, field := range v.Fields() {
			Fill(t, settable(field))
		}
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		Fill(t, p.Elem())
		v.Set(p)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 2, 4)
		for i := range s.Len() {
			Fill(t, s.Index(i))
		}
		v.Set(s)
	case reflect.Map:
		key, value := reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem()
		Fill(t, key)
		Fill(t, value)
		m := reflect.MakeMap(v.Type())
		m.SetMapIndex(key, value)
		v.Set(m)
	default:
		t.Fatalf("Fill doesn't know a %s", v.Type())
	}
}

// settable is the field, unexported or not, as a value that can be set.
func settable(field reflect.Value) reflect.Value {
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
}

// Shared fails for every pointer, map or slice array a and b both hold.
// Strings never change and time.Time's location is shared by design, so
// they are not checked.
func Shared(t testing.TB, path string, a, b reflect.Value) {
	t.Helper()
	if a.Type() == reflect.TypeFor[time.Time]() {
		return
	}
	switch a.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if a.IsNil() || a.Kind() != reflect.Pointer && a.Len() == 0 {
			t.Errorf("%s is empty in the original, so nothing is checked", path)
			return
		}
		if a.Pointer() == b.Pointer() {
			t.Errorf("%s is shared with the copy", path)
		}
	}
	switch a.Kind() {
	case reflect.Pointer:
		Shared(t, path, a.Elem(), b.Elem())
	case reflect.Map:
		for _, key := range a.MapKeys() {
			Shared(t, path+"[key]", a.MapIndex(key), b.MapIndex(key))
		}
	case reflect.Slice:
		for i := range a.Len() {
			Shared(t, path+"[i]", a.Index(i), b.Index(i))
		}
	case reflect.Struct:
		for field, value := range a.Fields() {
			Shared(t, path+"."+field.Name, value, b.FieldByIndex(field.Index))
		}
	case reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer, reflect.Array:
		t.Errorf("%s is a %s, which Shared doesn't check", path, a.Kind())
	}
}
