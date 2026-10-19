package web

import "testing"

func TestYouTurnTheVisionariesAfterAnEpicOffAndOn(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "GET", "/api/settings", ""); code != 200 || body != `{"visionariesOnEpicClose":true}`+"\n" {
		t.Errorf("never changed, it is on: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{"visionariesOnEpicClose":false}`); code != 200 || body != `{"visionariesOnEpicClose":false}`+"\n" {
		t.Errorf("turned off: %d %s", code, body)
	}
	if _, body := call(t, server, "GET", "/api/settings", ""); body != `{"visionariesOnEpicClose":false}`+"\n" {
		t.Errorf("it stays off: %s", body)
	}
	if code, body := call(t, server, "PUT", "/api/settings", `{}`); code != 400 {
		t.Errorf("naming no setting is a bad request: %d %s", code, body)
	}
	if _, body := call(t, server, "PUT", "/api/settings", `{"visionariesOnEpicClose":true}`); body != `{"visionariesOnEpicClose":true}`+"\n" {
		t.Errorf("turned on again: %s", body)
	}
}
