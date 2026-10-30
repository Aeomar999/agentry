package web

import (
	"crypto/rand"
	"math/big"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	codeLength   = 8
	codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // no 0/O or 1/I/L, which read alike off a screen
	codeLifetime = 5 * time.Minute
	maxLiveCodes = 20 // a new code past this drops the oldest

	failureWindow     = time.Minute
	failuresPerClient = 5
	// Failed codes from every client together, so many addresses don't get
	// more guesses. Failed tokens don't count here: a token can't be guessed,
	// and the printed link must still sign in while someone is guessing.
	codeFailuresInAll = 30
)

// signins are the live codes and the recent failed attempts to sign in.
type signins struct {
	now func() time.Time

	mu           sync.Mutex
	codes        map[string]time.Time   // code, without its dash -> when it expires
	failures     map[string][]time.Time // client's address -> its failed attempts in the window
	codeFailures []time.Time            // every client's failed codes in the window
}

func newSignins(now func() time.Time) *signins {
	return &signins{now: now, codes: map[string]time.Time{}, failures: map[string][]time.Time{}}
}

// issue makes a code, shown as XXXX-XXXX, and says when it expires.
func (s *signins) issue() (string, time.Time, error) {
	code, err := newCode()
	if err != nil {
		return "", time.Time{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for live, expires := range s.codes {
		if !now.Before(expires) {
			delete(s.codes, live)
		}
	}
	if len(s.codes) >= maxLiveCodes {
		delete(s.codes, s.oldestCode())
	}
	expires := now.Add(codeLifetime)
	s.codes[code] = expires
	return code[:4] + "-" + code[4:], expires, nil
}

// oldestCode is the code that expires first: every code lives as long.
func (s *signins) oldestCode() string {
	var oldest string
	for code, expires := range s.codes {
		if oldest == "" || expires.Before(s.codes[oldest]) {
			oldest = code
		}
	}
	return oldest
}

func newCode() (string, error) {
	code := make([]byte, codeLength)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", err
		}
		code[i] = codeAlphabet[n.Int64()]
	}
	return string(code), nil
}

// redeem uses up a code that is live. People type codes as they like, so
// case, spaces and dashes don't matter.
func (s *signins) redeem(typed string) bool {
	code := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(typed))
	s.mu.Lock()
	defer s.mu.Unlock()
	expires, ok := s.codes[code]
	if !ok {
		return false
	}
	delete(s.codes, code)
	return s.now().Before(expires)
}

// attempt is a try to sign in, counted as failed until it succeeds.
type attempt struct {
	client   string
	withCode bool
	at       time.Time
}

// try counts an attempt as failed up front, in the same locked step as the
// check, so attempts made at once can't all pass the check before any of
// them is counted. It returns how long client must wait instead, if it must.
func (s *signins) try(client string, withCode bool) (attempt, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.forgetOldFailures(now)
	wait := retryAfter(s.failures[client], failuresPerClient, now)
	if withCode {
		wait = max(wait, retryAfter(s.codeFailures, codeFailuresInAll, now))
	}
	if wait > 0 {
		return attempt{}, wait
	}
	s.failures[client] = append(s.failures[client], now)
	if withCode {
		s.codeFailures = append(s.codeFailures, now)
	}
	return attempt{client, withCode, now}, 0
}

// succeeded takes back the failure try counted: signing in never counts.
func (s *signins) succeeded(a attempt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if times := withoutOne(s.failures[a.client], a.at); len(times) == 0 {
		delete(s.failures, a.client)
	} else {
		s.failures[a.client] = times
	}
	if a.withCode {
		s.codeFailures = withoutOne(s.codeFailures, a.at)
	}
}

// withoutOne drops one failure made at, keeping the rest in time order.
func withoutOne(times []time.Time, at time.Time) []time.Time {
	if i := slices.IndexFunc(times, at.Equal); i >= 0 {
		return slices.Delete(times, i, i+1)
	}
	return times
}

func (s *signins) forgetOldFailures(now time.Time) {
	for client, times := range s.failures {
		if times = inWindow(times, now); len(times) == 0 {
			delete(s.failures, client)
		} else {
			s.failures[client] = times
		}
	}
	s.codeFailures = inWindow(s.codeFailures, now)
}

// inWindow drops the failures older than the window; they are in time order.
func inWindow(times []time.Time, now time.Time) []time.Time {
	for len(times) > 0 && !now.Before(times[0].Add(failureWindow)) {
		times = times[1:]
	}
	return times
}

// retryAfter is the wait until fewer than limit failures are in the window.
func retryAfter(times []time.Time, limit int, now time.Time) time.Duration {
	if len(times) < limit {
		return 0
	}
	return times[len(times)-limit].Add(failureWindow).Sub(now)
}
