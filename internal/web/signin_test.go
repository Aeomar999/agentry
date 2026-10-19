package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
)

// signinServer is a server whose clock the test moves, called in process so
// each request can come from its own address.
type signinServer struct {
	t       *testing.T
	handler http.Handler
	now     time.Time
}

func newSigninServer(t *testing.T) *signinServer {
	t.Helper()
	s, _ := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	test := &signinServer{t: t, now: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	server := NewServer(s, Config{UIToken: token, DeskToken: "desk-secret", NetworkURLs: []string{"http://192.168.1.5:7800"}}, http.NotFoundHandler(), nil)
	server.signins = newSignins(func() time.Time { return test.now })
	test.handler = server.Handler()
	return test
}

// send makes a request from client, with each header as "Name: value".
func (s *signinServer) send(method, path, body, client string, headers ...string) *http.Response {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.RemoteAddr = client + ":51000"
	for _, header := range headers {
		name, value, _ := strings.Cut(header, ": ")
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder.Result()
}

func (s *signinServer) issue() string {
	s.t.Helper()
	response := s.send("POST", "/api/signin/codes", "", "10.0.0.1", "Authorization: Bearer "+token)
	var issued struct{ Code string }
	if err := json.NewDecoder(response.Body).Decode(&issued); err != nil || response.StatusCode != 200 {
		s.t.Fatalf("issue: %d %v", response.StatusCode, err)
	}
	return issued.Code
}

func (s *signinServer) signIn(body, client string, headers ...string) *http.Response {
	return s.send("POST", "/api/signin", body, client, headers...)
}

func uiCookieOf(response *http.Response) *http.Cookie {
	for _, cookie := range response.Cookies() {
		if cookie.Name == uiCookie {
			return cookie
		}
	}
	return nil
}

func TestOnlyTheUITokenIssuesSigninCodes(t *testing.T) {
	s := newSigninServer(t)

	// A connected Claude Code's credential opens the API, but not this.
	invite := s.send("POST", "/api/integrations/claude-code", "", "10.0.0.1", "Authorization: Bearer "+token)
	var command struct{ Command string }
	json.NewDecoder(invite.Body).Decode(&command)
	installCode := command.Command[strings.Index(command.Command, "code=")+5 : strings.LastIndex(command.Command, "'")]
	connected := s.send("POST", "/api/integrations/connect", `{"code":"`+installCode+`","host":"laptop"}`, "10.0.0.2")
	var made struct{ Credential string }
	if json.NewDecoder(connected.Body).Decode(&made); made.Credential == "" {
		t.Fatalf("connect: %d", connected.StatusCode)
	}
	if response := s.send("GET", "/api/tasks", "", "10.0.0.2", "Authorization: Bearer "+made.Credential); response.StatusCode != 200 {
		t.Fatalf("the credential should open the API: %d", response.StatusCode)
	}

	for _, refused := range []string{"", "Authorization: Bearer wrong", "Authorization: Bearer desk-secret", "Authorization: Bearer " + made.Credential} {
		if response := s.send("POST", "/api/signin/codes", "", "10.0.0.2", refused); response.StatusCode != http.StatusUnauthorized {
			t.Errorf("issuing with %q: %d, want 401", refused, response.StatusCode)
		}
	}

	if response := s.send("POST", "/api/signin/codes?token="+token, "", "10.0.0.1"); response.StatusCode != 200 {
		t.Errorf("issuing with ?token=: %d", response.StatusCode)
	}
	response := s.send("POST", "/api/signin/codes", "", "10.0.0.1", "Cookie: "+uiCookie+"="+token)
	var issued struct {
		Code      string
		ExpiresAt time.Time
		ExpiresIn int
		Addresses []struct{ URL, Label string }
		Address   string
	}
	if err := json.NewDecoder(response.Body).Decode(&issued); err != nil || response.StatusCode != 200 {
		t.Fatalf("issuing with the cookie: %d %v", response.StatusCode, err)
	}
	if !regexp.MustCompile(`^[A-HJKMNP-Z2-9]{4}-[A-HJKMNP-Z2-9]{4}$`).MatchString(issued.Code) {
		t.Errorf("code %q", issued.Code)
	}
	if !issued.ExpiresAt.Equal(s.now.Add(5*time.Minute)) || issued.ExpiresIn != 300 || issued.Address != "http://example.com" ||
		len(issued.Addresses) != 2 || issued.Addresses[0] != (struct{ URL, Label string }{"http://192.168.1.5:7800", "On your network"}) {
		t.Errorf("issued: %+v", issued)
	}
}

func TestASigninCodeSignsInOnce(t *testing.T) {
	s := newSigninServer(t)
	code := s.issue()

	typed := strings.ToLower(strings.ReplaceAll(code, "-", " - "))
	response := s.signIn(`{"code":"`+typed+`"}`, "10.0.0.2")
	var signedIn struct{ Token string }
	json.NewDecoder(response.Body).Decode(&signedIn)
	if response.StatusCode != 200 || signedIn.Token != token {
		t.Fatalf("sign in with %q: %d %+v", typed, response.StatusCode, signedIn)
	}
	cookie := uiCookieOf(response)
	if cookie == nil || cookie.Value != token || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" ||
		cookie.MaxAge < 300*24*60*60 || cookie.Secure {
		t.Errorf("cookie over plain http: %+v", cookie)
	}

	// The cookie alone opens the API: an installed app may keep only it.
	if response := s.send("GET", "/api/tasks", "", "10.0.0.2", "Cookie: "+uiCookie+"="+cookie.Value); response.StatusCode != 200 {
		t.Errorf("the API with the cookie: %d", response.StatusCode)
	}

	if response := s.signIn(`{"code":"`+code+`"}`, "10.0.0.2"); response.StatusCode != http.StatusUnauthorized || uiCookieOf(response) != nil {
		t.Errorf("a used code: %d", response.StatusCode)
	}
}

func TestASigninCodeExpires(t *testing.T) {
	s := newSigninServer(t)
	stale, fresh := s.issue(), s.issue()
	s.now = s.now.Add(5 * time.Minute)
	if response := s.signIn(`{"code":"`+stale+`"}`, "10.0.0.2"); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("a code five minutes old: %d", response.StatusCode)
	}

	s.now = s.now.Add(-time.Second)
	if response := s.signIn(`{"code":"`+fresh+`"}`, "10.0.0.2"); response.StatusCode != 200 {
		t.Errorf("a code a second short of expiring: %d", response.StatusCode)
	}
}

func TestIssuingPastTheCapDropsTheOldestCode(t *testing.T) {
	s := newSigninServer(t)
	var codes []string
	for range maxLiveCodes + 1 {
		codes = append(codes, s.issue())
		s.now = s.now.Add(time.Second)
	}
	if response := s.signIn(`{"code":"`+codes[0]+`"}`, "10.0.0.2"); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("the oldest code: %d", response.StatusCode)
	}
	for _, code := range []string{codes[1], codes[maxLiveCodes]} {
		if response := s.signIn(`{"code":"`+code+`"}`, "10.0.0.2"); response.StatusCode != 200 {
			t.Errorf("code %s: %d", code, response.StatusCode)
		}
	}
}

