package endpoint

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// A form token says "this endpoint handed out this form, for this site, at
// this time". It is signed, so it can't be forged, and checked for age:
// younger than minAge is a script filling the form instantly, older than
// maxAge is a replay of something harvested long ago. Nothing is stored, so
// any number of endpoints behind a load balancer can check each other's
// tokens as long as they share the secret.
const (
	minAge = 3 * time.Second
	maxAge = 24 * time.Hour
)

type tokens struct{ secret []byte }

func (t tokens) sign(host, form string, ts int64) string {
	m := hmac.New(sha256.New, t.secret)
	m.Write([]byte("v1|" + host + "|" + form + "|" + strconv.FormatInt(ts, 10)))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (t tokens) issue(host, form string, now time.Time) string {
	ts := now.Unix()
	return "v1." + strconv.FormatInt(ts, 10) + "." + t.sign(host, form, ts)
}

var (
	errTokenMissing = errors.New("missing")
	errTokenBad     = errors.New("not valid")
	errTokenYoung   = errors.New("sent too quickly")
	errTokenOld     = errors.New("expired")
)

func (t tokens) check(tok, host, form string, now time.Time) error {
	if tok == "" {
		return errTokenMissing
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return errTokenBad
	}
	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return errTokenBad
	}
	if !hmac.Equal([]byte(parts[2]), []byte(t.sign(host, form, ts))) {
		return errTokenBad
	}
	age := now.Sub(time.Unix(ts, 0))
	switch {
	case age < minAge:
		return errTokenYoung
	case age > maxAge:
		return errTokenOld
	}
	return nil
}
