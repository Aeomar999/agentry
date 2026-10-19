package webpush

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)

// browser is a subscription as a browser would make it, at endpoint.
func browser(t *testing.T, endpoint string) Subscription {
	t.Helper()
	sub, _ := receiver(t, endpoint)
	return sub
}

func open(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestASubscriptionAndTheKeysSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	first := open(t, path)
	phone := browser(t, "https://push.example/phone")
	if err := first.Subscribe(phone, now); err != nil {
		t.Fatal(err)
	}
	again := open(t, path)
	if again.PublicKey() != first.PublicKey() || !again.Key().Equal(first.Key()) {
		t.Errorf("a restart made new keys: %s, then %s", first.PublicKey(), again.PublicKey())
	}
	got := again.Subscriptions()
	if len(got) != 1 || got[0].Endpoint != phone.Endpoint || got[0].Keys != phone.Keys || !got[0].Added.Equal(now) {
		t.Errorf("after a restart: %+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the file holds the private key, so its owner alone reads it: %v", info.Mode())
	}
	if leftovers, _ := filepath.Glob(path + ".tmp-*"); len(leftovers) > 0 {
		t.Errorf("writes left temporary files: %v", leftovers)
	}
}

func TestThePublicKeyIsAnUncompressedPoint(t *testing.T) {
	point, err := base64.RawURLEncoding.DecodeString(open(t, filepath.Join(t.TempDir(), File)).PublicKey())
	if err != nil || len(point) != 65 || point[0] != 4 {
		t.Errorf("want 65 bytes starting 0x04, got %x (%v)", point, err)
	}
}

func TestSubscribingAgainKeepsOneSubscription(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), File))
	phone := browser(t, "https://push.example/phone")
	s.Subscribe(phone, now)
	renewed := browser(t, phone.Endpoint)
	if err := s.Subscribe(renewed, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got := s.Subscriptions()
	if len(got) != 1 || got[0].Keys != renewed.Keys || !got[0].Added.Equal(now) {
		t.Errorf("one subscription with the new keys, added when it first was: %+v", got)
	}
}

func TestUnsubscribingRemovesOnlyThatEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	s := open(t, path)
	s.Subscribe(browser(t, "https://push.example/phone"), now)
	s.Subscribe(browser(t, "https://push.example/laptop"), now)
	if removed, err := s.Unsubscribe("https://push.example/phone"); !removed || err != nil {
		t.Fatalf("removed %v, %v", removed, err)
	}
	if removed, err := s.Unsubscribe("https://push.example/phone"); removed || err != nil {
		t.Errorf("a second time there is none: %v, %v", removed, err)
	}
	got := open(t, path).Subscriptions()
	if len(got) != 1 || got[0].Endpoint != "https://push.example/laptop" {
		t.Errorf("after a restart: %+v", got)
	}
}

func TestAMalformedSubscriptionIsRefused(t *testing.T) {
	good := browser(t, "https://push.example/phone")
	for name, change := range map[string]func(*Subscription){
		"no endpoint":       func(s *Subscription) { s.Endpoint = "" },
		"an http endpoint":  func(s *Subscription) { s.Endpoint = "http://push.example/phone" },
		"no host":           func(s *Subscription) { s.Endpoint = "https:///phone" },
		"not a URL":         func(s *Subscription) { s.Endpoint = "https://push example/%zz" },
		"no p256dh":         func(s *Subscription) { s.Keys.P256dh = "" },
		"no auth":           func(s *Subscription) { s.Keys.Auth = "" },
		"p256dh not base64": func(s *Subscription) { s.Keys.P256dh = "not base64!" },
		"p256dh too short":  func(s *Subscription) { s.Keys.P256dh = s.Keys.P256dh[:20] },
		"auth of 8 bytes":   func(s *Subscription) { s.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 8)) },
		"p256dh compressed": func(s *Subscription) {
			s.Keys.P256dh = base64.RawURLEncoding.EncodeToString(append([]byte{2}, make([]byte, 64)...))
		},
		"keys swapped round": func(s *Subscription) { s.Keys.P256dh, s.Keys.Auth = s.Keys.Auth, s.Keys.P256dh },
	} {
		path := filepath.Join(t.TempDir(), File)
		s := open(t, path)
		bad := good
		change(&bad)
		if err := s.Subscribe(bad, now); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: want ErrMalformed, got %v", name, err)
		}
		if got := open(t, path).Subscriptions(); len(got) != 0 {
			t.Errorf("%s: kept %+v", name, got)
		}
	}
	padded := good
	padded.Keys.Auth += "=="
	if err := Check(padded); err != nil {
		t.Errorf("padded base64url is still base64url: %v", err)
	}
}

func TestAFileThatCantBeReadIsAnErrorNotNewKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	os.WriteFile(path, []byte("{not json"), 0o600)
	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("want an error naming the file, got %v", err)
	} else if !strings.Contains(err.Error(), "move it aside to start with new keys") {
		t.Errorf("want how to recover, got %v", err)
	}
	if saved, _ := os.ReadFile(path); string(saved) != "{not json" {
		t.Errorf("the file was replaced: %s", saved)
	}
}

func TestOpeningRemovesUnfinishedWritesOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	open(t, path)
	kept, _ := os.ReadFile(path)
	leftover := path + ".tmp-123456"
	os.WriteFile(leftover, []byte(`{"privateKey":"a half-written key"`), 0o600)

	open(t, path)
	if _, err := os.Stat(leftover); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the unfinished write is still there: %v", err)
	}
	if saved, _ := os.ReadFile(path); string(saved) != string(kept) {
		t.Errorf("the file changed:\n%s\nwas\n%s", saved, kept)
	}
}

func TestPushIsAvailableOnlyAtAnHTTPSAddress(t *testing.T) {
	for _, c := range []struct {
		publicURL string
		available bool
		reason    string
	}{
		{"", false, "Start Agentry with -public-url https://"},
		{"http://agentry.home:7800", false, "-public-url is http://agentry.home:7800"},
		{"https://agentry.example", true, ""},
	} {
		available, reason := Available(c.publicURL)
		if available != c.available || !strings.Contains(reason, c.reason) || (c.reason == "") != (reason == "") {
			t.Errorf("%q: got %v %q", c.publicURL, available, reason)
		}
	}
	_, none := Available("")
	_, plain := Available("http://agentry.home")
	if none == plain {
		t.Errorf("no address and an http one have their own reasons: %q", none)
	}
}
