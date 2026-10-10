package publish

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/build"
)

// PullOptions configure the pull agent: cl-cms running on the web server
// itself, fetching the site's repository and rebuilding when the branch moves.
// Nothing pushes to the server, so the server needs no inbound deploy access
// at all, and the repository can be read with a read-only deploy key.
type PullOptions struct {
	Repo    string        // anything git can clone
	Branch  string        // default main
	Subdir  string        // the site's folder inside the repository, if not the root
	Out     string        // the web root to publish into
	Cache   string        // where the clone lives; default under the user cache dir
	Every   time.Duration // 0: check once and exit (for a systemd timer or a webhook)
	Log     io.Writer
	OnBuild func(*build.Result)
}

// Pull checks the branch and rebuilds when it has moved; with Every set, it
// keeps doing so.
func Pull(o PullOptions) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("the pull agent needs git installed")
	}
	if o.Branch == "" {
		o.Branch = "main"
	}
	if o.Cache == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(o.Repo + "\x00" + o.Branch))
		o.Cache = filepath.Join(base, "cl-cms", "pull-"+hex.EncodeToString(sum[:8]))
	}
	last := ""
	for {
		head, err := o.sync()
		switch {
		case err != nil:
			fmt.Fprintf(o.Log, "pull: %v\n", err)
		case head != last:
			var res *build.Result
			res, err = build.Run(build.Options{SiteDir: filepath.Join(o.Cache, o.Subdir), Out: o.Out})
			if err != nil {
				err = fmt.Errorf("build of %s failed, the live site is unchanged: %w", short(head), err)
				fmt.Fprintln(o.Log, err)
			} else {
				fmt.Fprintf(o.Log, "published %s: %d pages to %s in %s\n", short(head), res.Pages, res.Out, res.Duration.Round(time.Millisecond))
				last = head
				if o.OnBuild != nil {
					o.OnBuild(res)
				}
			}
		}
		if o.Every == 0 {
			return err
		}
		time.Sleep(o.Every)
	}
}

// sync brings the private clone to the branch's tip and returns its commit.
func (o PullOptions) sync() (string, error) {
	git := func(args ...string) (string, error) {
		var out, errb bytes.Buffer
		cmd := exec.Command("git", args...)
		cmd.Stdout, cmd.Stderr = &out, &errb
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(errb.String()))
		}
		return strings.TrimSpace(out.String()), nil
	}
	if _, err := os.Stat(filepath.Join(o.Cache, ".git")); err != nil {
		if err := os.MkdirAll(filepath.Dir(o.Cache), 0o755); err != nil {
			return "", err
		}
		if _, err := git("clone", "--quiet", "--depth", "1", "--branch", o.Branch, "--single-branch", o.Repo, o.Cache); err != nil {
			return "", err
		}
	} else {
		if _, err := git("-C", o.Cache, "fetch", "--quiet", "--depth", "1", "origin", o.Branch); err != nil {
			return "", err
		}
		// The clone is ours alone, so nothing in it is worth keeping over the branch.
		if _, err := git("-C", o.Cache, "reset", "--quiet", "--hard", "FETCH_HEAD"); err != nil {
			return "", err
		}
		if _, err := git("-C", o.Cache, "clean", "-qfdx"); err != nil {
			return "", err
		}
	}
	return git("-C", o.Cache, "rev-parse", "HEAD")
}

func short(h string) string {
	if len(h) > 10 {
		return h[:10]
	}
	return h
}
