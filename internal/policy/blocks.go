package policy

import (
	"bytes"
	"fmt"

	"github.com/pelletier/go-toml/v2/unstable"
)

// profileBlock is where one [profiles.NAME] table is written in a policy
// file: from its header line to the end of its last line, its sub-tables
// such as [[profiles.NAME.rules]] included. Agentry edits a file by
// replacing or adding these spans, so everything else in it, comments and
// layout included, stays exactly as the user wrote it.
type profileBlock struct {
	name       string
	start, end int // byte offsets: the header line's first byte, and just after the block's last non-blank line
}

func (b profileBlock) text(file []byte) []byte { return file[b.start:b.end] }

// tableHeader is a [table] or [[array.table]] line.
type tableHeader struct {
	line int // offset of the line's first byte
	key  []string
}

// profileBlocks finds every profile block in a policy file, in file order.
// A profile must be written in one place: one written in two, or only as an
// inline table, has no block, and Agentry leaves it to the user.
func profileBlocks(file []byte) ([]profileBlock, error) {
	headers, err := tableHeaders(file)
	if err != nil {
		return nil, err
	}
	var blocks []profileBlock
	seen := map[string]bool{}
	for i, h := range headers {
		name, ok := profileOf(h)
		if !ok || (i > 0 && sameProfile(headers[i-1], name)) {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("profile %q is written in more than one place in the file", name)
		}
		seen[name] = true
		next := len(file)
		for _, later := range headers[i+1:] {
			if !sameProfile(later, name) {
				next = later.line
				break
			}
		}
		blocks = append(blocks, profileBlock{name: name, start: h.line, end: blockEnd(file, h.line, next)})
	}
	return blocks, nil
}

func findBlock(file []byte, name string) (profileBlock, bool, error) {
	blocks, err := profileBlocks(file)
	if err != nil {
		return profileBlock{}, false, err
	}
	for _, b := range blocks {
		if b.name == name {
			return b, true, nil
		}
	}
	return profileBlock{}, false, nil
}

// profileOf is the profile a header belongs to: [profiles.NAME] or one of
// its sub-tables. [profiles] alone belongs to none.
func profileOf(h tableHeader) (string, bool) {
	if len(h.key) < 2 || h.key[0] != "profiles" {
		return "", false
	}
	return h.key[1], true
}

func sameProfile(h tableHeader, name string) bool {
	other, ok := profileOf(h)
	return ok && other == name
}

// blockEnd is where a block that starts at start, and runs until the next
// table header at next, really ends: the comment lines right above the next
// header introduce that table, and blank lines belong to neither.
func blockEnd(file []byte, start, next int) int {
	end, aboveNext := next, true
	for end > start {
		begin := lineStart(file, end-1)
		line := bytes.TrimSpace(file[begin:end])
		switch {
		case len(line) == 0:
			aboveNext = false
		case line[0] != '#' || !aboveNext:
			return end
		}
		end = begin
	}
	return end
}

func lineStart(file []byte, offset int) int { return bytes.LastIndexByte(file[:offset], '\n') + 1 }

// tableHeaders lists the file's table headers, using the TOML parser so a
// line inside a multi-line string or array is never taken for one.
func tableHeaders(file []byte) ([]tableHeader, error) {
	var p unstable.Parser
	p.Reset(file)
	var headers []tableHeader
	for p.NextExpression() {
		e := p.Expression()
		if e.Kind != unstable.Table && e.Kind != unstable.ArrayTable {
			continue
		}
		var key []string
		offset := -1
		for parts := e.Key(); parts.Next(); {
			part := parts.Node()
			if offset < 0 {
				offset = int(part.Raw.Offset)
			}
			key = append(key, string(part.Data))
		}
		headers = append(headers, tableHeader{line: lineStart(file, offset), key: key})
	}
	return headers, p.Error()
}

// keyLine is the offset of the line that sets the top-level key name, or
// -1 when the file doesn't set it.
func keyLine(file []byte, name string) (int, error) {
	var p unstable.Parser
	p.Reset(file)
	for p.NextExpression() {
		e := p.Expression()
		if e.Kind == unstable.Table || e.Kind == unstable.ArrayTable {
			break // top-level keys come before the first table
		}
		parts := e.Key()
		if e.Kind != unstable.KeyValue || !parts.Next() {
			continue
		}
		first := parts.Node()
		if string(first.Data) == name && !parts.Next() {
			return lineStart(file, int(first.Raw.Offset)), nil
		}
	}
	return -1, p.Error()
}
