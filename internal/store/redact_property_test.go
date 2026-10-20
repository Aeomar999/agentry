package store

import (
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// For seeded random data, nested objects and arrays with keys in any
// order, spaces between tokens, numbers as they were written, and strings
// of quotes, backslashes, control characters, <>&, text beyond ASCII and
// the secret at the start, middle or end, or repeated, redactedData writes
// the bytes expected: each string that held the secret encoded again with
// it replaced, and every other byte as it was. They decode to the data
// with the secret replaced in every string, and the fields and the bytes
// removed are the ones the data held.
func TestRedactedDataChangesOnlyTheStringsHoldingTheSecret(t *testing.T) {
	for _, secret := range []string{secret, `p"a\ss`, "<tok\n>", "wörd ✓"} {
		for seed := range uint64(300) {
			g := &randomData{r: rand.New(rand.NewPCG(seed, 42)), secret: secret}
			g.object("$", 0)
			original, want := g.original.String(), g.redacted.String()
			name := strconv.Quote(secret) + " seed " + strconv.FormatUint(seed, 10)

			rw, err := redactedData(original, secret)
			if err != nil {
				t.Fatalf("%s: %v\n%s", name, err, original)
			}
			if len(g.fields) == 0 {
				if len(rw.fields) != 0 {
					t.Errorf("%s: found %v in data without the secret", name, rw.fields)
				}
				continue
			}
			if rw.data != want || rw.refused != "" {
				t.Errorf("%s: refused %q\n got %s\nwant %s\nfrom %s", name, rw.refused, rw.data, want, original)
			}
			if !reflect.DeepEqual(rw.fields, g.fields) || rw.removed != g.removed {
				t.Errorf("%s: fields %v removed %d, want %v and %d", name, rw.fields, rw.removed, g.fields, g.removed)
			}
			if got, replaced := decoded(t, rw.data), replacedIn(decoded(t, original), secret); !reflect.DeepEqual(got, replaced) {
				t.Errorf("%s: decodes to %v, want %v", name, got, replaced)
			}
		}
	}
}

// randomData writes a random JSON value twice: as it is, and as Redact
// should leave it, with the fields and bytes it should find.
type randomData struct {
	r                  *rand.Rand
	secret             string
	original, redacted strings.Builder
	fields             []string
	removed            int
}

// both writes text into the original and the redacted data alike.
func (g *randomData) both(text string) {
	g.original.WriteString(text)
	g.redacted.WriteString(text)
}

// space writes what may come between tokens.
func (g *randomData) space() {
	g.both([]string{"", "", " ", "\n\t "}[g.r.IntN(4)])
}

// keys are the keys an object may have, plain or not.
var keys = []string{"title", "a", "_k", "k1", "a b", "x.y", `q"k`, "ünï", "<k>", ""}

// object writes an object at path with its keys in a random order.
func (g *randomData) object(path string, depth int) {
	g.both("{")
	order := g.r.Perm(len(keys))[:g.r.IntN(5)]
	for i, k := range order {
		if i > 0 {
			g.both(",")
		}
		g.space()
		g.both(encoded(keys[k]))
		g.space()
		g.both(":")
		g.space()
		g.value(path+"."+pathKey(keys[k]), depth+1)
		g.space()
	}
	g.both("}")
}

// array writes an array at path.
func (g *randomData) array(path string, depth int) {
	g.both("[")
	for i := range g.r.IntN(4) {
		if i > 0 {
			g.both(",")
		}
		g.space()
		g.value(path+"["+strconv.Itoa(i)+"]", depth+1)
		g.space()
	}
	g.both("]")
}

// value writes a random value at path: deeper, more of them are strings.
func (g *randomData) value(path string, depth int) {
	switch n := g.r.IntN(8 + depth); {
	case n == 0:
		g.object(path, depth)
	case n == 1:
		g.array(path, depth)
	case n == 2:
		g.both([]string{"1e21", "-0", "1.50", "12345678901234567890", "true", "null"}[g.r.IntN(6)])
	default:
		g.string(path)
	}
}

// pieces are what a random string is made of.
var pieces = []string{"a", "Z", " ", `"`, `\`, "\n", "\t", "\x01", "<", ">", "&", "é", "✓", "😀", " "}

// string writes a random string at path, half of them holding the secret.
func (g *randomData) string(path string) {
	var made []string
	for range g.r.IntN(6) {
		made = append(made, pieces[g.r.IntN(len(pieces))])
	}
	head, tail := strings.Join(made[:len(made)/2], ""), strings.Join(made[len(made)/2:], "")
	s := head + tail
	switch g.r.IntN(8) {
	case 0:
		s = g.secret + s
	case 1:
		s = head + g.secret + tail
	case 2:
		s += g.secret
	case 3:
		s = g.secret + s + g.secret + g.secret
	}
	copies := strings.Count(s, g.secret)
	if copies == 0 {
		g.both(otherwiseEscaped(s, g.r.IntN(2) == 0))
		return
	}
	g.fields = append(g.fields, path)
	g.removed += copies * len(g.secret)
	g.original.WriteString(encoded(s))
	g.redacted.WriteString(encoded(strings.ReplaceAll(s, g.secret, Redacted)))
}

// otherwiseEscaped is text as a JSON string, with <, > and & left as they
// are when raw: a string Redact must keep however it was escaped.
func otherwiseEscaped(text string, raw bool) string {
	var b strings.Builder
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(!raw)
	encoder.Encode(text)
	return strings.TrimSuffix(b.String(), "\n")
}

// decoded is data decoded, numbers as their text.
func decoded(t *testing.T, data string) any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	return value
}

// replacedIn is value with secret replaced in every string but the keys.
func replacedIn(value any, secret string) any {
	switch value := value.(type) {
	case string:
		return strings.ReplaceAll(value, secret, Redacted)
	case map[string]any:
		for key := range value {
			value[key] = replacedIn(value[key], secret)
		}
	case []any:
		for i := range value {
			value[i] = replacedIn(value[i], secret)
		}
	}
	return value
}
