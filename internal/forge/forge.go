// Package forge is what cl-cms knows about the places sites are hosted in
// git: GitHub (and GitHub Enterprise), GitLab, Gitea, Forgejo (Codeberg),
// Gogs, Bitbucket Cloud, and plain git anywhere else.
//
// For each it can verify and read push webhooks, so a push rebuilds at once
// instead of at the next check, and post a commit status, so a pull request
// shows its preview's address. Plain git gets a token-checked hook any
// post-receive script can call; it has no statuses to post.
package forge

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Kind names a platform.
type Kind string

const (
	GitHub    Kind = "github"
	GitLab    Kind = "gitlab"
	Gitea     Kind = "gitea"
	Forgejo   Kind = "forgejo"
	Gogs      Kind = "gogs"
	Bitbucket Kind = "bitbucket"
	Git       Kind = "git"
)

// Kinds lists every platform, for flags and help text.
var Kinds = []Kind{GitHub, GitLab, Gitea, Forgejo, Gogs, Bitbucket, Git}

// Repo is a repository on a platform.
type Repo struct {
	Kind Kind
	Host string // host[:port] of the web/API server
	Path string // owner/name, or group/subgroup/name on GitLab
	API  string // API base URL
}

// TokenUser is the user name each platform expects with a token over HTTPS.
func (k Kind) TokenUser() string {
	switch k {
	case GitHub:
		return "x-access-token"
	case GitLab:
		return "oauth2"
	case Bitbucket:
		return "x-token-auth"
	}
	// Gitea, Forgejo and Gogs accept the token with the account's user name;
	// set CLCMS_GIT_USER to it.
	return "cl-cms"
}

var scpRe = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?([A-Za-z0-9.-]+):(.+)$`)

// Parse works out the platform and repository from a clone URL. kind may be
// empty to guess from well-known hosts; a self-hosted Gitea, GitLab or GitHub
// Enterprise needs it said.
func Parse(cloneURL string, kind Kind) (Repo, error) {
	var host, p string
	if u, err := url.Parse(cloneURL); err == nil && u.Scheme != "" && u.Host != "" {
		host, p = u.Host, u.Path
		if u.Scheme == "ssh" {
			host = u.Hostname() // an SSH port is not the web server's port
		}
	} else if m := scpRe.FindStringSubmatch(cloneURL); m != nil && !strings.HasPrefix(cloneURL, "/") {
		host, p = m[1], m[2]
	} else {
		// A local path or file:// URL: plain git, nothing to talk to.
		return Repo{Kind: Git}, nil
	}
	p = strings.TrimSuffix(strings.Trim(p, "/"), ".git")
	if kind == "" {
		switch strings.ToLower(host) {
		case "github.com":
			kind = GitHub
		case "gitlab.com":
			kind = GitLab
		case "codeberg.org":
			kind = Forgejo
		case "gitea.com":
			kind = Gitea
		case "bitbucket.org":
			kind = Bitbucket
		default:
			kind = Git
		}
	}
	r := Repo{Kind: kind, Host: host, Path: p}
	switch kind {
	case GitHub:
		if strings.EqualFold(host, "github.com") {
			r.API = "https://api.github.com"
		} else {
			r.API = "https://" + host + "/api/v3"
		}
	case GitLab:
		r.API = "https://" + host + "/api/v4"
	case Gitea, Forgejo, Gogs:
		r.API = "https://" + host + "/api/v1"
	case Bitbucket:
		r.API = "https://api.bitbucket.org/2.0"
	case Git:
	default:
		return Repo{}, fmt.Errorf("unknown platform %q (one of %v)", kind, Kinds)
	}
	if kind != Git && strings.Count(p, "/") < 1 {
		return Repo{}, fmt.Errorf("%s doesn't look like owner/repository", cloneURL)
	}
	return r, nil
}
