// Package webpush keeps what Web Push needs: Agentry's VAPID key pair and
// the browsers subscribed to its pushes. They are kept in a file beside the
// log, never in it: a subscription is a device's address at its push
// service, which the log, read by every agent, has no use for. The package
// knows nothing of HTTP; the server reads and changes the file through it.
package webpush

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// File is the file's name in the data folder, beside the log.
const File = "push.json"

// ErrMalformed is a subscription a browser couldn't have sent: one with no
// endpoint or keys, or an endpoint that isn't https.
var ErrMalformed = errors.New("not a push subscription")

// Subscription is a browser's PushSubscription, as its toJSON gives it,
// and when it was added.
type Subscription struct {
	Endpoint string    `json:"endpoint"` // the device's address at its push service, always https
	Keys     Keys      `json:"keys"`
	Added    time.Time `json:"added"`
}

// Keys are what a push is encrypted for, base64url as the browser gives
// them.
type Keys struct {
	P256dh string `json:"p256dh"` // the browser's P-256 public key, an uncompressed point
	Auth   string `json:"auth"`   // the 16-byte secret shared with the browser
}

// Store is the file, read once when opened and written on every change.
type Store struct {
	path string
	key  *ecdsa.PrivateKey

	mu            sync.Mutex
	subscriptions []Subscription
}

// onDisk is the file as written.
type onDisk struct {
	PrivateKey    string         `json:"privateKey"` // the VAPID private key, its 32-byte scalar in base64url
	Subscriptions []Subscription `json:"subscriptions"`
}

// Open reads the file at path, or makes it with a new VAPID key pair when
// there is none. A file that can't be read is an error, not a new key: a
// new key would quietly end every subscription made with the old one.
// Writes that never finished, as after a crash mid-write, are removed
// first: each holds the private key.
func Open(path string) (*Store, error) {
	if err := removeLeftovers(path); err != nil {
		return nil, fmt.Errorf("unfinished writes of %s can't be removed: %w", path, err)
	}
	saved, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return create(path)
	}
	if err != nil {
		return nil, unreadable(err)
	}
	var file onDisk
	if err := json.Unmarshal(saved, &file); err != nil {
		return nil, unreadable(fmt.Errorf("%s: %w", path, err))
	}
	scalar, err := base64.RawURLEncoding.DecodeString(file.PrivateKey)
	if err != nil {
		return nil, unreadable(fmt.Errorf("%s: the private key: %w", path, err))
	}
	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), scalar)
	if err != nil {
		return nil, unreadable(fmt.Errorf("%s: the private key: %w", path, err))
	}
	return &Store{path: path, key: key, subscriptions: file.Subscriptions}, nil
}

// unreadable is err, which names the file, with how to recover from it.
func unreadable(err error) error {
	return fmt.Errorf("%w. Fix the file, or move it aside to start with new keys, which ends every subscription: each device has to subscribe again", err)
}

// removeLeftovers deletes the temporary files of writes to path that never
// finished. Nothing else is touched.
func removeLeftovers(path string) error {
	found, err := filepath.Glob(leftovers(path))
	if err != nil {
		return err
	}
	var errs []error
	for _, file := range found {
		errs = append(errs, os.Remove(file))
	}
	return errors.Join(errs...)
}

func leftovers(path string) string { return path + ".tmp-*" }

func create(path string) (*Store, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, key: key}
	return s, s.write()
}

// Key is the VAPID private key, which signs each push.
func (s *Store) Key() *ecdsa.PrivateKey { return s.key }

// PublicKey is the VAPID public key as a browser's pushManager.subscribe
// takes it: the uncompressed point, base64url.
func (s *Store) PublicKey() string {
	point, _ := s.key.PublicKey.Bytes() // always a P-256 point
	return base64.RawURLEncoding.EncodeToString(point)
}

// Subscriptions are the browsers subscribed, oldest first.
func (s *Store) Subscriptions() []Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.subscriptions)
}

