package launch

import (
	"bytes"
	"io"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// Hidden is what stands for a token in output.
const Hidden = "[hidden]"

// tokenParam is a token= value in a link, plain or URL-encoded, which
// serve prints before a reader may know the token.
var tokenParam = regexp.MustCompile(`(?i)(token(?:=|%3D))[^&\s"'<>#]+`)

// Redact is text with the token hidden wherever it appears, as is or
// URL-encoded, and with any token= value in a link hidden too.
func Redact(text, token string) string {
	if token != "" {
		for _, form := range []string{token, url.QueryEscape(token), url.PathEscape(token)} {
			text = strings.ReplaceAll(text, form, Hidden)
		}
	}
	return tokenParam.ReplaceAllString(text, "${1}"+Hidden)
}

// Redactor writes to W what is written to it, with Redact applied to each
// line. Token gives the token at the time of writing, so it can be read
// once the server has made it; it may be nil. A line is written when it
// ends, so a token split across writes is still hidden. A last line with
// no newline is written only by Close, and dropped if Close isn't called.
type Redactor struct {
	W     io.Writer
	Token func() string

	mu      sync.Mutex
	pending []byte
}

func (r *Redactor) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = append(r.pending, p...)
	end := bytes.LastIndexByte(r.pending, '\n')
	if end < 0 {
		return len(p), nil
	}
	lines := string(r.pending[:end+1])
	r.pending = append(r.pending[:0], r.pending[end+1:]...)
	if _, err := io.WriteString(r.W, Redact(lines, r.token())); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close writes what is left of the last line.
func (r *Redactor) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) == 0 {
		return nil
	}
	rest := string(r.pending)
	r.pending = nil
	_, err := io.WriteString(r.W, Redact(rest, r.token()))
	return err
}

func (r *Redactor) token() string {
	if r.Token == nil {
		return ""
	}
	return r.Token()
}
