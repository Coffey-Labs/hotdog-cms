package forge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		url  string
		kind Kind
		want Repo
	}{
		{"https://github.com/acme/site.git", "", Repo{GitHub, "github.com", "acme/site", "https://api.github.com"}},
		{"git@github.com:acme/site.git", "", Repo{GitHub, "github.com", "acme/site", "https://api.github.com"}},
		{"https://gitlab.com/group/sub/site", "", Repo{GitLab, "gitlab.com", "group/sub/site", "https://gitlab.com/api/v4"}},
		{"https://codeberg.org/me/site.git", "", Repo{Forgejo, "codeberg.org", "me/site", "https://codeberg.org/api/v1"}},
		{"ssh://git@git.example.org:2222/me/site.git", Gitea, Repo{Gitea, "git.example.org", "me/site", "https://git.example.org/api/v1"}},
		{"https://ghe.example.com/team/site", GitHub, Repo{GitHub, "ghe.example.com", "team/site", "https://ghe.example.com/api/v3"}},
		{"git@bitbucket.org:ws/site.git", "", Repo{Bitbucket, "bitbucket.org", "ws/site", "https://api.bitbucket.org/2.0"}},
		{"https://git.example.org/me/site.git", "", Repo{Git, "git.example.org", "me/site", ""}},
		{"/srv/git/site.git", "", Repo{Kind: Git}},
	}
	for _, c := range cases {
		got, err := Parse(c.url, c.kind)
		if err != nil {
			t.Errorf("%s: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.url, got, c.want)
		}
	}
	if _, err := Parse("https://example.org/x", "svn"); err == nil {
		t.Error("accepted an unknown platform")
	}
}

const secret = "s3cret-for-tests"

func TestVerify(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	sig := hmacHex(secret, body)
	good := map[Kind]http.Header{
		GitHub:    {"X-Hub-Signature-256": {"sha256=" + sig}},
		Gitea:     {"X-Gitea-Signature": {sig}},
		Forgejo:   {"X-Forgejo-Signature": {sig}},
		Gogs:      {"X-Gogs-Signature": {sig}},
		Bitbucket: {"X-Hub-Signature": {"sha256=" + sig}},
		GitLab:    {"X-Gitlab-Token": {secret}},
		Git:       {"X-Clcms-Token": {secret}},
	}
	for kind, h := range good {
		if err := Verify(kind, h, body, secret); err != nil {
			t.Errorf("%s: good delivery refused: %v", kind, err)
		}
		if err := Verify(kind, h, append(body, ' '), secret); err == nil && kind != GitLab && kind != Git {
			t.Errorf("%s: tampered body accepted", kind)
		}
		if err := Verify(kind, http.Header{}, body, secret); err == nil {
			t.Errorf("%s: unsigned delivery accepted", kind)
		}
		if err := Verify(kind, h, body, ""); err == nil {
			t.Errorf("%s: accepted with no secret configured", kind)
		}
	}
	if err := Verify(GitLab, http.Header{"X-Gitlab-Token": {"wrong"}}, body, secret); err == nil {
		t.Error("gitlab: wrong token accepted")
	}
}

