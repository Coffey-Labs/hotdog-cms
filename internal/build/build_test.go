package build

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/cl-cms/starter"
)

// writeStarter puts the starter site in a temporary folder.
func writeStarter(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
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
	return dir
}

func TestStarterBuilds(t *testing.T) {
	dir := writeStarter(t)
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.html", "about/index.html", "articles/index.html", "articles/hello-world/index.html",
		"tags/cl-cms/index.html", "404.html", "sitemap.xml", "robots.txt", "articles/feed.xml", "llms.txt"} {
		if _, err := os.Stat(filepath.Join(res.Out, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	home, _ := os.ReadFile(filepath.Join(res.Out, "index.html"))
	if strings.Contains(string(home), `href="/styles.css"`) || !strings.Contains(string(home), `/styles.`) {
		t.Error("stylesheet is not fingerprinted")
	}
	// A second build replaces the first.
	if _, err := Run(Options{SiteDir: dir}); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
}

func TestTemplatesEscape(t *testing.T) {
	dir := writeStarter(t)
	page := "---\ntitle: \"<script>alert(1)</script>\"\nsummary: '\"><img src=x onerror=alert(1)>'\n---\n\n<script>alert(2)</script>\n\n[x](javascript:alert(3))\n"
	if err := os.WriteFile(filepath.Join(dir, "content", "evil.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(filepath.Join(res.Out, "evil", "index.html"))
	for _, bad := range []string{"<script>alert", "<img src=x", "javascript:alert"} {
		if strings.Contains(string(out), bad) {
			t.Errorf("page contains %q", bad)
		}
	}
}

func TestRefusesForeignOutput(t *testing.T) {
	dir := writeStarter(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "precious.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{SiteDir: dir, Out: other}); err == nil {
		t.Fatal("replaced a folder cl-cms did not write")
	}
	if _, err := os.Stat(filepath.Join(other, "precious.txt")); err != nil {
		t.Fatal("the folder's file is gone")
	}
	if _, err := Run(Options{SiteDir: dir, Out: filepath.Join(dir, "content", "out")}); err == nil {
		t.Fatal("wrote output inside content/")
	}
	if _, err := Run(Options{SiteDir: dir, Out: filepath.Dir(dir)}); err == nil {
		t.Fatal("wrote output over the site's parent")
	}
}
