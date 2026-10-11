// Package gitx runs git for hotdog-cms: cloning and fetching sites from any
// repository git can reach, plain or hosted, with optional token auth for
// private HTTPS repositories.
//
// A token is handed to git through GIT_CONFIG_* environment variables, never
// on the command line, so it doesn't show up in the process list.
package gitx

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Auth is how git authenticates over HTTPS. Empty means git's own setup:
// SSH keys and agent, credential helpers, .netrc.
type Auth struct {
	User  string
	Token string
}

// FromEnv reads HOTDOG_GIT_TOKEN and HOTDOG_GIT_USER. defaultUser is the
// platform's convention for token logins (x-access-token on GitHub, oauth2 on
// GitLab, ...).
func FromEnv(defaultUser string) Auth {
	a := Auth{User: os.Getenv("HOTDOG_GIT_USER"), Token: os.Getenv("HOTDOG_GIT_TOKEN")}
	if a.User == "" {
		a.User = defaultUser
	}
	return a
}

// Available reports whether git is installed.
func Available() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is not installed")
	}
	return nil
}

// Run runs git in dir (or the current folder if dir is empty).
func Run(auth Auth, dir string, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	var out, errb bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if auth.Token != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(auth.User + ":" + auth.Token))
		cmd.Env = append(cmd.Env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraHeader",
			"GIT_CONFIG_VALUE_0=Authorization: Basic "+cred)
	}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if auth.Token != "" {
			msg = strings.ReplaceAll(msg, auth.Token, "***")
		}
		return "", fmt.Errorf("git %s: %v: %s", firstVerb(args), err, msg)
	}
	return strings.TrimSpace(out.String()), nil
}

func firstVerb(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-C" || args[i] == "-c" {
			i++
			continue
		}
		return args[i]
	}
	return ""
}

// ValidBranch says whether name is a branch name hotdog-cms will pass to
// git: what `git check-ref-format --branch` accepts, minus anything that
// could be read as an option or a revision expression.
func ValidBranch(name string) bool {
	if name == "" || len(name) > 200 || strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") ||
		strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".lock") || strings.Contains(name, "..") || strings.Contains(name, "//") ||
		strings.Contains(name, "@{") || name == "@" {
		return false
	}
	for _, r := range name {
		if r <= ' ' || r == 0x7f || strings.ContainsRune("~^:?*[\\", r) {
			return false
		}
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") || strings.HasPrefix(part, "-") {
			return false
		}
	}
	return true
}

// safeConfig is set on every clone hotdog-cms makes. Symbolic links in a
// repository are checked out as small files holding the link's target,
// never as links: a link committed to a site (static/x -> /etc/passwd)
// must not let anyone read or write outside the checkout through it.
var safeConfig = []string{"-c", "core.symlinks=false", "-c", "protocol.ext.allow=never"}

// Checkout makes dir a shallow clone of one branch at its tip: cloning the
// first time, fetching and resetting after. The folder belongs to hotdog-cms, so
// anything in it that isn't on the branch is thrown away.
func Checkout(auth Auth, repo, branch, dir string) (string, error) {
	if !ValidBranch(branch) {
		return "", fmt.Errorf("%q isn't a branch name hotdog-cms uses", branch)
	}
	if _, err := os.Stat(dir + "/.git"); err != nil {
		args := append(append([]string{}, safeConfig...), "clone", "--quiet", "--depth", "1", "--branch", branch, "--single-branch", "--config", "core.symlinks=false", "--end-of-options", repo, dir)
		if _, err := Run(auth, "", args...); err != nil {
			return "", err
		}
	} else {
		if _, err := Run(auth, dir, "config", "core.symlinks", "false"); err != nil {
			return "", err
		}
		if _, err := Run(auth, dir, "fetch", "--quiet", "--depth", "1", "--end-of-options", "origin", "+refs/heads/"+branch); err != nil {
			return "", err
		}
		if _, err := Run(auth, dir, "reset", "--quiet", "--hard", "FETCH_HEAD"); err != nil {
			return "", err
		}
		if _, err := Run(auth, dir, "clean", "-qfdx"); err != nil {
			return "", err
		}
	}
	return Run(auth, dir, "rev-parse", "HEAD")
}

// Heads lists a repository's branches and their commits.
func Heads(auth Auth, repo string) (map[string]string, error) {
	out, err := Run(auth, "", "ls-remote", "--heads", repo)
	if err != nil {
		return nil, err
	}
	heads := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.HasPrefix(f[1], "refs/heads/") {
			heads[strings.TrimPrefix(f[1], "refs/heads/")] = f[0]
		}
	}
	return heads, nil
}
