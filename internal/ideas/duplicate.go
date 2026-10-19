package ideas

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Duplicate is the idea in list that the idea raises again, nil for none.
// It is the one the Visionaries named in Repeats, even if list doesn't hold
// it; otherwise an idea about the same repo, open, accepted or dismissed,
// under the same title once normalised (sameTitle), or whose problem and
// proposal say much the same (sameText). An idea suggested earlier is the
// original whatever became of it, so the later one stays out. A later one
// is the original only when it was accepted, as you chose to build it: a
// later open idea, or one you dismissed, maybe for repeating this one,
// leaves this one eligible. The earliest match is the original.
func Duplicate(idea *Idea, list []*Idea) *Idea {
	if idea.Repeats != "" {
		if i := slices.IndexFunc(list, func(other *Idea) bool { return other.ID == idea.Repeats }); i >= 0 {
			return list[i]
		}
		return &Idea{ID: idea.Repeats}
	}
	var original *Idea
	for _, other := range list {
		switch {
		case other.ID == idea.ID || other.Repo != idea.Repo:
		case other.State != Accepted && !earlier(other, idea):
		case !sameTitle(other, idea) && !sameText(other, idea):
		case original == nil || earlier(other, original):
			original = other
		}
	}
	return original
}

// duplicateText says which idea this one repeats, and the task it became.
func duplicateText(original *Idea) string {
	if original.Became == "" {
		return "duplicate of " + original.ID
	}
	return "duplicate of " + original.ID + " (" + original.Became + ")"
}

// earlier says whether a was suggested before b: by when, then by the
// number in its id, as ideas made in the same instant still come in order.
func earlier(a, b *Idea) bool {
	if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
		return c < 0
	}
	return cmp.Less(idNumber(a.ID), idNumber(b.ID))
}

func idNumber(id string) int {
	n, _ := strconv.Atoi(id[strings.LastIndex(id, "-")+1:])
	return n
}

// sameTitle: the titles match once both are in lower case, with the
// punctuation, the extra spaces and the filler words dropped.
func sameTitle(a, b *Idea) bool {
	return slices.Equal(titleWords(a.Title), titleWords(b.Title))
}

func titleWords(title string) []string {
	return slices.DeleteFunc(words(title), func(word string) bool { return fillers[word] })
}

// Tasks are the tasks an idea's title is checked against: every task, as
// kept from the log (current.State).
type Tasks interface {
	TaskIDs() []string
	Task(id string) (*tasks.Task, error)
}

// OpenTaskLike is the open task the idea looks like, nil for none: one in
// the same repo, out of Done (the Backlog counts as open), not made from
// the idea itself, whose title is the same once normalised or close
// (titleLikeness). The closest is the one, then the earliest made. A task
// that can't be read is skipped.
func OpenTaskLike(board Tasks, idea *Idea) *tasks.Task {
	var like *tasks.Task
	best := 0.0
	for _, id := range board.TaskIDs() {
		task, err := board.Task(id)
		if err != nil || task == nil || task.IsDone || task.Repo != idea.Repo || task.MadeFrom == idea.ID {
			continue
		}
		likeness := titleLikeness(task.Title, idea.Title)
		switch {
		case likeness <= SameTitleAbove:
		case like == nil || likeness > best || likeness == best && madeEarlier(task, like):
			like, best = task, likeness
		}
	}
	return like
}

// madeEarlier says whether task a was made before b: by when, then by the
// number in its id.
func madeEarlier(a, b *tasks.Task) bool {
	if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
		return c < 0
	}
	return cmp.Less(idNumber(a.ID), idNumber(b.ID))
}

// SameTitleAbove is the share of their title words (Jaccard: shared over
// all, past the filler words) an idea and an open task must have in common
// for the idea to look like the task. Tuned on the board's own repeats:
// "Make a ship the user chose actually push, and count it shipped only
// after the push" was made again as "Make a ship the user chose push, and
// count it shipped only when it pushed", which share 0.70, while that and
// "Word a waiting push plainly, and grey out Shipping for teams that can't
// ship", about the same feature, share 0.13 (duplicate_test.go has them).
const SameTitleAbove = 0.5

// titleLikeness is the share of their title words two titles have in
// common, 1 for titles the same once normalised.
func titleLikeness(a, b string) float64 {
	these, those := setOf(titleWords(a)), setOf(titleWords(b))
	shared := 0
	for word := range these {
		if those[word] {
			shared++
		}
	}
	if all := len(these) + len(those) - shared; all > 0 {
		return float64(shared) / float64(all)
	}
	return 0
}

// SameTextAbove is the share of their words (Jaccard: shared over all) two
// ideas' problem and proposal must have in common to repeat each other.
// Tuned on the Portside ideas re-raised under new titles, which shared 0.46
// to 0.78, while different features about the same repo shared at most
// 0.20 (duplicate_test.go has the pairs).
const SameTextAbove = 0.35

// shortestText is how many words an idea's problem and proposal need
// before they are compared: a few words in common says little.
const shortestText = 10

// sameText: the two ideas' problems and proposals share more than
// SameTextAbove of their words, past the short and filler ones.
func sameText(a, b *Idea) bool {
	these, those := textWords(a), textWords(b)
	if len(these) < shortestText || len(those) < shortestText {
		return false
	}
	shared := 0
	for word := range these {
		if those[word] {
			shared++
		}
	}
	return float64(shared) > SameTextAbove*float64(len(these)+len(those)-shared)
}

func textWords(idea *Idea) map[string]bool {
	set := map[string]bool{}
	for _, word := range words(idea.Problem + " " + idea.Proposal) {
		if len(word) >= 3 && !fillers[word] {
			set[word] = true
		}
	}
	return set
}

var wordPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)

// words are the text's words, in lower case, without the punctuation.
func words(text string) []string { return wordPattern.FindAllString(strings.ToLower(text), -1) }

// fillers are words that say little about which feature an idea is.
var fillers = setOf(strings.Fields("a an and are as at be by can for from in into is it its no not of on one only or so than that the then this to via when with"))

func setOf(words []string) map[string]bool {
	set := map[string]bool{}
	for _, word := range words {
		set[word] = true
	}
	return set
}