func TestParsePush(t *testing.T) {
	sha := strings.Repeat("a", 40)
	cases := []struct {
		kind Kind
		h    http.Header
		body string
		want Push
		err  error
	}{
		{GitHub, http.Header{"X-Github-Event": {"push"}}, `{"ref":"refs/heads/draft/x","after":"` + sha + `"}`, Push{"draft/x", sha, false}, nil},
		{GitHub, http.Header{"X-Github-Event": {"push"}}, `{"ref":"refs/heads/old","after":"` + zeros + `","deleted":true}`, Push{"old", zeros, true}, nil},
		{GitHub, http.Header{"X-Github-Event": {"ping"}}, `{}`, Push{}, ErrNotPush},
		{GitHub, http.Header{"X-Github-Event": {"push"}}, `{"ref":"refs/tags/v1","after":"` + sha + `"}`, Push{}, ErrNotPush},
		{Gitea, http.Header{"X-Gitea-Event": {"push"}}, `{"ref":"refs/heads/main","after":"` + sha + `"}`, Push{"main", sha, false}, nil},
		{Forgejo, http.Header{"X-Forgejo-Event": {"push"}}, `{"ref":"refs/heads/main","after":"` + sha + `"}`, Push{"main", sha, false}, nil},
		{GitLab, http.Header{"X-Gitlab-Event": {"Push Hook"}}, `{"ref":"refs/heads/main","after":"` + sha + `"}`, Push{"main", sha, false}, nil},
		{Bitbucket, http.Header{"X-Event-Key": {"repo:push"}}, `{"push":{"changes":[{"new":{"type":"branch","name":"main","target":{"hash":"` + sha + `"}}}]}}`, Push{"main", sha, false}, nil},
		{Bitbucket, http.Header{"X-Event-Key": {"repo:push"}}, `{"push":{"changes":[{"new":null,"old":{"type":"branch","name":"gone"}}]}}`, Push{"gone", "", true}, nil},
		{Git, http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, "ref=refs%2Fheads%2Fmain&after=" + sha, Push{"main", sha, false}, nil},
	}
	for i, c := range cases {
		got, err := ParsePush(c.kind, c.h, []byte(c.body))
		if err != c.err || got != c.want {
			t.Errorf("case %d (%s): got %+v, %v; want %+v, %v", i, c.kind, got, err, c.want, c.err)
		}
	}
}

func TestSetStatus(t *testing.T) {
	type seen struct {
		method, path, query, auth, private string
		body                               map[string]string
	}
	var last seen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		last = seen{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("PRIVATE-TOKEN"), nil}
		if len(b) > 0 {
			_ = json.Unmarshal(b, &last.body)
		}
		w.WriteHeader(201)
	}))
	defer srv.Close()
	st := Status{State: Success, URL: "https://main-abc.preview.test/", Description: "Preview ready"}
	cases := []struct {
		repo  Repo
		check func() string
	}{
		{Repo{GitHub, "", "acme/site", srv.URL}, func() string {
			if last.path != "/repos/acme/site/statuses/abc123" || last.auth != "Bearer tok" || last.body["state"] != "success" || last.body["target_url"] != st.URL {
				return "github request wrong"
			}
			return ""
		}},
		{Repo{Gitea, "", "me/site", srv.URL}, func() string {
			if last.path != "/repos/me/site/statuses/abc123" || last.auth != "token tok" || last.body["context"] != "cl-cms/preview" {
				return "gitea request wrong"
			}
			return ""
		}},
		{Repo{GitLab, "", "group/sub/site", srv.URL}, func() string {
			if last.path != "/projects/group%2Fsub%2Fsite/statuses/abc123" || last.private != "tok" || !strings.Contains(last.query, "state=success") {
				return "gitlab request wrong: " + last.path + "?" + last.query
			}
			return ""
		}},
		{Repo{Bitbucket, "", "ws/site", srv.URL}, func() string {
			if last.path != "/repositories/ws/site/commit/abc123/statuses/build" || last.body["state"] != "SUCCESSFUL" {
				return "bitbucket request wrong"
			}
			return ""
		}},
	}
	for _, c := range cases {
		cl := &Client{Repo: c.repo, Token: "tok"}
		if err := cl.SetStatus(context.Background(), "abc123", st); err != nil {
			t.Errorf("%s: %v", c.repo.Kind, err)
			continue
		}
		if msg := c.check(); msg != "" {
			t.Errorf("%s: %s (%+v)", c.repo.Kind, msg, last)
		}
	}
	last = seen{}
	for _, k := range []Kind{Git, Gogs} {
		if err := (&Client{Repo: Repo{Kind: k, API: srv.URL}, Token: "tok"}).SetStatus(context.Background(), "abc", st); err != nil || last.method != "" {
			t.Errorf("%s: posted a status on a platform without them", k)
		}
	}
}
