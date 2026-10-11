package editor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forge"
)

// Finding sites. With discover on for a platform, the editor looks through
// the repositories each signed-in account can reach and offers every one
// with a site.yaml at the top, beside the sites listed in editor.yaml. It
// reads only what the account could read anyway, and an operator who wants
// the editor limited to the sites they list leaves it off.

// DiscoverConfig turns finding sites on for a platform: `discover: true`, or
// a map to narrow it.
type DiscoverConfig struct {
	Owners []string `yaml:"owners"` // only these users' and organizations' repositories; empty means any
	Limit  int      `yaml:"limit"`  // most recently updated repositories looked at per account; default 100
}

// UnmarshalYAML takes `discover: true` as well as the map form.
func (d *DiscoverConfig) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		var on bool
		if err := n.Decode(&on); err != nil {
			return fmt.Errorf("discover: true, false, or a map with owners and limit")
		}
		if !on {
			*d = DiscoverConfig{Limit: -1}
		}
		return nil
	}
	type plain DiscoverConfig
	return n.Decode((*plain)(d))
}

func (d *DiscoverConfig) on() bool { return d != nil && d.Limit >= 0 }

func (d *DiscoverConfig) limit() int {
	if d.Limit <= 0 {
		return 100
	}
	return min(d.Limit, 1000)
}

const discoverTTL = 10 * time.Minute

type discovery struct {
	sites []*SiteConfig
	at    time.Time
}

type discoverCache struct {
	mu   sync.Mutex
	seen map[string]discovery
}

// platformRepo is what Gitea, Forgejo and GitHub say about a repository.
type platformRepo struct {
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Archived      bool   `json:"archived"`
	Empty         bool   `json:"empty"` // Gitea
	Size          int    `json:"size"`  // GitHub: 0 when empty
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
	Permissions struct{ Pull, Push, Admin bool } `json:"permissions"`
}

// discovered returns the sites an account's repositories hold, from the
// cache when it's fresh.
func (s *Server) discovered(ctx context.Context, sess *Session) ([]*SiteConfig, error) {
	f := s.cfg.forge(sess.Forge)
	if f == nil || !f.Discover.on() || s.apiBase(f) == "" {
		return nil, nil
	}
	sum := sha256.Sum256([]byte(sess.Token))
	key := hex.EncodeToString(sum[:])
	s.found.mu.Lock()
	if d, ok := s.found.seen[key]; ok && time.Since(d.at) < discoverTTL {
		s.found.mu.Unlock()
		return d.sites, nil
	}
	s.found.mu.Unlock()

	repos, err := s.listRepos(ctx, f, sess.Token)
	if err != nil {
		return nil, err
	}
	owners := map[string]bool{}
	for _, o := range f.Discover.Owners {
		owners[strings.ToLower(o)] = true
	}
	var candidates []platformRepo
	for _, r := range repos {
		u, err := url.Parse(r.CloneURL)
		switch {
		case r.Archived, r.Empty, f.Kind == forge.GitHub && r.Size == 0, !r.Permissions.Pull, r.DefaultBranch == "":
		case err != nil || !strings.EqualFold(u.Hostname(), f.Host): // only clone from the platform itself
		case len(owners) > 0 && !owners[strings.ToLower(r.Owner.Login)]:
		default:
			candidates = append(candidates, r)
		}
	}

	// Look for site.yaml in each, a few at a time.
	type hit struct {
		repo platformRepo
		name string
	}
	hits := make([]*hit, len(candidates))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, r := range candidates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if name, ok := s.siteYAML(ctx, f, sess.Token, r); ok {
				hits[i] = &hit{r, name}
			}
		}()
	}
	wg.Wait()

	var sites []*SiteConfig
	for _, h := range hits {
		if h == nil {
			continue
		}
		name := h.name
		if name == "" {
			name = h.repo.FullName
		}
		st := &SiteConfig{ID: slug(h.repo.FullName), Name: name, Repo: h.repo.CloneURL, Branch: h.repo.DefaultBranch, discovered: true}
		sites = append(sites, st)
		// The listing already says what this account may do here.
		s.rememberAccess(sess.Token, st, Access{Read: h.repo.Permissions.Pull, Write: h.repo.Permissions.Push, Admin: h.repo.Permissions.Admin})
	}
	sort.Slice(sites, func(i, j int) bool { return strings.ToLower(sites[i].Name) < strings.ToLower(sites[j].Name) })

	s.found.mu.Lock()
	if s.found.seen == nil || len(s.found.seen) > 1024 {
		s.found.seen = map[string]discovery{}
	}
	s.found.seen[key] = discovery{sites: sites, at: time.Now()}
	s.found.mu.Unlock()
	return sites, nil
}

