package editor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/forge"
)

// What the editor reads about a repository from its platform, as the
// signed-in person. Gitea, Forgejo and GitHub share one API shape. GitLab and
// Bitbucket sign people in already; reading their branches and reviews comes
// with a later milestone.

var errNotYet = errors.New("this platform's branches and reviews aren't read by the editor yet")

// Access is what the person may do in a repository.
type Access struct {
	Read, Write, Admin bool
}

// Branch is one branch and its tip.
type Branch struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

// Review is an open pull request.
type Review struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Branch string `json:"branch"`
	Author string `json:"author"`
	URL    string `json:"url"`
}

func (s *Server) apiBase(f *ForgeConfig) string {
	switch f.Kind {
	case forge.GitHub:
		if strings.EqualFold(f.Host, "github.com") {
			return "https://api.github.com"
		}
		return f.base() + "/api/v3"
	case forge.Gitea, forge.Forgejo:
		return f.base() + "/api/v1"
	}
	return ""
}

func repoPath(site *SiteConfig) (string, error) {
	r, err := forge.Parse(site.Repo, "")
	if err != nil {
		return "", err
	}
	if r.Path == "" {
		u, _ := url.Parse(site.Repo)
		return strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), nil
	}
	return r.Path, nil
}

func (s *Server) access(ctx context.Context, f *ForgeConfig, token string, site *SiteConfig) (Access, error) {
	base := s.apiBase(f)
	if base == "" {
		return Access{Read: true}, nil // checked by cloning instead
	}
	p, err := repoPath(site)
	if err != nil {
		return Access{}, err
	}
	var d struct {
		Permissions struct{ Admin, Push, Pull bool } `json:"permissions"`
	}
	if err := s.api(ctx, f, token, base+"/repos/"+p, &d); err != nil {
		if errors.Is(err, errSignedOut) {
			return Access{}, err
		}
		return Access{}, nil // not found or not allowed: no access
	}
	return Access{Read: d.Permissions.Pull, Write: d.Permissions.Push, Admin: d.Permissions.Admin}, nil
}

func (s *Server) branches(ctx context.Context, f *ForgeConfig, token string, site *SiteConfig) ([]Branch, error) {
	base := s.apiBase(f)
	if base == "" {
		return nil, errNotYet
	}
	p, err := repoPath(site)
	if err != nil {
		return nil, err
	}
	var out []Branch
	for page := 1; page <= 10; page++ {
		var d []struct {
			Name   string `json:"name"`
			Commit struct {
				ID  string `json:"id"`
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		if err := s.api(ctx, f, token, fmt.Sprintf("%s/repos/%s/branches?limit=50&per_page=50&page=%d", base, p, page), &d); err != nil {
			return nil, err
		}
		for _, b := range d {
			out = append(out, Branch{Name: b.Name, Commit: firstNonEmpty(b.Commit.ID, b.Commit.SHA)})
		}
		if len(d) < 50 {
			break
		}
	}
	return out, nil
}

func (s *Server) reviews(ctx context.Context, f *ForgeConfig, token string, site *SiteConfig) ([]Review, error) {
	base := s.apiBase(f)
	if base == "" {
		return nil, errNotYet
	}
	p, err := repoPath(site)
	if err != nil {
		return nil, err
	}
	var d []struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := s.api(ctx, f, token, fmt.Sprintf("%s/repos/%s/pulls?state=open&limit=50&per_page=50", base, p), &d); err != nil {
		return nil, err
	}
	out := make([]Review, 0, len(d))
	for _, pr := range d {
		out = append(out, Review{Number: pr.Number, Title: pr.Title, Branch: pr.Head.Ref, Author: pr.User.Login, URL: pr.HTMLURL})
	}
	return out, nil
}

// send makes a write call to the platform API as the signed-in person.
func (s *Server) send(ctx context.Context, f *ForgeConfig, token, method, u string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cl-cms-editor")
	res, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errSignedOut
	}
	if res.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&e)
		if e.Message == "" {
			e.Message = res.Status
		}
		return &platformError{Status: res.StatusCode, Message: e.Message}
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out)
	}
	return nil
}

type platformError struct {
	Status  int
	Message string
}

func (e *platformError) Error() string { return fmt.Sprintf("the platform said: %s", e.Message) }

// openReview opens a pull request from branch into the site's publishing branch.
func (s *Server) openReview(ctx context.Context, f *ForgeConfig, token string, site *SiteConfig, branch, title, body string) (Review, error) {
	base := s.apiBase(f)
	if base == "" {
		return Review{}, errNotYet
	}
	p, err := repoPath(site)
	if err != nil {
		return Review{}, err
	}
	var d struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	if err := s.send(ctx, f, token, http.MethodPost, base+"/repos/"+p+"/pulls", map[string]string{"title": title, "head": branch, "base": site.Branch, "body": body}, &d); err != nil {
		return Review{}, err
	}
	return Review{Number: d.Number, Title: title, Branch: branch, URL: d.HTMLURL}, nil
}

// merge merges a pull request. The platform decides whether this person may.
func (s *Server) merge(ctx context.Context, f *ForgeConfig, token string, site *SiteConfig, number int) error {
	base := s.apiBase(f)
	if base == "" {
		return errNotYet
	}
	p, err := repoPath(site)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/repos/%s/pulls/%d/merge", base, p, number)
	if f.Kind == forge.GitHub {
		return s.send(ctx, f, token, http.MethodPut, u, map[string]string{"merge_method": "merge"}, nil)
	}
	return s.send(ctx, f, token, http.MethodPost, u, map[string]any{"Do": "merge", "delete_branch_after_merge": true}, nil)
}
