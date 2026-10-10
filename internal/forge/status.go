package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// State is a build's outcome, mapped onto each platform's own words.
type State string

const (
	Pending State = "pending"
	Success State = "success"
	Failure State = "failure"
)

// Status is what a pull request shows for a commit.
type Status struct {
	State       State
	URL         string // the preview
	Description string
	Context     string // the check's name; "cl-cms/preview" by default
}

// Client posts commit statuses with a platform token.
type Client struct {
	Repo  Repo
	Token string
	HTTP  *http.Client
}

// Supported reports whether the platform has commit statuses at all.
func (c *Client) Supported() bool {
	return c.Repo.Kind != Git && c.Repo.Kind != Gogs && c.Token != ""
}

// SetStatus posts a status on a commit. On platforms without statuses, or
// with no token, it does nothing.
func (c *Client) SetStatus(ctx context.Context, commit string, s Status) error {
	if !c.Supported() {
		return nil
	}
	if s.Context == "" {
		s.Context = "cl-cms/preview"
	}
	if len(s.Description) > 140 {
		s.Description = s.Description[:137] + "..."
	}
	var (
		method   = http.MethodPost
		endpoint string
		body     any
		auth     string
	)
	r := c.Repo
	switch r.Kind {
	case GitHub, Gitea, Forgejo:
		state := string(s.State)
		endpoint = fmt.Sprintf("%s/repos/%s/statuses/%s", r.API, r.Path, url.PathEscape(commit))
		body = map[string]string{"state": state, "target_url": s.URL, "description": s.Description, "context": s.Context}
		auth = "Bearer " + c.Token
		if r.Kind != GitHub {
			auth = "token " + c.Token
		}
	case GitLab:
		state := map[State]string{Pending: "running", Success: "success", Failure: "failed"}[s.State]
		q := url.Values{"state": {state}, "name": {s.Context}, "description": {s.Description}}
		if s.URL != "" {
			q.Set("target_url", s.URL)
		}
		endpoint = fmt.Sprintf("%s/projects/%s/statuses/%s?%s", r.API, url.PathEscape(r.Path), url.PathEscape(commit), q.Encode())
		auth = "PRIVATE-TOKEN " + c.Token
	case Bitbucket:
		state := map[State]string{Pending: "INPROGRESS", Success: "SUCCESSFUL", Failure: "FAILED"}[s.State]
		endpoint = fmt.Sprintf("%s/repositories/%s/commit/%s/statuses/build", r.API, r.Path, url.PathEscape(commit))
		body = map[string]string{"state": state, "key": s.Context, "name": s.Context, "url": s.URL, "description": s.Description}
		auth = "Bearer " + c.Token
	default:
		return nil
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, rd)
	if err != nil {
		return err
	}
	if name, value, ok := strings.Cut(auth, " "); ok && name == "PRIVATE-TOKEN" {
		req.Header.Set("PRIVATE-TOKEN", value)
	} else {
		req.Header.Set("Authorization", auth)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cl-cms")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("%s status: %s: %s", r.Kind, res.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}
