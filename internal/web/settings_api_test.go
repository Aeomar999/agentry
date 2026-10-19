package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestYouTurnTheVisionariesAfterAnEpicOffAndOn(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "GET", "/api/settings", ""); code != 200 || body != `{"visionariesOnEpicClose":true,"autoAcceptIdeas":false,"autoStartIdeas":false,"autoAcceptRule":"everyoneBacked","maxRunningTasks":0,"runningTasks":0}`+"\n" {
		t.Errorf("never changed, it is on: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{"visionariesOnEpicClose":false}`); code != 200 || body != `{"visionariesOnEpicClose":false,"autoAcceptIdeas":false,"autoStartIdeas":false,"autoAcceptRule":"everyoneBacked","maxRunningTasks":0,"runningTasks":0}`+"\n" {
		t.Errorf("turned off: %d %s", code, body)
	}
	if _, body := call(t, server, "GET", "/api/settings", ""); body != `{"visionariesOnEpicClose":false,"autoAcceptIdeas":false,"autoStartIdeas":false,"autoAcceptRule":"everyoneBacked","maxRunningTasks":0,"runningTasks":0}`+"\n" {
		t.Errorf("it stays off: %s", body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{}`); code != 400 {
		t.Errorf("naming no setting is a bad request: %d %s", code, body)
	}
	if _, body := call(t, server, "PUT", "/api/settings", `{"visionariesOnEpicClose":true}`); body != `{"visionariesOnEpicClose":true,"autoAcceptIdeas":false,"autoStartIdeas":false,"autoAcceptRule":"everyoneBacked","maxRunningTasks":0,"runningTasks":0}`+"\n" {
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
	if _, body := call(t, server, "GET", "/api/settings", ""); body != `{"visionariesOnEpicClose":false,"autoAcceptIdeas":false,"autoStartIdeas":false,"autoAcceptRule":"everyoneBacked","maxRunningTasks":0,"runningTasks":0}`+"\n" {
		t.Errorf("it should still be off: %s", body)
	}
}

func TestYouTurnAutoAcceptingIdeasOnAndOff(t *testing.T) {
	server, _ := newTestServer(t)
	off := `{"visionariesOnEpicClose":true,"autoAcceptIdeas":false,"autoStartIdeas":false,"autoAcceptRule":"everyoneBacked","maxRunningTasks":0,"runningTasks":0}` + "\n"
	if _, body := call(t, server, "GET", "/api/settings", ""); body != off {
		t.Errorf("never changed, both are off: %s", body)
	}
	both := `{"visionariesOnEpicClose":true,"autoAcceptIdeas":true,"autoStartIdeas":true,"autoAcceptRule":"everyoneBacked","maxRunningTasks":0,"runningTasks":0}` + "\n"
	if code, body := call(t, server, "PUT", "/api/settings", `{"autoAcceptIdeas":true,"autoStartIdeas":true,"autoAcceptRule":"everyoneBacked"}`); code != 200 || body != both {
		t.Errorf("turned both on: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{"autoStartIdeas":false,"autoAcceptRule":"everyoneBacked"}`); code != 200 ||
		body != `{"visionariesOnEpicClose":true,"autoAcceptIdeas":true,"autoStartIdeas":false,"autoAcceptRule":"everyoneBacked","maxRunningTasks":0,"runningTasks":0}`+"\n" {
		t.Errorf("turned starting off, accepting stays on: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{"autoAcceptIdeas":"yes"}`); code != 400 {
		t.Errorf("not a boolean: want 400, got %d %s", code, body)
	}
}

// You choose which ideas Agentry may accept for you; a rule it doesn't
// know is refused, naming the ones it does, and the rule stays.
func TestYouChooseTheAutoAcceptRule(t *testing.T) {
	server, _ := newTestServer(t)
	loose := `{"visionariesOnEpicClose":true,"autoAcceptIdeas":false,"autoStartIdeas":false,"autoAcceptRule":"nobodyDoubted","maxRunningTasks":0,"runningTasks":0}` + "\n"
	if code, body := call(t, server, "PUT", "/api/settings", `{"autoAcceptRule":"nobodyDoubted"}`); code != 200 || body != loose {
		t.Errorf("loosened: %d %s", code, body)
	}
	for _, bad := range []string{`{"autoAcceptRule":"mostBacked"}`, `{"autoAcceptRule":""}`, `{"autoAcceptRule":true}`} {
		code, body := call(t, server, "PUT", "/api/settings", bad)
		if code != 400 || bad != `{"autoAcceptRule":true}` && (!strings.Contains(body, "everyoneBacked") || !strings.Contains(body, "nobodyDoubted")) {
			t.Errorf("%s: want 400 naming both rules, got %d %s", bad, code, body)
		}
	}
	if _, body := call(t, server, "GET", "/api/settings", ""); body != loose {
		t.Errorf("the rule should stay: %s", body)
	}
}

// You set the most tasks running at once, or 0 for no limit; a limit out of
// range, or not a whole number, is refused and the one before stays.
func TestYouSetTheMostTasksRunningAtOnce(t *testing.T) {
	server, _ := newTestServer(t)
	two := `{"visionariesOnEpicClose":true,"autoAcceptIdeas":false,"autoStartIdeas":false,"autoAcceptRule":"everyoneBacked","maxRunningTasks":2,"runningTasks":0}` + "\n"
	if code, body := call(t, server, "PUT", "/api/settings", `{"maxRunningTasks":2}`); code != 200 || body != two {
		t.Errorf("set to 2: %d %s", code, body)
	}
	for _, bad := range []string{`{"maxRunningTasks":-1}`, `{"maxRunningTasks":101}`, `{"maxRunningTasks":1000000000000}`,
		`{"maxRunningTasks":1.5}`, `{"maxRunningTasks":"3"}`, `{"maxRunningTasks":true}`, `{"maxRunningTasks":null}`} {
		if code, body := call(t, server, "PUT", "/api/settings", bad); code != 400 {
			t.Errorf("%s: want 400, got %d %s", bad, code, body)
		}
	}
	if _, body := call(t, server, "GET", "/api/settings", ""); body != two {
		t.Errorf("the limit should stay 2: %s", body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{"maxRunningTasks":100}`); code != 200 || !strings.Contains(body, `"maxRunningTasks":100,`) {
		t.Errorf("the highest limit: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{"maxRunningTasks":0}`); code != 200 || !strings.Contains(body, `"maxRunningTasks":0,`) {
		t.Errorf("cleared: %d %s", code, body)
	}
}
