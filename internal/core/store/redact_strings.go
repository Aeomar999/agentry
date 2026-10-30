package store

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// A rewrittenRow is a row with its secret redacted.
type rewrittenRow struct {
	position int64
	kind     events.Type
	data     string   // the row's data, rewritten
	fields   []string // the JSON paths of the strings that held the secret
	removed  int      // the secret's bytes taken out
	refused  string   // why the row can't be rewritten, if it can't
	inKey    bool     // a key holds the secret, so the row has no fields
	inNumber bool     // the secret is left outside the strings, as in a number
}

// holds says whether the row held the secret: in a string it rewrote, or
// where it refuses to rewrite it.
func (rw rewrittenRow) holds() bool { return len(rw.fields) > 0 || rw.refused != "" }

// rewritten is the rows that hold secret, rewritten, in order.
func rewritten(rows []row, secret string) ([]rewrittenRow, error) {
	var rewrites []rewrittenRow
	for _, r := range rows {
		rw, err := redactedData(r.Data, secret)
		if err != nil {
			return nil, fmt.Errorf("the data at %d: %w", r.Position, err)
		}
		if rw.holds() {
			rw.position, rw.kind = r.Position, events.Type(r.Type)
			rewrites = append(rewrites, rw)
		}
	}
	return rewrites, nil
}

// redactedData is a row's data with secret replaced in every string value,
// and where it was. Only the strings that held it are
// written again, escaped as appendEvent's json.Marshal escapes them, so a
// rewritten string looks like one the log wrote; every other byte is
// copied as it was: the keys in their order, which a struct's fields set,
// the numbers and the other strings. A string the log would have escaped
// otherwise, which it never writes, would change by more than the secret,
// so its row is refused, as is one whose secret would still show next to
// [redacted], or whose bytes would still hold it anywhere else, such as in
// a number, which no string holds. A row with the secret in a key is
// refused with its data as it was and no fields, whose paths would hold
// the secret too. A row that doesn't hold the secret comes back empty.
func redactedData(data, secret string) (rewrittenRow, error) {
	var found finding
	escapedOtherwise, inKey := false, false
	rewritten, err := eachString(data, func(text, raw, path string) (string, bool) {
		replaced, ok := found.in(text, path, secret)
		if !ok {
			return "", false
		}
		escapedOtherwise = escapedOtherwise || encoded(text) != raw
		return encoded(replaced), true
	}, func(key string) {
		inKey = inKey || strings.Contains(key, secret)
	})
	if err != nil {
		return rewrittenRow{}, err
	}
	if inKey {
		return rewrittenRow{data: data, refused: "holds the secret in a key, which redacting doesn't rewrite", inKey: true}, nil
	}
	leftOver := strings.Contains(rewritten, secret)
	if len(found.fields) == 0 && !leftOver {
		return rewrittenRow{}, nil
	}
	rw := rewrittenRow{data: rewritten, fields: found.fields, removed: found.removed}
	switch {
	case escapedOtherwise:
		rw.refused = "isn't written as the log writes data: a string holding the secret is escaped otherwise, so rewriting it would change more than the secret"
	case found.survives:
		rw.refused = "would still show the secret next to " + Redacted
	case leftOver:
		rw.refused, rw.inNumber = "holds the secret outside its text, as in a number, which redacting can't rewrite", true
	}
	return rw, nil
}

// eachString is data with each string value, not a key, put through
// rewrite: given its text, decoded, its raw bytes, quotes included, and
// its JSON path, rewrite says what to write in its place, or false to keep
// it. Every other byte is copied as it was. Keys are only shown to key,
// decoded, and kept as they are: no event type has a map whose keys a user
// writes, and a row whose key holds the secret is refused rather than left
// half-redacted.
func eachString(data string, rewrite func(text, raw, path string) (string, bool), key func(text string)) (string, error) {
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.UseNumber()
	var rewritten strings.Builder
	var at cursor
	copied, read := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		end := int(decoder.InputOffset())
		if text, ok := token.(string); ok && at.keyNext() {
			key(text)
		} else if ok {
			// Before a string are only spaces, a comma or a colon, none a quote.
			start := read + strings.IndexByte(data[read:end], '"')
			if replaced, ok := rewrite(text, data[start:end], at.path()); ok {
				rewritten.WriteString(data[copied:start])
				rewritten.WriteString(replaced)
				copied = end
			}
		}
		at.step(token)
		read = end
	}
	rewritten.WriteString(data[copied:])
	return rewritten.String(), nil
}

// A cursor is where a walk of JSON tokens is: the objects and arrays it is
// in, the innermost last.
type cursor []level

// A level is an object or an array a cursor is in.
type level struct {
	object  bool
	key     string // in an object, the key of the value read now
	index   int    // in an array, the index of the value read now
	keyNext bool   // in an object, the next string is a key
}

// keyNext says whether the next string is a key.
func (c cursor) keyNext() bool { return len(c) > 0 && c[len(c)-1].keyNext }

// path is the JSON path of the value read now.
func (c cursor) path() string {
	path := "$"
	for _, l := range c {
		if l.object {
			path += "." + pathKey(l.key)
		} else {
			path += "[" + strconv.Itoa(l.index) + "]"
		}
	}
	return path
}

// step moves the cursor past token.
func (c *cursor) step(token json.Token) {
	switch token {
	case json.Delim('{'):
		*c = append(*c, level{object: true, keyNext: true})
	case json.Delim('['):
		*c = append(*c, level{})
	case json.Delim('}'), json.Delim(']'):
		*c = (*c)[:len(*c)-1]
		c.pastValue()
	default:
		if key, ok := token.(string); ok && c.keyNext() {
			top := &(*c)[len(*c)-1]
			top.key, top.keyNext = key, false
			return
		}
		c.pastValue()
	}
}

// pastValue moves the cursor past a value: to an array's next index, or
// an object's next key.
func (c cursor) pastValue() {
	if len(c) == 0 {
		return
	}
	top := &c[len(c)-1]
	if top.object {
		top.keyNext = true
	} else {
		top.index++
	}
}

// A finding is what redacting one row's data found. Its fields are in the
// order the row holds them: for data appendEvent wrote, each object's keys
// sorted.
type finding struct {
	fields   []string
	removed  int
	survives bool // the secret is still in a string once replaced
}

// in is text, the string at path, with secret replaced, which it notes,
// or false when the text doesn't hold the secret.
func (f *finding) in(text, path, secret string) (string, bool) {
	copies := strings.Count(text, secret)
	if copies == 0 {
		return "", false
	}
	f.fields = append(f.fields, path)
	f.removed += copies * len(secret)
	replaced := strings.ReplaceAll(text, secret, Redacted)
	f.survives = f.survives || strings.Contains(replaced, secret)
	return replaced, true
}

// encoded is text as a JSON string, as appendEvent's json.Marshal writes it.
func encoded(text string) string {
	b, _ := json.Marshal(text) // a string always encodes
	return string(b)
}

// plainKey is a key a JSON path can name as it is.
var plainKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// pathKey is a key as SQLite's JSON paths write it: quoted unless plain.
func pathKey(key string) string {
	if plainKey.MatchString(key) {
		return key
	}
	return strconv.Quote(key)
}
