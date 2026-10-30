// Package schedules computes a schedule's state from its events, as package
// ideas does for an idea. A schedule makes a task at a set time: daily, on
// weekdays or on some days of the week, in a time zone.
package schedules

import (
	"time"
)

// Rule is when a schedule runs: Every day, weekdays or week (on Days), at a
// local time in a zone. It holds the event data as written (keys every,
// days, at, zone); Next makes sense of it.
type Rule struct {
	Every string   // "day", "weekdays" or "week"
	Days  []string // for "week": "mon" to "sun"
	At    string   // the local time, "HH:MM", 24-hour
	Zone  string   // an IANA time zone, read with time.LoadLocation
}

var dayNames = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// Next is the rule's first slot strictly after after, as an instant in its
// zone. A local time a clock change skips runs at the first instant after
// the gap, and one it repeats at its first occurrence, so the rule makes
// one slot for each day it runs. That slot falls on the day itself, unless
// a gap ends at midnight: then it runs at the next day's 00:00. A rule Next
// can't compute (an unknown every, day or zone, a bad at, a week with no
// days) returns the zero time: such a schedule never fires.
func (r Rule) Next(after time.Time) time.Time {
	runs, ok := r.runsOn()
	if !ok || r.Zone == "" {
		return time.Time{}
	}
	at, err := time.Parse("15:04", r.At)
	if err != nil || len(r.At) != len("15:04") {
		return time.Time{}
	}
	zone, err := time.LoadLocation(r.Zone)
	if err != nil {
		return time.Time{}
	}
	year, month, day := after.In(zone).Date()
	// A week on and a day: the same weekday comes round again.
	for i := 0; i <= 7; i++ {
		wall := time.Date(year, month, day+i, at.Hour(), at.Minute(), 0, 0, time.UTC)
		if !runs[wall.Weekday()] {
			continue
		}
		if slot := instant(wall, zone); slot.After(after) {
			return slot
		}
	}
	return time.Time{}
}

// runsOn is the weekdays the rule runs on, false if it names none it knows.
func (r Rule) runsOn() (map[time.Weekday]bool, bool) {
	runs := map[time.Weekday]bool{}
	switch r.Every {
	case "day":
		for _, d := range dayNames {
			runs[d] = true
		}
	case "weekdays":
		for d := time.Monday; d <= time.Friday; d++ {
			runs[d] = true
		}
	case "week":
		for _, name := range r.Days {
			d, ok := dayNames[name]
			if !ok {
				return nil, false
			}
			runs[d] = true
		}
	}
	return runs, len(runs) > 0
}

// instant is the first instant zone's clocks show wall (a wall-clock time
// written in UTC), or the first instant after the gap if they skip it.
// time.Date settles neither case reliably, so it is done here.
func instant(wall time.Time, zone *time.Location) time.Time {
	t := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), 0, 0, zone)
	start, end := t.ZoneBounds()
	switch shown := clock(t); {
	case shown.After(wall): // in a gap, read with the offset before it
		return start
	case shown.Before(wall): // in a gap, read with the offset after it
		return end
	}
	// Repeated: an earlier occurrence would be in the zone period before.
	if !start.IsZero() {
		_, before := start.Add(-time.Second).Zone()
		earlier := time.Unix(wall.Unix()-int64(before), 0).In(zone)
		if earlier.Before(start) && clock(earlier).Equal(wall) {
			return earlier
		}
	}
	return t
}

// clock is the wall-clock time t shows in its own zone, written in UTC.
func clock(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
}
