package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrGone is a subscription its push service no longer has, as when the
// browser unsubscribed or the app was removed. Send removes it from the
// Store.
var ErrGone = errors.New("the subscription is gone")

// ErrTooLong is a message that doesn't fit in one push.
var ErrTooLong = errors.New("the message is too long for a push")

const (
	recordSize = 4096                  // the one record's size, rs in the header
	headerSize = 16 + 4 + 1 + 65       // salt, rs, idlen, and the sender's public key
	overhead   = headerSize + 1 + 16   // the header, the padding delimiter and the AES-GCM tag
	maxMessage = recordSize - overhead // push services take bodies of up to 4096 bytes
	timeToLive = 24 * time.Hour        // how long a push service keeps a push for a phone that's off
	tokenLife  = 12 * time.Hour        // how long a VAPID token is good for; RFC 8292 allows 24 hours
	errorShown = 200                   // how much of a push service's reply an error quotes
)

// Sender sends pushes to the browsers in a Store, signed with its VAPID key.
type Sender struct {
	store   *Store
	client  *http.Client
	subject string
	now     func() time.Time
}

// NewSender sends to store's subscriptions with client. subject says who
// sends, a mailto: or https address, such as the server's public URL; push
// services contact it when the pushes misbehave.
func NewSender(store *Store, client *http.Client, subject string) *Sender {
	return &Sender{store: store, client: client, subject: subject, now: time.Now}
}

// Send pushes message to sub. A subscription its push service says is gone
// is removed from the Store, and Send returns ErrGone. Any other refusal is
// an error naming the status.
func (s *Sender) Send(ctx context.Context, sub Subscription, message []byte) error {
	body, err := seal(message, sub.Keys)
	if err != nil {
		return err
	}
	authorization, err := s.vapid(sub.Endpoint)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Encoding", "aes128gcm")
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("TTL", fmt.Sprint(int(timeToLive.Seconds())))
	request.Header.Set("Urgency", "high")
	request.Header.Set("Authorization", authorization)
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return s.answered(response, sub.Endpoint)
}

// answered is what the push service's response to a push to endpoint
// means.
func (s *Sender) answered(response *http.Response, endpoint string) error {
	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return nil
	case http.StatusNotFound, http.StatusGone:
		if _, err := s.store.Unsubscribe(endpoint); err != nil {
			return fmt.Errorf("%w, and it can't be removed: %w", ErrGone, err)
		}
		return ErrGone
	}
	reply, _ := io.ReadAll(io.LimitReader(response.Body, errorShown))
	return fmt.Errorf("the push service answered %s: %s", response.Status, strings.TrimSpace(string(reply)))
}

// vapid is the Authorization header that says the push to endpoint comes
// from the holder of the Store's key, per RFC 8292.
func (s *Sender) vapid(endpoint string) (string, error) {
	token, err := s.token(endpoint)
	if err != nil {
		return "", err
	}
	return "vapid t=" + token + ", k=" + s.store.PublicKey(), nil
}

// token is a JWT for endpoint's push service, signed ES256 with the
// Store's key.
func (s *Sender) token(endpoint string) (string, error) {
	address, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	header, _ := json.Marshal(map[string]string{"typ": "JWT", "alg": "ES256"})
	claims, err := json.Marshal(map[string]any{
		"aud": address.Scheme + "://" + address.Host,
		"exp": s.now().Add(tokenLife).Unix(),
		"sub": s.subject,
	})
	if err != nil {
		return "", err
	}
	signed := encode(header) + "." + encode(claims)
	signature, err := sign(s.store.Key(), signed)
	if err != nil {
		return "", err
	}
	return signed + "." + encode(signature), nil
}

// sign is the ES256 signature of text: r and s, 32 bytes each, as JWS
// wants them rather than ASN.1.
func sign(key *ecdsa.PrivateKey, text string) ([]byte, error) {
	digest := sha256.Sum256([]byte(text))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return nil, err
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return signature, nil
}

func encode(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }

// seal encrypts message for the browser holding keys, with a new sender
// key and salt.
func seal(message []byte, keys Keys) ([]byte, error) {
	sender, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return encrypt(message, keys, sender, salt)
}

// encrypt is message encrypted for the browser holding keys, per RFC 8291:
// one aes128gcm record (RFC 8188) behind a header carrying salt and the
// sender's public key.
func encrypt(message []byte, keys Keys, sender *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(message) > maxMessage {
		return nil, fmt.Errorf("%w: %d bytes, and a push holds %d", ErrTooLong, len(message), maxMessage)
	}
	point, err := decodeKey(keys.P256dh)
	if err != nil {
		return nil, fmt.Errorf("keys.p256dh: %w", err)
	}
	auth, err := decodeKey(keys.Auth)
	if err != nil {
		return nil, fmt.Errorf("keys.auth: %w", err)
	}
	receiver, err := ecdh.P256().NewPublicKey(point)
	if err != nil {
		return nil, fmt.Errorf("keys.p256dh: %w", err)
	}
	shared, err := sender.ECDH(receiver)
	if err != nil {
		return nil, err
	}
	gcm, nonce, err := contentKeys(shared, auth, point, sender.PublicKey().Bytes(), salt)
	if err != nil {
		return nil, err
	}
	header := binary.BigEndian.AppendUint32(salt[:16:16], recordSize)
	header = append(header, 65)
	header = append(header, sender.PublicKey().Bytes()...)
	padded := append(append([]byte{}, message...), 2) // 2 ends the last record
	return gcm.Seal(header, nonce, padded, nil), nil
}

// contentKeys are the AES-GCM cipher and nonce for a message, derived from
// the ECDH secret shared by the browser (receiver) and Agentry (sender),
// the browser's auth secret and the message's salt, per RFC 8291 section 3.
func contentKeys(shared, auth, receiver, sender, salt []byte) (cipher.AEAD, []byte, error) {
	info := append([]byte("WebPush: info\x00"), receiver...)
	info = append(info, sender...)
	ikm, err := hkdf.Key(sha256.New, shared, auth, string(info), 32)
	if err != nil {
		return nil, nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	return gcm, nonce, err
}
