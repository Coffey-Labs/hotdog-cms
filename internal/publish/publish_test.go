package publish

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/starter"
)

func starterSite(t *testing.T, dir string) {
	t.Helper()
	err := fs.WalkDir(starter.Files, "site", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, strings.TrimPrefix(p, "site"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, _ := starter.Files.ReadFile(p)
		return os.WriteFile(target, []byte(strings.ReplaceAll(string(data), "{{SITE_NAME}}", "Test")), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func built(t *testing.T) string {
	t.Helper()
	site := t.TempDir()
	starterSite(t, site)
	res, err := build.Run(build.Options{SiteDir: site})
	if err != nil {
		t.Fatal(err)
	}
	return res.Out
}

func TestDirTarget(t *testing.T) {
	out := built(t)
	root := filepath.Join(t.TempDir(), "www")
	var log bytes.Buffer
	for i := 0; i < 2; i++ { // the second run replaces the first
		if err := Run("local", Target{Type: "dir", Path: root}, out, &log); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		t.Fatal("index.html not published")
	}
	foreign := t.TempDir()
	os.WriteFile(filepath.Join(foreign, "keep.txt"), []byte("x"), 0o644)
	if err := Run("local", Target{Type: "dir", Path: foreign}, out, &log); err == nil {
		t.Fatal("replaced a folder hotdog-cms did not write")
	}
}

// fakeSSH puts an ssh on PATH that runs the remote command on this machine,
// so the rsync target can be exercised without a server.
func fakeSSH(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
while [ $# -gt 0 ]; do
  case "$1" in
    -o|-p|-l) shift 2 ;;
    -*) shift ;;
    *) break ;;
  esac
done
shift # the host
exec sh -c "$*"
`
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRsyncTarget(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not installed")
	}
	fakeSSH(t)
	out := built(t)
	live := filepath.Join(t.TempDir(), "srv", "site")
	tg := Target{Type: "rsync", Host: "deploy@example.test", Path: live}
	var log bytes.Buffer
	for i := 0; i < 2; i++ {
		if err := Run("server", tg, out, &log); err != nil {
			t.Fatalf("run %d: %v\n%s", i, err, log.String())
		}
	}
	if _, err := os.Stat(filepath.Join(live, "articles", "hello-world", "index.html")); err != nil {
		t.Fatal("article not published")
	}
	for _, leftover := range []string{live + ".hotdog-cms-staging", live + ".hotdog-cms-old"} {
		if _, err := os.Stat(leftover); err == nil {
			t.Errorf("%s left behind", leftover)
		}
	}
	foreign := filepath.Join(t.TempDir(), "srv", "other")
	os.MkdirAll(foreign, 0o755)
	os.WriteFile(filepath.Join(foreign, "keep.txt"), []byte("x"), 0o644)
	if err := Run("server", Target{Type: "rsync", Host: "h", Path: foreign}, out, &log); err == nil {
		t.Fatal("replaced a remote folder hotdog-cms did not write")
	}
	if _, err := os.Stat(filepath.Join(foreign, "keep.txt")); err != nil {
		t.Fatal("the remote folder's file is gone")
	}
}

func TestTargetValidation(t *testing.T) {
	bad := []Target{
		{Type: "rsync", Host: "h", Path: "/"},
		{Type: "rsync", Host: "h", Path: "/var"},
		{Type: "rsync", Host: "h", Path: "relative/path"},
		{Type: "rsync", Host: "h; rm -rf /", Path: "/srv/site"},
		{Type: "container"},
		{Type: "ftp", Path: "/x"},
	}
	for _, tg := range bad {
		if err := tg.validate(); err == nil {
			t.Errorf("accepted %+v", tg)
		}
	}
}

func TestCommandAndContainerContext(t *testing.T) {
	out := built(t)
	var log bytes.Buffer
	if err := Run("hook", Target{Type: "command", Command: []string{"sh", "-c", `test -f "$HOTDOG_OUT/index.html" && test -f "$0/index.html"`, "{out}"}}, out, &log); err != nil {
		t.Fatalf("command target: %v", err)
	}
	ctx := filepath.Join(t.TempDir(), "ctx")
	if err := Run("image", Target{Type: "container", Context: ctx}, out, &log); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"Containerfile", "Dockerfile", "nginx.conf", "site/index.html"} {
		if _, err := os.Stat(filepath.Join(ctx, f)); err != nil {
			t.Errorf("context is missing %s", f)
		}
	}
}

func TestPull(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	starterSite(t, repo)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.test", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-q", "-m", "site")
	www := filepath.Join(t.TempDir(), "www")
	opt := PullOptions{Repo: repo, Out: www, Cache: filepath.Join(t.TempDir(), "clone"), Log: &bytes.Buffer{}}
	if err := Pull(opt); err != nil {
		t.Fatal(err)
	}
	about := filepath.Join(www, "about", "index.html")
	if b, _ := os.ReadFile(about); !strings.Contains(string(b), "<h1>About</h1>") {
		t.Fatal("first pull did not publish")
	}
	os.WriteFile(filepath.Join(repo, "content", "about.md"), []byte("---\ntitle: Changed\n---\nNew text.\n"), 0o644)
	git("commit", "-q", "-am", "edit")
	if err := Pull(opt); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(about); !strings.Contains(string(b), "Changed") {
		t.Fatal("second pull did not publish the change")
	}
	// A commit that breaks the build leaves the live site as it was.
	os.WriteFile(filepath.Join(repo, "content", "broken.md"), []byte("---\nlayout: nope\ntitle: x\n---\n"), 0o644)
	git("add", "-A")
	git("commit", "-q", "-m", "broken")
	if err := Pull(opt); err == nil {
		t.Fatal("a broken build reported success")
	}
	if b, _ := os.ReadFile(about); !strings.Contains(string(b), "Changed") {
		t.Fatal("a broken build changed the live site")
	}
}

func TestPullRefusesFailedChecks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	starterSite(t, repo)
	cfg, _ := os.ReadFile(filepath.Join(repo, "site.yaml"))
	os.WriteFile(filepath.Join(repo, "site.yaml"), []byte(strings.Replace(string(cfg), "  forbid: []", "  forbid: ['internal\\.example']", 1)), 0o644)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.test", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-q", "-m", "site")
	www := filepath.Join(t.TempDir(), "www")
	opt := PullOptions{Repo: repo, Out: www, Cache: filepath.Join(t.TempDir(), "clone"), Log: &bytes.Buffer{}}
	if err := Pull(opt); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "content", "about.md"), []byte("---\ntitle: About\n---\nOur server is db1.internal.example.\n"), 0o644)
	git("commit", "-q", "-am", "leak a host name")
	err := Pull(opt)
	if err == nil || !strings.Contains(err.Error(), "check error") {
		t.Fatalf("a commit failing its checks was published: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(www, "about", "index.html")); strings.Contains(string(b), "internal.example") {
		t.Fatal("the leak reached the live site")
	}
	if _, err := os.Stat(www + ".hotdog-cms-check"); err == nil {
		t.Error("staging folder left behind")
	}
}

func TestContainerCSP(t *testing.T) {
	out := built(t)
	ctx := filepath.Join(t.TempDir(), "ctx")
	if err := Run("image", Target{Type: "container", Context: ctx, CSP: true}, out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	conf, _ := os.ReadFile(filepath.Join(ctx, "nginx.conf"))
	if !strings.Contains(string(conf), `add_header Content-Security-Policy "default-src 'self'; script-src 'self';`) {
		t.Errorf("nginx.conf has no strict CSP:\n%s", conf)
	}
}

func TestBeforeAndAfterSteps(t *testing.T) {
	out := built(t)
	root := filepath.Join(t.TempDir(), "www")
	seen := filepath.Join(t.TempDir(), "after")
	tg := Target{
		Type:   "dir",
		Path:   root,
		Before: [][]string{{"sh", "-c", `echo indexed > "$0/search.txt"`, "{out}"}},
		After:  [][]string{{"sh", "-c", `test -f "$0/search.txt" && echo "$HOTDOG_TARGET" > "$1"`, root, seen}},
	}
	if err := tg.validate(); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if err := Run("live", tg, out, &log); err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if got, _ := os.ReadFile(seen); string(got) != "live\n" {
		t.Errorf("after step didn't see the published change: %q", got)
	}
	// A failing before step stops the publish.
	root2 := filepath.Join(t.TempDir(), "www")
	tg = Target{Type: "dir", Path: root2, Before: [][]string{{"false"}}}
	if err := Run("live", tg, out, &log); err == nil {
		t.Error("published after a failed before step")
	}
	if _, err := os.Stat(root2); err == nil {
		t.Error("before step failed but the site was published")
	}
	if err := (Target{Type: "dir", Path: "/x", After: [][]string{{}}}).validate(); err == nil {
		t.Error("accepted an empty step")
	}
}