func TestThePastedTokenSignsIn(t *testing.T) {
	s := newSigninServer(t)
	if response := s.signIn(`{"token":"wrong"}`, "10.0.0.2"); response.StatusCode != http.StatusUnauthorized || uiCookieOf(response) != nil {
		t.Errorf("a wrong token: %d", response.StatusCode)
	}
	if response := s.signIn(`{}`, "10.0.0.2"); response.StatusCode != http.StatusBadRequest {
		t.Errorf("neither code nor token: %d", response.StatusCode)
	}

	// Behind an https proxy the cookie only goes back over https.
	response := s.signIn(`{"token":"`+token+`"}`, "10.0.0.2", "X-Forwarded-Proto: https")
	if cookie := uiCookieOf(response); response.StatusCode != 200 || cookie == nil || cookie.Value != token || !cookie.Secure {
		t.Errorf("the right token over https: %d %+v", response.StatusCode, cookie)
	}
	page := s.send("GET", "/?token="+token, "", "10.0.0.2", "X-Forwarded-Proto: https")
	if cookie := uiCookieOf(page); cookie == nil || !cookie.Secure || !cookie.HttpOnly {
		t.Errorf("the printed link over https: %+v", cookie)
	}
}

func TestFailedSigninsAreLimited(t *testing.T) {
	s := newSigninServer(t)
	code := s.issue()
	for range failuresPerClient {
		s.signIn(`{"code":"WRONG-CODE"}`, "10.0.0.66")
	}
	response := s.signIn(`{"code":"`+code+`"}`, "10.0.0.66")
	if response.StatusCode != http.StatusTooManyRequests || response.Header.Get("Retry-After") != "60" {
		t.Errorf("past the failures, even a good code: %d, Retry-After %q", response.StatusCode, response.Header.Get("Retry-After"))
	}
	if response := s.signIn(`{"token":"`+token+`"}`, "10.0.0.66"); response.StatusCode != http.StatusTooManyRequests {
		t.Errorf("past the failures, the token: %d", response.StatusCode)
	}
	// Someone else's failures don't stop you.
	if response := s.signIn(`{"code":"`+code+`"}`, "10.0.0.2"); response.StatusCode != 200 {
		t.Errorf("another client: %d", response.StatusCode)
	}
	s.now = s.now.Add(time.Minute)
	if response := s.signIn(`{"token":"`+token+`"}`, "10.0.0.66"); response.StatusCode != 200 {
		t.Errorf("a minute later: %d", response.StatusCode)
	}

	// Guessing codes from many addresses stops codes for everyone for a
	// while, but not the token.
	code = s.issue()
	for i := range codeFailuresInAll {
		s.signIn(`{"code":"WRONG-CODE"}`, "10.0.1."+itoa(int64(i)))
	}
	if response := s.signIn(`{"code":"`+code+`"}`, "10.0.0.3"); response.StatusCode != http.StatusTooManyRequests {
		t.Errorf("a code past the cap for all: %d", response.StatusCode)
	}
	if response := s.signIn(`{"token":"`+token+`"}`, "10.0.0.3"); response.StatusCode != 200 {
		t.Errorf("the token past the cap for all: %d", response.StatusCode)
	}
	s.now = s.now.Add(time.Minute)
	if response := s.signIn(`{"code":"`+code+`"}`, "10.0.0.3"); response.StatusCode != 200 {
		t.Errorf("a code a minute later: %d", response.StatusCode)
	}
}

