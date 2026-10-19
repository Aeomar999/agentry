package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Patch is a new version of a text, stored as what it keeps of the version
// before: that version's first Head bytes and last Tail bytes, with Middle
// between them. SHA256 is the hex hash of the whole new text, so Verify
// catches a patch meant for any text but the one it is checked against.
type Patch struct {
	Head   int
	Tail   int
	Middle string
	SHA256 string
}

// Diff is the patch that makes new from old: the longest prefix and suffix
// they share, never overlapping, and the rest of new between them. A
// prefix or suffix never ends inside a UTF-8 character of either text,
// since Middle is written as a JSON string, which can't hold part of one.
func Diff(old, new string) Patch {
	head := 0
	for head < len(old) && head < len(new) && old[head] == new[head] {
		head++
	}
	for head > 0 && !(startsRune(old, head) && startsRune(new, head)) {
		head--
	}
	tail, most := 0, min(len(old), len(new))-head
	for tail < most && old[len(old)-1-tail] == new[len(new)-1-tail] {
		tail++
	}
	for tail > 0 && !(startsRune(old, len(old)-tail) && startsRune(new, len(new)-tail)) {
		tail--
	}
	return Patch{Head: head, Tail: tail, Middle: new[head : len(new)-tail], SHA256: hashOf(new)}
}

// startsRune says whether i is where a character of text starts, or its end.
func startsRune(text string, i int) bool {
	return i == len(text) || utf8.RuneStart(text[i])
}

// Apply is the text p makes of old, or an error if p keeps more of old than
// there is. It doesn't check the hash: replay applies what the rules
// verified to the same text again, and hashing every version on every fold
// would cost far more than the fold.
func (p Patch) Apply(old string) (string, error) {
	if p.Head < 0 || p.Tail < 0 || p.Head+p.Tail > len(old) {
		return "", fmt.Errorf("the patch keeps %d bytes before and %d after, of a text of %d", p.Head, p.Tail, len(old))
	}
	return old[:p.Head] + p.Middle + old[len(old)-p.Tail:], nil
}

// Verify is Apply, with an error too if old isn't the text p was made from.
func (p Patch) Verify(old string) (string, error) {
	text, err := p.Apply(old)
	if err == nil && hashOf(text) != p.SHA256 {
		err = errors.New("the patch was made from another version of the text")
	}
	return text, err
}

func hashOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Patching is the event that changes the agent's knowledge by p.
func (a Agent) Patching(by events.Actor, p Patch) events.Proposal {
	return events.Proposal{Stream: events.AgentsStream, Actor: by, Type: events.AgentLearned,
		Data: map[string]any{"name": a.Name, "head": p.Head, "tail": p.Tail, "middle": p.Middle, "sha256": p.SHA256}}
}

// IsPatch says whether an agent.learned is a patch, which has a "middle",
// rather than a full copy of the knowledge, which has "knowledge".
func IsPatch(p events.Proposal) bool {
	return p.Has("middle")
}

// PatchOf reads the patch an agent.learned carries.
func PatchOf(p events.Proposal) (Patch, error) {
	head, headOK := count(p.Data["head"])
	tail, tailOK := count(p.Data["tail"])
	middle, middleOK := p.Data["middle"].(string)
	hash, hashOK := p.Data["sha256"].(string)
	if !headOK || !tailOK || !middleOK || !hashOK {
		return Patch{}, errors.New("a patch has a head and a tail (whole numbers, 0 or more), a middle and a sha256")
	}
	return Patch{Head: head, Tail: tail, Middle: middle, SHA256: hash}, nil
}

// count is value as a whole number of bytes. JSON numbers decode as
// float64; a proposal not yet written carries an int.
func count(value any) (int, bool) {
	switch n := value.(type) {
	case int:
		return n, n >= 0
	case float64:
		return int(n), n >= 0 && n <= math.MaxInt32 && n == math.Trunc(n)
	}
	return 0, false
}
