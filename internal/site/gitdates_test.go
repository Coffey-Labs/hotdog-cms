package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestGitLastmod dates pages by their last commit, today for changes not
// yet committed, and leaves front matter's updated: in charge. The site is
// in a subfolder of its repository, as a monorepo would have it.
func TestGitLastmod(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := t.TempDir()
	dir := filepath.Join(repo, "site")
	run := func(date string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date,
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.org", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.org",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	put := func(rel, body string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("site.yaml", "name: T\nurl: https://example.org\nlastmod: git\n")
	put("content/a.md", "---\ntitle: A\n---\n")
	put("content/docs/b.md", "---\ntitle: B\n---\n")
	put("content/c.md", "---\ntitle: C\nupdated: 2020-01-02\n---\n")
	run("", "init", "-q")
	run("2026-03-01T10:00:00Z", "add", ".")
	run("2026-03-01T10:00:00Z", "commit", "-q", "-m", "one")
	put("content/a.md", "---\ntitle: A, again\n---\n")
	run("2026-05-07T23:30:00-07:00", "commit", "-q", "-am", "two")
	put("content/docs/b.md", "---\ntitle: B, changed\n---\n")
	put("content/new.md", "---\ntitle: New\n---\n")

	now := time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)
	s, err := Load(dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"a.md":      "2026-05-07", // the second commit, in its own zone
		"docs/b.md": "2026-10-19", // changed, not committed
		"new.md":    "2026-10-19", // not added yet
		"c.md":      "2020-01-02", // updated: wins
	}
	for _, p := range s.Pages {
		if w, ok := want[p.Source]; ok {
			if got := p.Lastmod().Format("2006-01-02"); got != w {
				t.Errorf("%s: lastmod %s, want %s", p.Source, got, w)
			}
			delete(want, p.Source)
		}
	}
	if len(want) > 0 {
		t.Errorf("pages not found: %v", want)
	}

	// Without the setting, or outside git, there are no commit dates.
	put("site.yaml", "name: T\nurl: https://example.org\n")
	if s, _ = Load(dir, Options{Now: now}); s.PageFrom("a.md").Lastmod().IsZero() == false {
		t.Error("dated a page without lastmod: git")
	}
	plain := t.TempDir()
	os.WriteFile(filepath.Join(plain, "site.yaml"), []byte("name: T\nurl: https://example.org\nlastmod: git\n"), 0o644)
	os.MkdirAll(filepath.Join(plain, "content"), 0o755)
	os.WriteFile(filepath.Join(plain, "content", "a.md"), []byte("---\ntitle: A\n---\n"), 0o644)
	if s, err = Load(plain, Options{Now: now}); err != nil || !s.PageFrom("a.md").Lastmod().IsZero() {
		t.Errorf("outside git: %v", err)
	}
	put("site.yaml", "name: T\nurl: https://example.org\nlastmod: mtime\n")
	if _, err := Load(dir, Options{Now: now}); err == nil {
		t.Error("accepted lastmod: mtime")
	}
}
