// Package settingsfile is the settings file, features.toml beside
// policy.toml: each feature's settings and secrets, such as a git host's
// token. It is watched as the policy is, and kept out of the log, which is
// replayed, kept in snapshots and shown on the page. Nothing here writes or
// logs what the file holds: a problem names the file and where it is, not
// the line.
//
// The file holds one table per feature, of strings, and a secrets table
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
	"sync/atomic"
	"time"

	"github.com/pelletier/go-toml/v2"

	"gitlab.com/dev.chempa/agentry/internal/features"
)

// Live is the settings file in force. A version that doesn't load never
// replaces the one before it, which stays in force until the file is fixed.
// No file is no settings.
type Live struct {
	path     string
	current  atomic.Pointer[map[string]features.Table]
	lastSeen [32]byte // the content last loaded or refused, so each version is judged once
}

// unread is lastSeen while the file can't be read, so that is said once.
var unread [32]byte

// Open loads path. It never fails: a file that is missing starts with no
// settings, and one that doesn't load starts with none and logs why.
func Open(path string) *Live {
	l := &Live{path: path}
	l.current.Store(&map[string]features.Table{})
	l.lastSeen = sha256.Sum256(nil) // so a missing file isn't reported
	l.reload()
	return l
}

// Tables are the tables in force, by feature. Callers must not change them.
func (l *Live) Tables() map[string]features.Table { return *l.current.Load() }

// For is feature's settings, which read the version in force on every call.
func (l *Live) For(feature string) features.Settings {
	return features.SettingsIn(l.Tables, feature)
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
// can't. A missing file is an empty one: removing it removes the secrets.
func (l *Live) reload() {
	text, err := os.ReadFile(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		text, err = nil, nil
	}
	if err != nil {
		if l.lastSeen == unread {
			return // already reported
		}
		l.lastSeen = unread
		slog.Error("agentry: the settings file can't be read; the last version that loaded stays in force",
			"file", l.path, "err", err)
		return
	}
	sum := sha256.Sum256(text)
	if sum == l.lastSeen {
		return
	}
	l.lastSeen = sum
	tables, err := parse(text)
	if err != nil {
		slog.Error("agentry: the settings file doesn't load; the last version that loaded stays in force",
			"file", l.path, "problem", err)
		return
	}
	l.current.Store(&tables)
	slog.Info("agentry: settings file loaded", "file", l.path, "features", len(tables))
}

// parse reads the file's tables. Its errors never quote the file: go-toml's
// can, so only their position is kept. They name a feature's table, never a
// key, where a token pasted in the wrong place would be.
func parse(text []byte) (map[string]features.Table, error) {
	var file map[string]any
	if err := toml.NewDecoder(bytes.NewReader(text)).Decode(&file); err != nil {
		var syntax *toml.DecodeError
		if errors.As(err, &syntax) {
			row, column := syntax.Position()
			return nil, fmt.Errorf("line %d, column %d: it isn't valid TOML", row, column)
		}
		return nil, errors.New("it isn't valid TOML")
	}
	tables := map[string]features.Table{}
	for feature, value := range file {
		table, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("the top of the file should hold only tables, one per feature")
		}
		parsed, err := parseTable(feature, table)
		if err != nil {
			return nil, err
		}
		tables[feature] = parsed
	}
	return tables, nil
}

// parseTable reads one feature's table: strings, and a secrets table of strings.
func parseTable(feature string, table map[string]any) (features.Table, error) {
	parsed := features.Table{Values: map[string]string{}, Secrets: map[string]string{}}
	for key, value := range table {
		if key == "secrets" {
			secrets, ok := value.(map[string]any)
			if !ok {
				return parsed, fmt.Errorf("[%s.secrets] should be a table", feature)
			}
			for name, secret := range secrets {
				text, ok := secret.(string)
				if !ok {
					return parsed, fmt.Errorf("a secret in [%s.secrets] should be a string", feature)
				}
				parsed.Secrets[name] = text
			}
			continue
		}
		text, ok := value.(string)
		if !ok {
			return parsed, fmt.Errorf("a setting in [%s] should be a string", feature)
		}
		parsed.Values[key] = text
	}
	return parsed, nil
}
