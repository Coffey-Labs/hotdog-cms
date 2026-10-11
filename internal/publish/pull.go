package publish

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/check"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forge"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/gitx"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/isolate"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// PullOptions configure the pull agent: hotdog-cms running on the web server
// itself, fetching the site's repository and rebuilding when the branch moves.
// Nothing pushes to the server, so the server needs no deploy access, and the
// repository can be read with a read-only deploy key or token.
type PullOptions struct {
	Repo    string        // anything git can clone
	Branch  string        // default main
	Subdir  string        // the site's folder inside the repository, if not the root
	Out     string        // the web root to publish into
	Cache   string        // where the clone lives; default under the user cache dir
	Every   time.Duration // 0 with no hook: check once and exit (systemd timer)
	Git     gitx.Auth
	Log     io.Writer
	OnBuild func(*isolate.Summary)
	// IndexNow tells search engines which pages changed after each publish.
	IndexNow bool

	// Optional: listen for the platform's push webhook and rebuild at once.
	// This opens a port on the server, so it is off unless asked for, and it
	// only answers deliveries signed with the secret.
	HookAddr   string
	HookSecret string
	HookKind   forge.Kind
}

// Pull checks the branch and rebuilds when it has moved; with Every or a
// hook set, it keeps doing so.
func Pull(o PullOptions) error {
	if err := gitx.Available(); err != nil {
		return fmt.Errorf("the pull agent needs git: %w", err)
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
		o.Cache = filepath.Join(base, "hotdog-cms", "pull-"+hex.EncodeToString(sum[:8]))
	}
	kick := make(chan struct{}, 1)
	if o.HookAddr != "" {
		if o.HookSecret == "" {
			return errors.New("a webhook needs a secret (HOTDOG_HOOK_SECRET)")
		}
		go func() {
			srv := &http.Server{Addr: o.HookAddr, Handler: hookHandler(o, kick), ReadHeaderTimeout: 10 * time.Second}
			fmt.Fprintf(o.Log, "listening for %s webhooks on %s/_hotdog/hook\n", o.HookKind, o.HookAddr)
			if err := srv.ListenAndServe(); err != nil {
				fmt.Fprintf(o.Log, "webhook listener: %v\n", err)
			}
		}()
	}
	var tick <-chan time.Time
	if o.Every > 0 {
		t := time.NewTicker(o.Every)
		defer t.Stop()
		tick = t.C
	}
	last := ""
	var due time.Time // a scheduled page's publish_at: rebuild then, commit or not
	for {
		head, err := o.sync()
		dueNow := !due.IsZero() && !time.Now().Before(due)
		switch {
		case err != nil:
			fmt.Fprintf(o.Log, "pull: %v\n", err)
		case head != last || dueNow:
			var res *isolate.Summary
			res, err = buildChecked(filepath.Join(o.Cache, o.Subdir), o.Out)
			if err != nil {
				err = fmt.Errorf("build of %s failed, the live site is unchanged: %w", short(head), err)
				fmt.Fprintln(o.Log, err)
			} else {
				fmt.Fprintf(o.Log, "published %s: %d pages to %s in %s\n", short(head), res.Pages, res.Out, res.Duration.Round(time.Millisecond))
				if o.IndexNow {
					indexNow("pull", Target{Type: "dir", Path: o.Out}, o.Out, o.Log)
				}
				last = head
				due = res.NextScheduled
				if !due.IsZero() {
					fmt.Fprintf(o.Log, "next scheduled page goes live at %s\n", due.Format(time.RFC3339))
				}
				if o.OnBuild != nil {
					o.OnBuild(res)
				}
			}
		}
		if o.Every == 0 && o.HookAddr == "" {
			return err
		}
		var wake <-chan time.Time
		if !due.IsZero() {
			wake = time.After(time.Until(due) + time.Second)
		}
		select {
		case <-tick:
		case <-kick:
		case <-wake:
		}
	}
}

// buildChecked builds into a staging folder, runs the site's checks on it,
// and only then puts it live, so a commit that fails a check never reaches
// the web root.
func buildChecked(siteDir, out string) (*isolate.Summary, error) {
	staging := out + ".hotdog-cms-check"
	res, err := isolate.Build(build.Options{SiteDir: siteDir, Out: staging})
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)
	cfg, err := site.LoadConfig(siteDir)
	if err != nil {
		return nil, err
	}
	rep, err := check.Run(siteDir, staging, cfg)
	if err != nil {
		return nil, err
	}
	if n := rep.Errors(); n > 0 {
		first := ""
		for _, p := range rep.Problems {
			if p.Level == check.Error {
				first = p.String()
				break
			}
		}
		return nil, fmt.Errorf("%d check error(s), first: %s", n, first)
	}
	if err := build.Install(staging, out); err != nil {
		return nil, err
	}
	res.Out = out
	return res, nil
}

// sync brings the private clone to the branch's tip and returns its commit.
func (o PullOptions) sync() (string, error) {
	if err := os.MkdirAll(filepath.Dir(o.Cache), 0o755); err != nil {
		return "", err
	}
	return gitx.Checkout(o.Git, o.Repo, o.Branch, o.Cache)
}

func hookHandler(o PullOptions, kick chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_hotdog/hook" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
		if err != nil {
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}
		if err := forge.Verify(o.HookKind, r.Header, body, o.HookSecret); err != nil {
			fmt.Fprintf(o.Log, "webhook refused: %v\n", err)
			http.Error(w, "signature check failed", http.StatusUnauthorized)
			return
		}
		push, err := forge.ParsePush(o.HookKind, r.Header, body)
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if push.Branch == o.Branch && !push.Deleted {
			select {
			case kick <- struct{}{}:
			default:
			}
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

func short(h string) string {
	if len(h) > 10 {
		return h[:10]
	}
	return h
}
