package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceLinks(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"site.yaml":                     "name: T\nurl: https://example.org\n",
		"content/index.md":              "---\ntitle: Home\n---\nSee [install](docs/install/linux.md#docker) and [docs](docs/index.md).\n",
		"content/docs/_index.md":        "---\ntitle: Docs\n---\nStart at [Linux](install/linux.md).\n",
		"content/docs/install/linux.md": "---\ntitle: Linux\n---\n[Back](../_index.md), [macOS](mac.md), [top](/index.md), [gone](nowhere.md), [away](https://example.com/x.md), [up and out](../../../etc.md).\n",
		"content/docs/install/mac.md":   "---\ntitle: macOS\n---\nSee [Linux](./linux.md#top).\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	s, err := Load(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	page := func(src string) string {
		for _, p := range s.Pages {
			if p.Source == src {
				return string(p.Content)
			}
		}
		t.Fatalf("no page from %s", src)
		return ""
	}
	for src, wants := range map[string][]string{
		"index.md":              {`href="/docs/install/linux/#docker"`, `href="/docs/"`},
		"docs/_index.md":        {`href="/docs/install/linux/"`},
		"docs/install/linux.md": {`href="/docs/"`, `href="/docs/install/mac/"`, `href="/"`, `href="nowhere.md"`, `href="https://example.com/x.md"`, `href="../../../etc.md"`},
		"docs/install/mac.md":   {`href="/docs/install/linux/#top"`},
	} {
		got := page(src)
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("%s: missing %s in %s", src, w, got)
			}
		}
	}
}
