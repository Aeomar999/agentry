package coordinator

import (
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A coordinator can't finish a task until its work is checked. The agents
// whose work counts as Review (a reviewer, a tester, a fact-checker) check
// it, and each ends its report with a verdict. Since the last turn that
// changed the work, every one of them that has checked it must approve it
// again. So a change made after a review gets another round of review. You
// can still finish a task as it is, from the board, or ship it: moved to
// Shipping, the work as it was is yours, and once an agent has pushed it
// the task can be done; only a change after that needs checking again. A
// ship turn that crashed, that you stopped or that ended without a push
// (refused, a merge conflict, failed tests, a denied push) hasn't shipped
// it, but your choice stands for the next ship turn if nothing has changed
// or checked the work in between.

type Verdict string

const (
	Approved      Verdict = "approved"
	ChangesNeeded Verdict = "changes needed"
	NoVerdict     Verdict = ""
)

// VerdictFormat is how a checking agent ends its report.
const VerdictFormat = "End your report with your verdict on its own line, exactly one of:\n\n" +
	"Verdict: approved\nVerdict: changes needed\n\n" +
	"Approve only work you would ship as it is. Anything that must be fixed first is changes needed, with the list of what. " +
	"The coordinator can't finish the task until you approve, and any change made after you approve comes back to you."

// verdictOf reads the verdict a checking agent's report ends with.
func verdictOf(report string) Verdict {
	lines := strings.Split(strings.TrimSpace(report), "\n")
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-5; i-- {
		line := strings.ToLower(strings.Trim(lines[i], " *_#>`-"))
		after, found := strings.CutPrefix(line, "verdict")
		if !found {
			continue
		}
		switch after = strings.TrimLeft(after, " :*_-—"); {
		case strings.HasPrefix(after, "changes"), strings.HasPrefix(after, "not approved"), strings.HasPrefix(after, "rejected"):
			return ChangesNeeded
		case strings.HasPrefix(after, "approved"):
			return Approved
		}
	}
	return NoVerdict
}

// check is a checker's verdict on the work, given in a turn.
type check struct {
	turn    *tasks.Turn
	at      int
	verdict Verdict
}

// workRecord is what the task's ended turns did to its work.
type workRecord struct {
	last    map[string]check // each checker's latest check
	order   []string         // the checkers, in the order they first checked
	change  int              // the turn that last changed the work, or -1
	shipped int              // the last turn that pushed the work because you moved it to Shipping, or -1
	retry   bool             // your ship didn't push, and nothing has changed or checked the work since
}

func recordOf(task *tasks.Task, roster agents.Roster) workRecord {
	r := workRecord{last: map[string]check{}, change: -1, shipped: -1}
	for i, turn := range task.Turns {
		agent, known := roster.Get(turn.Agent)
		switch {
		case !turn.Ended || turn.Agent == task.Coordinator || agent.Coordinates:
		case known && agent.Column == agents.Reviews:
			r.retry = false
			if _, seen := r.last[turn.Agent]; !seen {
				r.order = append(r.order, turn.Agent)
			}
			verdict := NoVerdict
			if turn.Issue == "" && !turn.Stopped {
				verdict = verdictOf(turn.Report)
			}
			r.last[turn.Agent] = check{turn, i, verdict}
		case known && agent.Column == agents.Ships: // ships the checked work; doesn't change it
			yours := turn.Shipped || r.retry
			r.retry = yours && !task.Pushed(turn.ID)
			if yours && !r.retry {
				r.shipped = i
			}
		default:
			r.change, r.retry = i, false
		}
	}
	return r
}

// yourShip says whether the next ship turn carries your move to Shipping:
// it is the handoff your move asked for, or it retries your ship that
// didn't push, with nothing changing or checking the work since.
func yourShip(task *tasks.Task, roster agents.Roster) bool {
	return task.Handoff != nil && task.Handoff.Ships || recordOf(task, roster).retry
}

// unchecked says why the task's work isn't checked yet, or "" when it is:
// nothing has changed it, the team has no one to check it, every check
// since the last change approves it, or you had it shipped since.
func unchecked(task *tasks.Task, roster agents.Roster) string {
	lead, _ := roster.Get(task.Coordinator)
	var checkers []string
	for _, agent := range roster.Assignees(lead.TeamName()) {
		if agent.Column == agents.Reviews {
			checkers = append(checkers, agent.Name)
		}
	}
	if len(checkers) == 0 {
		return ""
	}

	r := recordOf(task, roster)
	change := r.change
	if change < 0 || change < r.shipped { // nothing changed it, or nothing since you had it shipped
		return ""
	}

	changed := task.Turns[change].ID + ", by " + task.Turns[change].Agent
	var why []string
	approvedSince := false
	for _, name := range r.order {
		c := r.last[name]
		switch {
		case c.at < change:
			why = append(why, name+" checked it in "+c.turn.ID+", and it changed after that ("+changed+"): "+name+" checks it again")
		case c.verdict == ChangesNeeded:
			why = append(why, name+" asked for changes in "+c.turn.ID+": they are made, then "+name+" checks again")
		case c.verdict == NoVerdict:
			why = append(why, name+"'s report in "+c.turn.ID+" gives no verdict: ask "+name+" again, for one")
		default:
			approvedSince = true
		}
	}
	if !approvedSince && len(why) == 0 {
		why = append(why, "no one has checked it since it last changed ("+changed+"); the team's checkers are "+strings.Join(checkers, ", "))
	}
	if len(why) == 0 {
		return ""
	}
	return "the work isn't checked yet, so the task can't be done: " + strings.Join(why, "; ") +
		". If the user wants it finished unchecked, they can move it to Done on the board"
}