// Wrong codes sent at once must not all pass the check before any of them
// is counted.
func TestFailedSigninsSentAtOnceAreLimited(t *testing.T) {
	s := newSigninServer(t)
	statuses := make(chan int, 50)
	var sent sync.WaitGroup
	for range cap(statuses) {
		sent.Go(func() { statuses <- s.signIn(`{"code":"WRONG-CODE"}`, "10.0.0.66").StatusCode })
	}
	sent.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[http.StatusUnauthorized] != failuresPerClient || counts[http.StatusTooManyRequests] != cap(statuses)-failuresPerClient {
		t.Errorf("50 wrong codes at once: %v, want %d refused and the rest told to wait", counts, failuresPerClient)
	}
}

// A good attempt is counted while it is checked, then taken back.
func TestSigningInDoesNotCountAsAFailure(t *testing.T) {
	s := newSigninServer(t)
	for range failuresPerClient + 1 {
		if response := s.signIn(`{"token":"`+token+`"}`, "10.0.0.2"); response.StatusCode != 200 {
			t.Fatalf("signing in again: %d", response.StatusCode)
		}
	}
	for range failuresPerClient - 1 {
		s.signIn(`{"code":"WRONG-CODE"}`, "10.0.0.2")
	}
	if response := s.signIn(`{"code":"`+s.issue()+`"}`, "10.0.0.2"); response.StatusCode != 200 {
		t.Errorf("a good code after %d failures: %d", failuresPerClient-1, response.StatusCode)
	}
}

func TestSigningOutClearsTheCookie(t *testing.T) {
	s := newSigninServer(t)
	response := s.send("POST", "/api/signout", "", "10.0.0.2")
	cookie := uiCookieOf(response)
	if response.StatusCode != 200 || cookie == nil || cookie.MaxAge >= 0 || cookie.Value != "" || cookie.Path != "/" {
		t.Errorf("sign out: %d %+v", response.StatusCode, cookie)
	}
}