// listRepos lists the repositories an account can reach, most recently
// updated first where the platform sorts, up to the configured limit.
func (s *Server) listRepos(ctx context.Context, f *ForgeConfig, token string) ([]platformRepo, error) {
	base := s.apiBase(f)
	limit := f.Discover.limit()
	var out []platformRepo
	for page := 1; len(out) < limit && page <= 20; page++ {
		var u string
		per := 50
		if f.Kind == forge.GitHub {
			per = 100
			u = fmt.Sprintf("%s/user/repos?per_page=%d&page=%d&sort=updated&affiliation=owner,collaborator,organization_member", base, per, page)
		} else {
			u = fmt.Sprintf("%s/user/repos?limit=%d&page=%d", base, per, page)
		}
		var batch []platformRepo
		if err := s.api(ctx, f, token, u, &batch); err != nil {
			return nil, err
		}
		out = append(out, batch...)
		if len(batch) < per {
			break
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// siteYAML reports whether a repository has a HotDog CMS site.yaml at the
// top of its default branch, and the site's name from it.
func (s *Server) siteYAML(ctx context.Context, f *ForgeConfig, token string, r platformRepo) (string, bool) {
	base := s.apiBase(f)
	var u, accept string
	if f.Kind == forge.GitHub {
		u = fmt.Sprintf("%s/repos/%s/contents/site.yaml?ref=%s", base, r.FullName, url.QueryEscape(r.DefaultBranch))
		accept = "application/vnd.github.raw+json"
	} else {
		u = fmt.Sprintf("%s/repos/%s/raw/site.yaml?ref=%s", base, r.FullName, url.QueryEscape(r.DefaultBranch))
		accept = "text/plain"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "hotdog-cms-editor")
	res, err := s.http.Do(req)
	if err != nil {
		return "", false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", false
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 256<<10))
	if err != nil {
		return "", false
	}
	// Other tools use a file of that name too: it has to read like ours.
	var cfg struct {
		Name string `yaml:"name"`
		URL  string `yaml:"url"`
	}
	if yaml.Unmarshal(raw, &cfg) != nil || cfg.URL == "" {
		return "", false
	}
	return strings.TrimSpace(cfg.Name), true
}

// allSites is every site the signed-in accounts can be offered: those in
// editor.yaml, then those found in the accounts' repositories. A repository
// listed in editor.yaml is never offered twice, and every site keeps an id
// no other has.
func (s *Server) allSites(ctx context.Context, accts []*Session) []*SiteConfig {
	var out []*SiteConfig
	repos := map[string]bool{}
	ids := map[string]bool{}
	for i := range s.cfg.Sites {
		st := &s.cfg.Sites[i]
		out = append(out, st)
		repos[repoKey(st.Repo)] = true
		ids[st.ID] = true
	}
	for _, a := range accts {
		found, err := s.discovered(ctx, a)
		if err != nil {
			continue // a signed-out account is dealt with when it's used
		}
		for _, st := range found {
			if repos[repoKey(st.Repo)] {
				continue
			}
			repos[repoKey(st.Repo)] = true
			c := *st
			if ids[c.ID] {
				u, _ := url.Parse(c.Repo)
				c.ID = slug(c.ID + "-" + u.Hostname())
			}
			for n := 2; ids[c.ID]; n++ {
				c.ID = fmt.Sprintf("%s-%d", slug(st.ID), n)
			}
			ids[c.ID] = true
			out = append(out, &c)
		}
	}
	return out
}

func repoKey(repo string) string {
	return strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(repo), "/"), ".git")
}

// findSite looks a site up by id among everything the accounts are offered.
func (s *Server) findSite(ctx context.Context, accts []*Session, id string) *SiteConfig {
	if st := s.cfg.site(id); st != nil {
		return st
	}
	for _, st := range s.allSites(ctx, accts) {
		if st.ID == id {
			return st
		}
	}
	return nil
}
