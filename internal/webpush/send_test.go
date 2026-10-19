package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func decoded(t *testing.T, text string) []byte {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The example of RFC 8291, Appendix A.
func TestEncryptionMatchesTheRFCExample(t *testing.T) {
	sender, err := ecdh.P256().NewPrivateKey(decoded(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	keys := Keys{P256dh: "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4", Auth: "BTBZMqHH6r4Tts7J_aSIgg"}
	salt := decoded(t, "DGv6ra1nlYgDCS1FRnbzlw")
	got, err := encrypt([]byte("When I grow up, I want to be a watermelon"), keys, sender, salt)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if encode(got) != want {
		t.Errorf("got  %s\nwant %s", encode(got), want)
	}
}

func TestAMessageTooLongForAPushIsRefused(t *testing.T) {
	phone, _ := receiver(t, "https://push.example/phone")
	if _, err := seal(make([]byte, maxMessage), phone.Keys); err != nil {
		t.Errorf("the longest message: %v", err)
	}
	if _, err := seal(make([]byte, maxMessage+1), phone.Keys); !errors.Is(err, ErrTooLong) {
		t.Errorf("a byte more: want ErrTooLong, got %v", err)
	}
}

func TestTheVAPIDTokenVerifiesAndNamesThePushService(t *testing.T) {
	store := open(t, filepath.Join(t.TempDir(), File))
	sender := NewSender(store, http.DefaultClient, "https://agentry.example")
	sender.now = func() time.Time { return now }
	header, err := sender.vapid("https://push.example:8443/send/abc?x=1")
	if err != nil {
		t.Fatal(err)
	}
	token, key, ok := strings.Cut(strings.TrimPrefix(header, "vapid t="), ", k=")
	if !strings.HasPrefix(header, "vapid t=") || !ok || key != store.PublicKey() {
		t.Fatalf("want vapid t=<token>, k=%s, got %s", store.PublicKey(), header)
	}
	claims := verified(t, token, &store.Key().PublicKey)
	want := map[string]any{"aud": "https://push.example:8443", "exp": float64(now.Add(12 * time.Hour).Unix()), "sub": "https://agentry.example"}
	if len(claims) != len(want) || claims["aud"] != want["aud"] || claims["exp"] != want["exp"] || claims["sub"] != want["sub"] {
		t.Errorf("claims: got %v, want %v", claims, want)
	}
}

// verified is the claims of token, failing t unless it is an ES256 JWT
// signed by key.
func verified(t *testing.T, token string, key *ecdsa.PublicKey) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("a JWT has three parts: %s", token)
	}
	var header map[string]string
	if err := json.Unmarshal(decoded(t, parts[0]), &header); err != nil || header["typ"] != "JWT" || header["alg"] != "ES256" || len(header) != 2 {
		t.Errorf("header: %v (%v)", header, err)
	}
	signature := decoded(t, parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(signature) != 64 || !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		t.Fatalf("the signature doesn't verify with the public key: %x", signature)
	}
	var claims map[string]any
	if err := json.Unmarshal(decoded(t, parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

// receiver is a subscription at endpoint and the browser's private key,
// which reads what is sent to it.
func receiver(t *testing.T, endpoint string) (Subscription, *ecdh.PrivateKey) {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 16)
	rand.Read(secret)
	return Subscription{Endpoint: endpoint, Keys: Keys{P256dh: encode(key.PublicKey().Bytes()), Auth: encode(secret)}}, key
}

// decrypt reads body as the browser holding key and sub's auth secret
// would.
func decrypt(t *testing.T, body []byte, sub Subscription, key *ecdh.PrivateKey) string {
	t.Helper()
	if len(body) < headerSize || binary.BigEndian.Uint32(body[16:20]) != recordSize || body[20] != 65 {
		t.Fatalf("not an aes128gcm header: %x", body)
	}
	salt, senderPoint, record := body[:16], body[21:headerSize], body[headerSize:]
	sender, err := ecdh.P256().NewPublicKey(senderPoint)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := key.ECDH(sender)
	if err != nil {
		t.Fatal(err)
	}
	gcm, nonce, err := contentKeys(shared, decoded(t, sub.Keys.Auth), key.PublicKey().Bytes(), senderPoint, salt)
	if err != nil {
		t.Fatal(err)
	}
	padded, err := gcm.Open(nil, nonce, record, nil)
	if err != nil || len(padded) == 0 || padded[len(padded)-1] != 2 {
		t.Fatalf("the record doesn't open to a padded last record: %x (%v)", padded, err)
	}
	return string(padded[:len(padded)-1])
}

// pushService is a fake push service that answers every push with status
// and keeps the last one.
type pushService struct {
	*httptest.Server
	status int
	last   *http.Request
	body   []byte
}

func newPushService(t *testing.T, status int) *pushService {
	p := &pushService{status: status}
	p.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.last = r
		p.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(p.status)
		io.WriteString(w, "a reason\n")
	}))
	t.Cleanup(p.Close)
	return p
}

