package rules

import (
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Redacting rewrites rows of a stream with a secret replaced by
// [redacted] (docs/design/log-growth.md section 5, "Removing a secret").
// Only the store writes event.redacted: the catalogue lets no other kind
// of actor write it. It goes onto any stream the log has, a broken or
// done task's too, but not an erased task, which takes no event, nor an
// archived one, whose rows are in a blob: that one is unarchived,
// redacted and archived again. What these rules can't see is left to
// Store.Redact: that "positions" are rows of the stream below the marker,
// as it reads them under its lock, and that none is an agent.learned.

// aRedaction says which rows it rewrote, and what in them, never the
// secret: one list of JSON paths per row, in the order of the rows,
// which come in the log's order, each once.
func aRedaction(p events.Proposal) string {
	positions := p.Numbers("positions")
	switch {
	case len(positions) == 0:
		return "a redaction names the rows it rewrote, by position"
	case positions[0] < 1 || !slices.IsSorted(positions) || len(slices.Compact(slices.Clone(positions))) != len(positions):
		return "a redaction names each row once, in the log's order"
	case !eachNamesFields(p.Data["fields"], len(positions)):
		return "a redaction names, for each row, the fields it changed"
	case p.Number("removed") < 1:
		return "a redaction says how many bytes it removed"
	case p.Text("who") == "":
		return "a redaction says who asked for it"
	}
	return ""
}

// eachNamesFields says whether fields is one non-empty list of paths per
// row, as proposed ([][]string) or read back from the log ([]any).
func eachNamesFields(fields any, rows int) bool {
	var lists [][]string
	switch value := fields.(type) {
	case [][]string:
		lists = value
	case []any:
		for _, item := range value {
			paths, _ := item.([]any)
			list := make([]string, 0, len(paths))
			for _, path := range paths {
				text, _ := path.(string) // "" for anything but a path, which refuses it
				list = append(list, text)
			}
			lists = append(lists, list)
		}
	}
	if len(lists) != rows {
		return false
	}
	for _, list := range lists {
		if len(list) == 0 || slices.Contains(list, "") {
			return false
		}
	}
	return true
}

// ArchivedRedactionRefusal is why an archived task can't be redacted as it
// is: its rows are in the archive's blob, not the live log.
func ArchivedRedactionRefusal(stream string) string {
	return stream + " is archived: unarchive it, redact it, then archive it again"
}
