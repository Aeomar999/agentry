// Package settingsfile is the settings file, features.toml beside
// policy.toml: each extension's settings and secrets, such as a git host's
// token. It is watched as the policy is, and kept out of the log, which is
// replayed, kept in snapshots and shown on the page. Nothing here writes or
// logs what the file holds: a problem names the file and where it is, not
// the line.
//
// The file holds one table per extension, of strings, and a secrets table
// under it:
//
//	[ideas]
//	host = "gitlab.com"
//
//	[ideas.secrets]
//	token = "..."
package settingsfile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/pelletier/go-toml/v2"

	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// Live is the settings file in force. A version that doesn't load never
// replaces the one before it, which stays in force until the file is fixed.
// No file is no settings.
type Live struct {
	path     string
	current  atomic.Pointer[map[string]extensions.Table]
	lastSeen [32]byte // the content last loaded or refused, so each version is judged once
	inForce  bool     // whether a version that loaded is in force, rather than none
}

// unread is lastSeen while the file can't be read, so that is said once.
var unread [32]byte

// Open loads path. It never fails: a file that is missing starts with no
// settings, and one that doesn't load starts with none and logs why.
func Open(path string) *Live {
	l := &Live{path: path}
	l.current.Store(&map[string]extensions.Table{})
	l.lastSeen = sha256.Sum256(nil) // so a missing file isn't reported
	l.reload()
	return l
}

// Tables are the tables in force, by extension. Callers must not change them.
func (l *Live) Tables() map[string]extensions.Table { return *l.current.Load() }

// For is extension's settings, which read the version in force on every call.
func (l *Live) For(extension string) extensions.Settings {
	return extensions.SettingsIn(l.Tables, extension)
}

// Watch reloads the file every interval until ctx ends. Only one Watch may
// run.
func (l *Live) Watch(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.reload()
		}
	}
}

// reload reads the file and, if it changed, puts it in force or logs why it
// can't.
func (l *Live) reload() {
	text, err := os.ReadFile(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		l.gone()
		return
	}
	if err != nil {
		if l.lastSeen == unread {
			return // already reported
		}
		l.lastSeen = unread
		slog.Error("agentry: the settings file can't be read; "+l.meanwhile(), "file", l.path, "err", err)
		return
	}
	sum := sha256.Sum256(text)
	if sum == l.lastSeen {
		return
	}
	l.lastSeen = sum
	tables, err := parse(text)
	if err != nil {
		slog.Error("agentry: the settings file doesn't load; "+l.meanwhile(), "file", l.path, "problem", err)
		return
	}
	l.current.Store(&tables)
	l.inForce = true
	slog.Info("agentry: settings file loaded", "file", l.path, "features", len(tables))
}

// meanwhile is what is in force while the file doesn't load: the last
// version that did, or none, at the start or once the file was gone.
func (l *Live) meanwhile() string {
	if l.inForce {
		return "the last version that loaded stays in force"
	}
	return "features have no settings until it is fixed"
}

// gone is a missing file: no settings, as an empty one. Removing the file
// removes the secrets, which is said once, naming only the file.
func (l *Live) gone() {
	none := sha256.Sum256(nil)
	if l.lastSeen == none {
		return // never there, empty, or already said
	}
	l.lastSeen = none
	l.current.Store(&map[string]extensions.Table{})
	l.inForce = false
	slog.Warn("agentry: the settings file is gone; features have no settings now", "file", l.path)
}

// parse reads the file's tables. Its errors never quote the file: go-toml's
// can, so only their position is kept. They name an extension's table, never a
// key or a header that isn't an extension's name, where a token pasted in the
// wrong place would be.
func parse(text []byte) (map[string]extensions.Table, error) {
	var file map[string]any
	if err := toml.NewDecoder(bytes.NewReader(text)).Decode(&file); err != nil {
		var syntax *toml.DecodeError
		if errors.As(err, &syntax) {
			row, column := syntax.Position()
			return nil, fmt.Errorf("line %d, column %d: it isn't valid TOML", row, column)
		}
		return nil, errors.New("it isn't valid TOML")
	}
	tables := map[string]extensions.Table{}
	for extension, value := range file {
		table, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("the top of the file should hold only tables, one per feature")
		}
		parsed, err := parseTable(extension, table)
		if err != nil {
			return nil, err
		}
		tables[extension] = parsed
	}
	return tables, nil
}

// parseTable reads one extension's table: strings, and a secrets table of strings.
func parseTable(extension string, table map[string]any) (extensions.Table, error) {
	parsed := extensions.Table{Values: map[string]string{}, Secrets: map[string]string{}}
	for key, value := range table {
		if key == "secrets" {
			secrets, ok := value.(map[string]any)
			if !ok {
				return parsed, fmt.Errorf("%s should be a table", header(extension, true))
			}
			for name, secret := range secrets {
				text, ok := secret.(string)
				if !ok {
					return parsed, fmt.Errorf("a secret in %s should be a string", header(extension, true))
				}
				parsed.Secrets[name] = text
			}
			continue
		}
		text, ok := value.(string)
		if !ok {
			return parsed, fmt.Errorf("a setting in %s should be a string", header(extension, false))
		}
		parsed.Values[key] = text
	}
	return parsed, nil
}

// header names extension's table, or the secrets table under it, in a problem:
// by its header when extension looks like an extension's name, and otherwise
// without it, as a token pasted as a header would be logged.
func header(extension string, secrets bool) string {
	switch {
	case !looksLikeAnExtension(extension) && secrets:
		return "a secrets table in the file"
	case !looksLikeAnExtension(extension):
		return "a table in the file"
	case secrets:
		return "[" + extension + ".secrets]"
	}
	return "[" + extension + "]"
}

// extensionName is the shape of an extension's name, such as ideas or git-host:
// short lowercase words joined by dashes. A token rarely has it, having
// digits or capitals.
var extensionName = regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)

// looksLikeAnExtension is whether name has the shape of an extension's name.
func looksLikeAnExtension(name string) bool {
	return len(name) <= 32 && extensionName.MatchString(name)
}
