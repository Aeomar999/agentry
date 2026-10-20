package schedules

import (
	"testing"
	"time"
)

// utc is an instant, written in UTC so the test doesn't lean on time.Date
// to settle a clock change.
func utc(month time.Month, day, hour, minute int) time.Time {
	return time.Date(2026, month, day, hour, minute, 0, 0, time.UTC)
}

type nextCase struct {
	name  string
	rule  Rule
	after time.Time
	want  time.Time
}

func checkNext(t *testing.T, cases []nextCase) {
	t.Helper()
	for _, c := range cases {
		got := c.rule.Next(c.after)
		if !got.Equal(c.want) {
			t.Errorf("%s: Next(%v) = %v, want %v", c.name, c.after, got, c.want.In(got.Location()))
			continue
		}
		if !got.IsZero() && got.Location().String() != c.rule.Zone {
			t.Errorf("%s: slot in %s, want %s", c.name, got.Location(), c.rule.Zone)
		}
	}
}

// Before 25 Oct 2026 London is on BST (UTC+1), after it on GMT. 19 Oct is a
// Monday.
func TestNextRunsDailyOnWeekdaysAndWeekly(t *testing.T) {
	daily := Rule{Every: "day", At: "02:00", Zone: "Europe/London"}
	weekdays := Rule{Every: "weekdays", At: "02:00", Zone: "Europe/London"}
	monday := Rule{Every: "week", Days: []string{"mon"}, At: "09:00", Zone: "Europe/London"}
	monThu := Rule{Every: "week", Days: []string{"thu", "mon"}, At: "09:00", Zone: "Europe/London"}
	checkNext(t, []nextCase{
		{"daily, just before", daily, utc(10, 19, 0, 59), utc(10, 19, 1, 0)},
		{"daily, exactly at", daily, utc(10, 19, 1, 0), utc(10, 20, 1, 0)},
		{"daily, just after", daily, utc(10, 19, 1, 1), utc(10, 20, 1, 0)},
		{"daily, late evening", daily, utc(10, 19, 22, 30), utc(10, 20, 1, 0)},

		{"weekdays, just before Friday's", weekdays, utc(10, 23, 0, 59), utc(10, 23, 1, 0)},
		{"weekdays, exactly at Friday's", weekdays, utc(10, 23, 1, 0), utc(10, 26, 2, 0)},
		{"weekdays, just after Friday's", weekdays, utc(10, 23, 1, 1), utc(10, 26, 2, 0)},
		{"weekdays, Saturday", weekdays, utc(10, 24, 12, 0), utc(10, 26, 2, 0)},
		{"weekdays, Sunday", weekdays, utc(10, 25, 12, 0), utc(10, 26, 2, 0)},
		{"weekdays, Monday's to Tuesday's", weekdays, utc(10, 26, 2, 0), utc(10, 27, 2, 0)},

		{"monday, just before", monday, utc(10, 19, 7, 59), utc(10, 19, 8, 0)},
		{"monday, exactly at", monday, utc(10, 19, 8, 0), utc(10, 26, 9, 0)},
		{"monday, just after", monday, utc(10, 19, 8, 1), utc(10, 26, 9, 0)},
		{"monday, midweek", monday, utc(10, 21, 12, 0), utc(10, 26, 9, 0)},

		{"mon+thu, just before Monday's", monThu, utc(10, 19, 7, 59), utc(10, 19, 8, 0)},
		{"mon+thu, exactly at Monday's", monThu, utc(10, 19, 8, 0), utc(10, 22, 8, 0)},
		{"mon+thu, just after Thursday's", monThu, utc(10, 22, 8, 1), utc(10, 26, 9, 0)},
	})
}

// London's clocks go from 01:00 GMT to 02:00 BST on 29 Mar 2026 (at 01:00
// UTC), and from 02:00 BST back to 01:00 GMT on 25 Oct 2026 (at 01:00 UTC).
func TestNextSettlesLondonsClockChanges(t *testing.T) {
	at := func(hhmm string) Rule { return Rule{Every: "day", At: hhmm, Zone: "Europe/London"} }
	checkNext(t, []nextCase{
		{"spring, a time in the gap runs as it ends", at("01:30"), utc(3, 28, 12, 0), utc(3, 29, 1, 0)},
		{"spring, and the next day as usual", at("01:30"), utc(3, 29, 1, 0), utc(3, 30, 0, 30)},
		{"spring, before the gap", at("00:30"), utc(3, 28, 12, 0), utc(3, 29, 0, 30)},
		{"spring, after the gap", at("02:30"), utc(3, 28, 12, 0), utc(3, 29, 1, 30)},

		{"autumn, a repeated time runs at its first", at("01:30"), utc(10, 24, 12, 0), utc(10, 25, 0, 30)},
		{"autumn, and not again in the repeat", at("01:30"), utc(10, 25, 0, 30), utc(10, 26, 1, 30)},
		{"autumn, between the two", at("01:30"), utc(10, 25, 1, 0), utc(10, 26, 1, 30)},
		{"autumn, before the repeat", at("00:30"), utc(10, 24, 12, 0), utc(10, 24, 23, 30)},
		{"autumn, after the repeat", at("02:30"), utc(10, 24, 12, 0), utc(10, 25, 2, 30)},
	})
	if got := at("01:30").Next(utc(10, 24, 12, 0)); got.Format("15:04 -07:00") != "01:30 +01:00" {
		t.Errorf("autumn's 01:30 runs at %s, want 01:30 +01:00 (BST)", got.Format("15:04 -07:00"))
	}
}

