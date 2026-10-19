package engines

import (
	"testing"
	"time"
)

func TestResetTimeReadsWhatClaudeCodeSays(t *testing.T) {
	london, _ := time.LoadLocation("Europe/London")
	now := time.Date(2026, 10, 19, 23, 40, 0, 0, london)
	at := func(month time.Month, day, hour, minute int, zone *time.Location) time.Time {
		return time.Date(2026, month, day, hour, minute, 0, 0, zone)
	}
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	kathmandu, _ := time.LoadLocation("Asia/Kathmandu")
	cases := []struct {
		text string
		want time.Time
	}{
		{"You've hit your session limit · resets 3am (Europe/London)", at(10, 20, 3, 0, london)},
		{"You've hit your session limit · resets 11:50pm (Europe/London) · progress saved", at(10, 19, 23, 50, london)},
		{"resets 2:30pm (UTC)", at(10, 20, 14, 30, time.UTC)},
		{"resets 12am (Asia/Tokyo)", at(10, 21, 0, 0, tokyo)}, // already the 20th there
		{"resets 12pm (Asia/Kathmandu)", at(10, 20, 12, 0, kathmandu)},
		{"You've hit your weekly limit · resets Oct 22, 3pm (Europe/London)", at(10, 22, 15, 0, london)},
		{"resets Jan 2, 2027, 9am (UTC)", time.Date(2027, 1, 2, 9, 0, 0, 0, time.UTC)},
		{"resets 3am", at(10, 20, 3, 0, london)}, // no zone: the machine's
		{"limit resets at 2026-10-20T04:00:00Z", time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC)},
		{`{"resetsAt": 1792468800}`, time.Unix(1792468800, 0)},
		{`{"resets_at": "2026-10-20T04:00:00Z"}`, time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC)},
		{"request sent 2026-10-20T04:00:00Z", time.Time{}},    // not a reset
		{"limit resets at 2026-10-19T20:00:00Z", time.Time{}}, // already past
		{`{"resetsAt": 1760000000}`, time.Time{}},             // already past
		{"resets Jan 2, 2025, 9am (UTC)", time.Time{}},        // already past
		{"", time.Time{}},
		{"You've hit your limit", time.Time{}},
		{"resets soon", time.Time{}},
		{"resets 13pm (UTC)", time.Time{}},
		{"resets 3:75am (UTC)", time.Time{}},
		{"resets 3am (Mars/Olympus_Mons)", time.Time{}},
		{"resets Feb 31, 3am (UTC)", time.Time{}},
		{"resets Foo 3, 3am (UTC)", time.Time{}},
		{"resets 0am (UTC)", time.Time{}},
	}
	for _, c := range cases {
		if got := ResetTime(c.text, now); !got.Equal(c.want) {
			t.Errorf("%q: got %v, want %v", c.text, got, c.want)
		}
	}
}
