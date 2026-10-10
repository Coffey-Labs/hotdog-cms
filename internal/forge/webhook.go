package forge

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Push is a branch moving (or being deleted).
type Push struct {
	Branch  string
	Commit  string
	Deleted bool
}

// ErrNotPush means the delivery was valid but not a push (a ping, a tag, an
// issue event): acknowledge it and do nothing.
var ErrNotPush = errors.New("not a branch push")

func hmacHex(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// Verify checks that a delivery was signed with the shared secret, the way
// the platform signs them. A webhook without a secret is refused outright:
// anyone could otherwise make cl-cms rebuild on demand.
func Verify(kind Kind, h http.Header, body []byte, secret string) error {
	if secret == "" {
		return errors.New("no webhook secret configured")
	}
	want := hmacHex(secret, body)
	sig := func(name, prefix string) (bool, bool) {
		v := h.Get(name)
		if v == "" {
			return false, false
		}
		return true, equal(strings.TrimPrefix(v, prefix), want)
	}
	var checks [][2]string
	switch kind {
	case GitHub:
		checks = [][2]string{{"X-Hub-Signature-256", "sha256="}}
	case Gitea, Forgejo, Gogs:
		checks = [][2]string{{"X-Forgejo-Signature", ""}, {"X-Gitea-Signature", ""}, {"X-Gogs-Signature", ""}, {"X-Hub-Signature-256", "sha256="}}
	case Bitbucket:
		checks = [][2]string{{"X-Hub-Signature", "sha256="}}
	case GitLab:
		if t := h.Get("X-Gitlab-Token"); t != "" && equal(t, secret) {
			return nil
		}
		return errors.New("X-Gitlab-Token missing or wrong")
	case Git:
		if t := h.Get("X-CLCMS-Token"); t != "" {
			if equal(t, secret) {
				return nil
			}
			return errors.New("X-CLCMS-Token is wrong")
		}
		checks = [][2]string{{"X-Hub-Signature-256", "sha256="}}
	}
	for _, c := range checks {
		if present, ok := sig(c[0], c[1]); present {
			if ok {
				return nil
			}
			return fmt.Errorf("%s does not match", c[0])
		}
	}
	return errors.New("delivery is not signed")
}

var zeros = strings.Repeat("0", 40)

func branchOf(ref string) (string, bool) {
	return strings.CutPrefix(ref, "refs/heads/")
}

// ParsePush reads a push delivery. It returns ErrNotPush for anything else.
func ParsePush(kind Kind, h http.Header, body []byte) (Push, error) {
	switch kind {
	case GitHub, Gitea, Forgejo, Gogs:
		ev := h.Get("X-GitHub-Event")
		for _, n := range []string{"X-Forgejo-Event", "X-Gitea-Event", "X-Gogs-Event"} {
			if v := h.Get(n); v != "" {
				ev = v
			}
		}
		if ev != "push" {
			return Push{}, ErrNotPush
		}
		return refPush(body)
	case GitLab:
		if h.Get("X-Gitlab-Event") != "Push Hook" {
			return Push{}, ErrNotPush
		}
		return refPush(body)
	case Bitbucket:
		if h.Get("X-Event-Key") != "repo:push" {
			return Push{}, ErrNotPush
		}
		var d struct {
			Push struct {
				Changes []struct {
					New *struct {
						Type, Name string
						Target     struct{ Hash string }
					}
					Old *struct{ Type, Name string }
				}
			}
		}
		if err := json.Unmarshal(body, &d); err != nil {
			return Push{}, err
		}
		for _, c := range d.Push.Changes {
			if c.New != nil && c.New.Type == "branch" {
				return Push{Branch: c.New.Name, Commit: c.New.Target.Hash}, nil
			}
			if c.New == nil && c.Old != nil && c.Old.Type == "branch" {
				return Push{Branch: c.Old.Name, Deleted: true}, nil
			}
		}
		return Push{}, ErrNotPush
	case Git:
		// JSON {"ref":..,"after":..} or a form ref=..&after=.. from a
		// post-receive script.
		if strings.HasPrefix(h.Get("Content-Type"), "application/json") {
			return refPush(body)
		}
		v, err := url.ParseQuery(string(body))
		if err != nil {
			return Push{}, err
		}
		b, _ := json.Marshal(map[string]string{"ref": v.Get("ref"), "after": v.Get("after")})
		return refPush(b)
	}
	return Push{}, ErrNotPush
}

func refPush(body []byte) (Push, error) {
	var d struct {
		Ref     string `json:"ref"`
		After   string `json:"after"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return Push{}, err
	}
	b, ok := branchOf(d.Ref)
	if !ok {
		return Push{}, ErrNotPush // a tag
	}
	return Push{Branch: b, Commit: d.After, Deleted: d.Deleted || d.After == zeros}, nil
}