func TestAPushReachesThePushServiceAndOpensForTheBrowser(t *testing.T) {
	service := newPushService(t, http.StatusCreated)
	store := open(t, filepath.Join(t.TempDir(), File))
	phone, key := receiver(t, service.URL+"/send/phone")
	store.Subscribe(phone, now)
	sender := NewSender(store, service.Client(), "https://agentry.example")

	if err := sender.Send(context.Background(), phone, []byte(`{"task":"task-117"}`)); err != nil {
		t.Fatal(err)
	}
	if got := decrypt(t, service.body, phone, key); got != `{"task":"task-117"}` {
		t.Errorf("the browser reads %q", got)
	}
	want := map[string]string{"Content-Encoding": "aes128gcm", "Content-Type": "application/octet-stream", "Ttl": "86400", "Urgency": "high"}
	for name, value := range want {
		if got := service.last.Header.Get(name); got != value {
			t.Errorf("%s: got %q, want %q", name, got, value)
		}
	}
	token, _, _ := strings.Cut(strings.TrimPrefix(service.last.Header.Get("Authorization"), "vapid t="), ", k=")
	if aud := verified(t, token, &store.Key().PublicKey)["aud"]; aud != service.URL {
		t.Errorf("aud: got %v, want %s", aud, service.URL)
	}
	if service.last.Method != http.MethodPost || service.last.URL.Path != "/send/phone" {
		t.Errorf("got %s %s", service.last.Method, service.last.URL)
	}
	if len(store.Subscriptions()) != 1 {
		t.Errorf("a push that arrived removed its subscription")
	}
}

func TestASubscriptionThePushServiceNoLongerHasIsRemoved(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		service := newPushService(t, status)
		path := filepath.Join(t.TempDir(), File)
		store := open(t, path)
		phone, _ := receiver(t, service.URL+"/send/phone")
		laptop, _ := receiver(t, service.URL+"/send/laptop")
		store.Subscribe(phone, now)
		store.Subscribe(laptop, now)

		err := NewSender(store, service.Client(), "https://agentry.example").Send(context.Background(), phone, []byte("hello"))
		if !errors.Is(err, ErrGone) {
			t.Errorf("%d: want ErrGone, got %v", status, err)
		}
		for _, s := range []*Store{store, open(t, path)} {
			if got := s.Subscriptions(); len(got) != 1 || got[0].Endpoint != laptop.Endpoint {
				t.Errorf("%d: want only the laptop left, got %+v", status, got)
			}
		}
	}
}

func TestAnyOtherRefusalIsAnErrorNamingTheStatus(t *testing.T) {
	service := newPushService(t, http.StatusTooManyRequests)
	store := open(t, filepath.Join(t.TempDir(), File))
	phone, _ := receiver(t, service.URL+"/send/phone")
	store.Subscribe(phone, now)

	err := NewSender(store, service.Client(), "https://agentry.example").Send(context.Background(), phone, []byte("hello"))
	if err == nil || errors.Is(err, ErrGone) || !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "a reason") {
		t.Errorf("want an error naming 429 and the reason, got %v", err)
	}
	if len(store.Subscriptions()) != 1 {
		t.Errorf("a refused push removed its subscription")
	}
}

func TestASendStopsWithItsContext(t *testing.T) {
	service := newPushService(t, http.StatusCreated)
	store := open(t, filepath.Join(t.TempDir(), File))
	phone, _ := receiver(t, service.URL+"/send/phone")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewSender(store, service.Client(), "https://agentry.example").Send(ctx, phone, []byte("hello"))
	if !errors.Is(err, context.Canceled) || service.last != nil {
		t.Errorf("want context.Canceled and no push, got %v", err)
	}
}
