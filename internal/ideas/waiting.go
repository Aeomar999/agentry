package ideas

import (
	"fmt"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// openIdeas is one entry for every open idea, of every repo: how many, and
// the newest titles, newest session first as the Ideas page lists them. It
// points at list_ideas with all when some are in another repo than repo, or
// when repo is "" (not known). Its key is the number of ideas ever
// suggested, so a new idea is news and one you accept or dismiss isn't; one
// Agentry accepted for you was never yours to decide, so it isn't counted.
// There is none with no idea open.
func openIdeas(state extensions.Reader, repo string) []extensions.Waiting {
	all := List(state, "")
	newestSessionFirst(state, all)
	var open []*Idea
	yours := 0
	pointer := "list_ideas"
	if repo == "" {
		pointer = "list_ideas all"
	}
	for _, i := range all {
		if !i.AutoAccepted {
			yours++
		}
		if i.State != Open {
			continue
		}
		open = append(open, i)
		if repo != "" && i.Repo != repo {
			pointer = "list_ideas all"
		}
	}
	if len(open) == 0 {
		return nil
	}
	var titles []string
	for _, i := range open[:min(3, len(open))] {
		titles = append(titles, tasks.OneLine(i.Title, 60))
	}
	if more := len(open) - len(titles); more > 0 {
		titles = append(titles, fmt.Sprintf("and %d more", more))
	}
	what := "open ideas"
	if len(open) == 1 {
		what = "open idea"
	}
	return []extensions.Waiting{{Key: "ideas/" + strconv.Itoa(yours), Task: open[0].Task, Kind: "ideas",
		Line: fmt.Sprintf("%d %s from the Visionaries: %s (%s)", len(open), what, strings.Join(titles, ", "), pointer)}}
}
