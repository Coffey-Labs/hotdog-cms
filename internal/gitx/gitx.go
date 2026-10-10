// Package gitx runs git for cl-cms: cloning and fetching sites from any
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

// FromEnv reads CLCMS_GIT_TOKEN and CLCMS_GIT_USER. defaultUser is the
// platform's convention for token logins (x-access-token on GitHub, oauth2 on
// GitLab, ...).
func FromEnv(defaultUser string) Auth {
	a := Auth{User: os.Getenv("CLCMS_GIT_USER"), Token: os.Getenv("CLCMS_GIT_TOKEN")}
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

// Checkout makes dir a shallow clone of one branch at its tip: cloning the
// first time, fetching and resetting after. The folder belongs to cl-cms, so
// anything in it that isn't on the branch is thrown away.
func Checkout(auth Auth, repo, branch, dir string) (string, error) {
	if _, err := os.Stat(dir + "/.git"); err != nil {
		if _, err := Run(auth, "", "clone", "--quiet", "--depth", "1", "--branch", branch, "--single-branch", repo, dir); err != nil {
			return "", err
		}
	} else {
		if _, err := Run(auth, dir, "fetch", "--quiet", "--depth", "1", "origin", branch); err != nil {
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
