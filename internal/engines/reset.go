package engines

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Claude Code 2.1.303 says when a usage limit resets in its message, as
// "· resets 3am (Europe/London)", "resets 2:30pm (UTC)" or, more than a day
// away, "resets Oct 22, 3pm (Europe/London)" with the year when it differs.
var resetsPattern = regexp.MustCompile(`(?i)resets\s+(?:([A-Z][a-z]{2})\s+(\d{1,2}),\s*(?:(\d{4}),\s*)?)?(\d{1,2})(?::(\d{2}))?\s*([ap]m)(?:\s*\(([^)]+)\))?`)

// ResetTime is when the limit a message describes resets, the next such
// time after now, or zero when the message doesn't say in a way it reads
// or names a time already past.
// The text is free and may change between versions, so it never panics.
func ResetTime(text string, now time.Time) time.Time {
	at := resetIn(text, now)
	if at.Before(now) {
		return time.Time{}
	}
	return at
}

func resetIn(text string, now time.Time) time.Time {
	// A time already past may be some other field; the words may still say.
	if at, ok := isoOrEpoch(text); ok && !at.Before(now) {
		return at
	}
	m := resetsPattern.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}
	}
	zone := now.Location()
	if m[7] != "" {
		loaded, err := time.LoadLocation(strings.TrimSpace(m[7]))
		if err != nil {
			return time.Time{}
		}
		zone = loaded
	}
	hour, _ := strconv.Atoi(m[4])
	minute, _ := strconv.Atoi(cmpOr(m[5], "0"))
	if hour < 1 || hour > 12 || minute > 59 {
		return time.Time{}
	}
	hour %= 12
	if strings.EqualFold(m[6], "pm") {
		hour += 12
	}
	local := now.In(zone)
	year, month, day := local.Date()
	if m[1] != "" {
		parsed, err := time.Parse("Jan", m[1])
		if err != nil {
			return time.Time{}
		}
		month = parsed.Month()
		day, _ = strconv.Atoi(m[2])
		if m[3] != "" {
			year, _ = strconv.Atoi(m[3])
		}
	}
	at := time.Date(year, month, day, hour, minute, 0, 0, zone)
	if at.Day() != day && m[1] != "" { // Feb 31 and the like
		return time.Time{}
	}
	for at.Before(now) && m[3] == "" {
		if m[1] != "" {
			at = at.AddDate(1, 0, 0)
		} else {
			at = at.AddDate(0, 0, 1)
		}
	}
	return at
}

// A reset word, as in "resets at", "reset_at": or "resetsAt"=, so other
// times in the text, such as when a request was sent, aren't taken for it.
const resetsWord = `(?i)\bresets?\w*["':=\s]+(?:at\s+)?`

var (
	isoPattern   = regexp.MustCompile(resetsWord + `(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))`)
	epochPattern = regexp.MustCompile(resetsWord + `(\d{10})\b`)
)

// isoOrEpoch reads a reset given as an RFC 3339 time or in Unix seconds.
func isoOrEpoch(text string) (time.Time, bool) {
	if m := isoPattern.FindStringSubmatch(text); m != nil {
		if at, err := time.Parse(time.RFC3339, m[1]); err == nil {
			return at, true
		}
	}
	if m := epochPattern.FindStringSubmatch(text); m != nil {
		seconds, _ := strconv.ParseInt(m[1], 10, 64)
		return time.Unix(seconds, 0), true
	}
	return time.Time{}, false
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
