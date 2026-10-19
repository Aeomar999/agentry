package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestYouTurnTheVisionariesAfterAnEpicOffAndOn(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "GET", "/api/settings", ""); code != 200 || body != `{"visionariesOnEpicClose":true,"autoAcceptIdeas":false,"autoStartIdeas":false}`+"\n" {
		t.Errorf("never changed, it is on: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{"visionariesOnEpicClose":false}`); code != 200 || body != `{"visionariesOnEpicClose":false,"autoAcceptIdeas":false,"autoStartIdeas":false}`+"\n" {
		t.Errorf("turned off: %d %s", code, body)
	}
	if _, body := call(t, server, "GET", "/api/settings", ""); body != `{"visionariesOnEpicClose":false,"autoAcceptIdeas":false,"autoStartIdeas":false}`+"\n" {
		t.Errorf("it stays off: %s", body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{}`); code != 400 {
		t.Errorf("naming no setting is a bad request: %d %s", code, body)
	}
	if _, body := call(t, server, "PUT", "/api/settings", `{"visionariesOnEpicClose":true}`); body != `{"visionariesOnEpicClose":true,"autoAcceptIdeas":false,"autoStartIdeas":false}`+"\n" {
		t.Errorf("turned on again: %s", body)
	}
}

// A request that changes nothing it can read is refused, and the setting
// stays as it was; without the UI token nothing changes either.
func TestABadSettingsChangeLeavesTheSettingAlone(t *testing.T) {
	server, _ := newTestServer(t)
	call(t, server, "PUT", "/api/settings", `{"visionariesOnEpicClose":false}`)
	for _, body := range []string{`{"visionariesOnEpicClose":null}`, `{"visionariesOnEpicClose":"yes"}`, `{"another":true}`, `{bad`, ``} {
		if code, answer := call(t, server, "PUT", "/api/settings", body); code != 400 {
			t.Errorf("%s: want 400, got %d %s", body, code, answer)
		}
	}
	request, _ := http.NewRequest("PUT", server.URL+"/api/settings", strings.NewReader(`{"visionariesOnEpicClose":true}`))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Errorf("no token: want 401, got %d", response.StatusCode)
	}
	if _, body := call(t, server, "GET", "/api/settings", ""); body != `{"visionariesOnEpicClose":false,"autoAcceptIdeas":false,"autoStartIdeas":false}`+"\n" {
		t.Errorf("it should still be off: %s", body)
	}
}

func TestYouTurnAutoAcceptingIdeasOnAndOff(t *testing.T) {
	server, _ := newTestServer(t)
	off := `{"visionariesOnEpicClose":true,"autoAcceptIdeas":false,"autoStartIdeas":false}` + "\n"
	if _, body := call(t, server, "GET", "/api/settings", ""); body != off {
		t.Errorf("never changed, both are off: %s", body)
	}
	both := `{"visionariesOnEpicClose":true,"autoAcceptIdeas":true,"autoStartIdeas":true}` + "\n"
	if code, body := call(t, server, "PUT", "/api/settings", `{"autoAcceptIdeas":true,"autoStartIdeas":true}`); code != 200 || body != both {
		t.Errorf("turned both on: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{"autoStartIdeas":false}`); code != 200 ||
		body != `{"visionariesOnEpicClose":true,"autoAcceptIdeas":true,"autoStartIdeas":false}`+"\n" {
		t.Errorf("turned starting off, accepting stays on: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{"autoAcceptIdeas":"yes"}`); code != 400 {
		t.Errorf("not a boolean: want 400, got %d %s", code, body)
	}
}
