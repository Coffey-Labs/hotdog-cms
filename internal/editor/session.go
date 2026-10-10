package editor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// Sessions live in the browser, sealed: AES-GCM-encrypted cookies holding
// who signed in, on which platform, and their token for it. Nothing is kept
// server-side, so there's nothing to leak from the editor's disk and nothing
// to clean up; a session ends when the cookie expires or the person signs
// out. The key comes from HOTDOG_EDITOR_SECRET, shared by every editor
// instance that serves the same people.
//
// A person can be signed in to several accounts at once, on one platform or
// several: each account is its own cookie (hotdog_a0 … hotdog_a7), since one
// platform token alone can fill much of a cookie.

const (
	accountPrefix = "hotdog_a"
	maxAccounts   = 8
	stateCookie   = "hotdog_oauth"
	sessionTTL    = 8 * time.Hour
)

func accountCookie(slot int) string { return accountPrefix + strconv.Itoa(slot) }

// Session is one signed-in account.
type Session struct {
	Forge   string    `json:"f"` // platform host
	Login   string    `json:"l"`
	Name    string    `json:"n"`
	Email   string    `json:"m"` // commits are attributed to this address
	Token   string    `json:"t"`
	Expires time.Time `json:"e"`
	slot    int       // which cookie holds it
}

// ID names the account in the interface and in requests: platform/login.
func (s *Session) ID() string { return s.Forge + "/" + s.Login }

type sealer struct{ aead cipher.AEAD }

func newSealer(secret string) (*sealer, error) {
	if len(secret) < 32 {
		return nil, errors.New("HOTDOG_EDITOR_SECRET must be set, at least 32 characters (openssl rand -hex 32)")
	}
	key := sha256.Sum256([]byte("hotdog-cms editor session v1\x00" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &sealer{aead: aead}, nil
}

// seal encrypts v for a cookie named name; the name is bound into the
// ciphertext, so one cookie can't be replayed as another.
func (s *sealer) seal(name string, v any) (string, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(s.aead.Seal(nonce, nonce, plain, []byte(name))), nil
}

func (s *sealer) open(name, value string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < s.aead.NonceSize() {
		return errors.New("bad cookie")
	}
	plain, err := s.aead.Open(nil, raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():], []byte(name))
	if err != nil {
		return errors.New("bad cookie")
	}
	return json.Unmarshal(plain, v)
}

func (s *sealer) setCookie(w http.ResponseWriter, r *http.Request, name string, v any, ttl time.Duration) error {
	val, err := s.seal(name, v)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: val, Path: "/", MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// accounts returns every account the browser is signed in to, in slot order.
func (s *sealer) accounts(r *http.Request) []*Session {
	var out []*Session
	for i := 0; i < maxAccounts; i++ {
		c, err := r.Cookie(accountCookie(i))
		if err != nil {
			continue
		}
		var sess Session
		if s.open(accountCookie(i), c.Value, &sess) != nil || time.Now().After(sess.Expires) || sess.Token == "" {
			continue
		}
		sess.slot = i
		out = append(out, &sess)
	}
	return out
}
