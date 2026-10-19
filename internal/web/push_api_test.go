package web

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/webpush"
)

// newPushServer is a server on the data folder dir, its log and its push
// subscriptions, served at publicURL.
func newPushServer(t *testing.T, dir, publicURL string) (*httptest.Server, *store.Store) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(dir, "agentry.db"))
	push, err := webpush.Open(filepath.Join(dir, webpush.File))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, PublicURL: publicURL, Push: push}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server, s
}

// subscriptionJSON is a PushSubscription as a browser's toJSON gives it.
func subscriptionJSON(t *testing.T, endpoint string) string {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 16)
	rand.Read(secret)
	body, _ := json.Marshal(map[string]any{"endpoint": endpoint, "expirationTime": nil, "keys": map[string]string{
		"p256dh": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), "auth": base64.RawURLEncoding.EncodeToString(secret)}})
	return string(body)
}

func readPush(t *testing.T, server *httptest.Server) pushView {
	t.Helper()
	code, body := call(t, server, "GET", "/api/push", "")
	var view pushView
	if err := json.Unmarshal([]byte(body), &view); code != 200 || err != nil {
		t.Fatalf("GET /api/push: %d %s", code, body)
	}
	return view
}

func TestAPushSubscriptionSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	first, _ := newPushServer(t, dir, "https://agentry.example")
	before := readPush(t, first)
	if code, body := call(t, first, "POST", "/api/push/subscriptions", subscriptionJSON(t, "https://push.example/phone")); code != 200 {
		t.Fatalf("subscribing: %d %s", code, body)
	}
	first.Close()

	again, _ := newPushServer(t, dir, "https://agentry.example")
	after := readPush(t, again)
	if after.Subscribed != 1 || after.PublicKey != before.PublicKey || after.PublicKey == "" {
		t.Errorf("after a restart, want the subscription and the same key %s, got %+v", before.PublicKey, after)
	}
}

func TestPushDataStaysOutOfTheLog(t *testing.T) {
	server, log := newPushServer(t, t.TempDir(), "https://agentry.example")
	position := log.LastPosition()
	call(t, server, "POST", "/api/push/subscriptions", subscriptionJSON(t, "https://push.example/phone"))
	call(t, server, "POST", "/api/push/subscriptions", subscriptionJSON(t, "https://push.example/laptop"))
	call(t, server, "DELETE", "/api/push/subscriptions?endpoint="+url.QueryEscape("https://push.example/laptop"), "")
	if readPush(t, server).Subscribed != 1 {
		t.Fatal("the subscriptions weren't kept")
	}
	if log.LastPosition() != position {
		t.Errorf("subscribing wrote to the log: position %d, then %d", position, log.LastPosition())
	}
	all, err := log.After(0, 100000)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range all {
		if text, _ := json.Marshal(e); strings.Contains(string(text), "push.example") {
			t.Errorf("an event holds a subscription: %s", text)
		}
	}
}

func TestPushNeedsAnHTTPSPublicURL(t *testing.T) {
	for _, c := range []struct {
		publicURL string
		available bool
		reason    string
	}{
		{"", false, "Start Agentry with -public-url https://"},
		{"http://agentry.home:7800", false, "-public-url is http://agentry.home:7800"},
		{"https://agentry.example", true, ""},
	} {
		server, _ := newPushServer(t, t.TempDir(), c.publicURL)
		view := readPush(t, server)
		if view.Available != c.available || !strings.Contains(view.Reason, c.reason) || (c.reason == "") != (view.Reason == "") {
			t.Errorf("%q: %+v", c.publicURL, view)
		}
		if (view.PublicKey != "") != c.available {
			t.Errorf("%q: the key is given only when push is available: %+v", c.publicURL, view)
		}
		code, body := call(t, server, "POST", "/api/push/subscriptions", subscriptionJSON(t, "https://push.example/phone"))
		if c.available != (code == 200) || (!c.available && !strings.Contains(body, c.reason)) {
			t.Errorf("%q: subscribing: %d %s", c.publicURL, code, body)
		}
	}
}

func TestAMalformedPushSubscriptionIsABadRequest(t *testing.T) {
	server, _ := newPushServer(t, t.TempDir(), "https://agentry.example")
	good := subscriptionJSON(t, "https://push.example/phone")
	for name, body := range map[string]string{
		"not JSON":         `{bad`,
		"no endpoint":      strings.Replace(good, `"endpoint":"https://push.example/phone"`, `"endpoint":""`, 1),
		"an http endpoint": strings.Replace(good, `"https://push.example/phone"`, `"http://push.example/phone"`, 1),
		"no keys":          `{"endpoint":"https://push.example/phone"}`,
	} {
		if code, answer := call(t, server, "POST", "/api/push/subscriptions", body); code != 400 || answer == "" {
			t.Errorf("%s: want 400 with a reason, got %d %s", name, code, answer)
		}
	}
	if view := readPush(t, server); view.Subscribed != 0 {
		t.Errorf("a refused subscription was kept: %+v", view)
	}
}

func TestSubscribingTwiceAndUnsubscribing(t *testing.T) {
	server, _ := newPushServer(t, t.TempDir(), "https://agentry.example")
	phone := "https://push.example/phone"
	call(t, server, "POST", "/api/push/subscriptions", subscriptionJSON(t, phone))
	call(t, server, "POST", "/api/push/subscriptions", subscriptionJSON(t, phone))
	if view := readPush(t, server); view.Subscribed != 1 {
		t.Errorf("one endpoint is one subscription: %+v", view)
	}
	remove := "/api/push/subscriptions?endpoint=" + url.QueryEscape(phone)
	if code, body := call(t, server, "DELETE", remove, ""); code != 200 || !strings.Contains(body, `"subscribed":0`) {
		t.Errorf("unsubscribing: %d %s", code, body)
	}
	if code, _ := call(t, server, "DELETE", remove, ""); code != 404 {
		t.Errorf("a second time there is none: %d", code)
	}
}

func TestPushNeedsTheUIToken(t *testing.T) {
	server, _ := newPushServer(t, t.TempDir(), "https://agentry.example")
	for _, method := range []string{"GET", "POST", "DELETE"} {
		path := "/api/push/subscriptions"
		if method == "GET" {
			path = "/api/push"
		}
		request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(subscriptionJSON(t, "https://push.example/phone")))
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Errorf("%s %s without the token: %d", method, path, response.StatusCode)
		}
	}
}