// Subscribe adds sub, added now. A browser subscribing again with the same
// endpoint keeps its place and when it was added, with the keys it gives
// now. A malformed sub is ErrMalformed, and nothing changes.
func (s *Store) Subscribe(sub Subscription, now time.Time) error {
	if err := Check(sub); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.Clone(s.subscriptions)
	if i := s.indexOf(sub.Endpoint); i >= 0 {
		next[i].Keys = sub.Keys
	} else {
		next = append(next, Subscription{Endpoint: sub.Endpoint, Keys: sub.Keys, Added: now.UTC()})
	}
	return s.keep(next)
}

// Unsubscribe removes the subscription with endpoint, and says whether
// there was one.
func (s *Store) Unsubscribe(endpoint string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexOf(endpoint)
	if i < 0 {
		return false, nil
	}
	return true, s.keep(slices.Delete(slices.Clone(s.subscriptions), i, i+1))
}

func (s *Store) indexOf(endpoint string) int {
	return slices.IndexFunc(s.subscriptions, func(sub Subscription) bool { return sub.Endpoint == endpoint })
}

// keep writes next and then holds it, so a write that fails changes
// nothing.
func (s *Store) keep(next []Subscription) error {
	previous := s.subscriptions
	s.subscriptions = next
	if err := s.write(); err != nil {
		s.subscriptions = previous
		return err
	}
	return nil
}

func (s *Store) write() error {
	scalar, err := s.key.Bytes()
	if err != nil {
		return err
	}
	file := onDisk{PrivateKey: base64.RawURLEncoding.EncodeToString(scalar), Subscriptions: s.subscriptions}
	if file.Subscriptions == nil {
		file.Subscriptions = []Subscription{}
	}
	return writeAtomically(s.path, func(w io.Writer) error {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(file)
	})
}

// Check says what is wrong with sub, as ErrMalformed, or nil.
func Check(sub Subscription) error {
	endpoint, err := url.Parse(sub.Endpoint)
	switch {
	case sub.Endpoint == "":
		return fmt.Errorf("%w: it has no endpoint", ErrMalformed)
	case err != nil || endpoint.Scheme != "https" || endpoint.Host == "":
		return fmt.Errorf("%w: its endpoint is not an https address", ErrMalformed)
	}
	if point, err := decodeKey(sub.Keys.P256dh); err != nil || len(point) != 65 || point[0] != 4 {
		return fmt.Errorf("%w: keys.p256dh is not a P-256 public key in base64url", ErrMalformed)
	}
	if secret, err := decodeKey(sub.Keys.Auth); err != nil || len(secret) != 16 {
		return fmt.Errorf("%w: keys.auth is not 16 bytes in base64url", ErrMalformed)
	}
	return nil
}

// decodeKey reads base64url with or without its padding: browsers leave it
// out, some libraries put it in.
func decodeKey(text string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(text, "="))
}

// Available says whether push can work with the server at publicURL, its
// -public-url, and why not when it can't. Browsers take push subscriptions
// only on a secure page, and phones away from this machine reach it only
// at its public address, so push needs that address, over https.
func Available(publicURL string) (bool, string) {
	switch {
	case publicURL == "":
		return false, "Push needs an https address. Start Agentry with -public-url https://… so phones can reach it."
	case !strings.HasPrefix(publicURL, "https://"):
		return false, "Push needs an https address, and -public-url is " + publicURL + ". Start Agentry with -public-url https://…, such as a proxy that does TLS in front of it."
	}
	return true, ""
}

// writeAtomically writes path with write, so that it is either the old
// file or the whole new one: a temporary file beside it, mode 0600 as it
// holds the private key, synced and renamed over it.
func writeAtomically(path string, write func(io.Writer) error) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), filepath.Base(leftovers(path))) // mode 0600
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			temporary.Close()
			os.Remove(temporary.Name())
		}
	}()
	if err := write(temporary); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}
