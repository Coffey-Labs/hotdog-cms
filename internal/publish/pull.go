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

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/forge"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/gitx"
)

// PullOptions configure the pull agent: cl-cms running on the web server
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
	OnBuild func(*build.Result)

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
		o.Cache = filepath.Join(base, "cl-cms", "pull-"+hex.EncodeToString(sum[:8]))
	}
	kick := make(chan struct{}, 1)
	if o.HookAddr != "" {
		if o.HookSecret == "" {
			return errors.New("a webhook needs a secret (CLCMS_HOOK_SECRET)")
		}
		go func() {
			srv := &http.Server{Addr: o.HookAddr, Handler: hookHandler(o, kick), ReadHeaderTimeout: 10 * time.Second}
			fmt.Fprintf(o.Log, "listening for %s webhooks on %s/_cl/hook\n", o.HookKind, o.HookAddr)
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
		if o.Every == 0 && o.HookAddr == "" {
			return err
		}
		select {
		case <-tick:
		case <-kick:
		}
	}
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
		if r.URL.Path != "/_cl/hook" || r.Method != http.MethodPost {
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