// Kolkata is UTC+05:30 all year. Adelaide is UTC+09:30, and +10:30 from
// 02:00 on 4 Oct 2026, when its clocks skip to 03:00.
func TestNextWorksInHalfHourZones(t *testing.T) {
	kolkata := Rule{Every: "day", At: "09:00", Zone: "Asia/Kolkata"}
	adelaide := Rule{Every: "day", At: "09:00", Zone: "Australia/Adelaide"}
	adelaideGap := Rule{Every: "day", At: "02:30", Zone: "Australia/Adelaide"}
	checkNext(t, []nextCase{
		{"kolkata", kolkata, utc(10, 19, 0, 0), utc(10, 19, 3, 30)},
		{"kolkata, exactly at", kolkata, utc(10, 19, 3, 30), utc(10, 20, 3, 30)},
		{"adelaide, standard time", adelaide, utc(10, 2, 12, 0), utc(10, 2, 23, 30)},
		{"adelaide, summer time", adelaide, utc(10, 2, 23, 30), utc(10, 3, 22, 30)},
		{"adelaide, in the gap", adelaideGap, utc(10, 3, 12, 0), utc(10, 3, 16, 30)},
	})
}

// Through a whole year and both clock changes, a daily rule makes one slot
// on every day and never two.
func TestNextMakesOneSlotADay(t *testing.T) {
	for _, hhmm := range []string{"00:30", "01:00", "01:30", "02:00", "02:30"} {
		rule := Rule{Every: "day", At: hhmm, Zone: "Europe/London"}
		london, _ := time.LoadLocation("Europe/London")
		want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		slot := rule.Next(utc(1, 1, 0, 0).Add(-time.Hour))
		for ; slot.Year() == 2026; slot = rule.Next(slot) {
			year, month, day := slot.In(london).Date()
			if got := time.Date(year, month, day, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
				t.Fatalf("%s: after %s the next slot is %v", hhmm, want.AddDate(0, 0, -1).Format("2 Jan"), slot)
			}
			want = want.AddDate(0, 0, 1)
		}
		if want.Year() != 2027 {
			t.Errorf("%s: slots stopped on %s", hhmm, want.Format("2 Jan"))
		}
	}
}

// A rule Next can't compute never fires.
func TestNextOfAnUnusableRuleIsZero(t *testing.T) {
	ok := Rule{Every: "week", Days: []string{"mon"}, At: "02:00", Zone: "Europe/London"}
	with := func(change func(*Rule)) Rule {
		r := ok
		change(&r)
		return r
	}
	after := utc(10, 19, 12, 0)
	if ok.Next(after).IsZero() {
		t.Fatal("the rule the cases change doesn't work to begin with")
	}
	for name, rule := range map[string]Rule{
		"unknown every":    with(func(r *Rule) { r.Every = "month" }),
		"no every":         with(func(r *Rule) { r.Every = "" }),
		"week with no day": with(func(r *Rule) { r.Days = nil }),
		"unknown day":      with(func(r *Rule) { r.Days = []string{"mon", "monday"} }),
		"at without colon": with(func(r *Rule) { r.At = "0200" }),
		"at one digit":     with(func(r *Rule) { r.At = "2:00" }),
		"at 24:00":         with(func(r *Rule) { r.At = "24:00" }),
		"at 02:60":         with(func(r *Rule) { r.At = "02:60" }),
		"at with am":       with(func(r *Rule) { r.At = "02:00am" }),
		"no at":            with(func(r *Rule) { r.At = "" }),
		"unknown zone":     with(func(r *Rule) { r.Zone = "Mars/Olympus_Mons" }),
		"no zone":          with(func(r *Rule) { r.Zone = "" }),
	} {
		if got := rule.Next(after); !got.IsZero() {
			t.Errorf("%s: Next = %v, want the zero time", name, got)
		}
	}
}
