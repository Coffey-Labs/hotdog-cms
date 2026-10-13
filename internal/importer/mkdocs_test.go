package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/packs"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

func TestMkDocs(t *testing.T) {
	mk := t.TempDir()
	files := map[string]string{
		"mkdocs.yml": `site_name: Example docs
site_url: https://docs.example.org/
site_description: How to use it.
theme:
  name: material
  custom_dir: overrides
markdown_extensions:
  - admonition
  - abbr
  - pymdownx.keys:
      key_map:
        hash: "#"
  - pymdownx.emoji:
      emoji_index: !!python/name:material.extensions.emoji.twemoji
plugins:
  - search
  - macros
nav:
  - Home: index.md
  - Install it:
      - install/index.md
      - On Linux: install/linux.md
      - Elsewhere:
          - install/mac.md
  - Back to the site: https://example.org/
`,
		"docs/index.md":         "# Example docs\n\nStart with [installing](install/index.md).\n",
		"docs/install/index.md": "---\ndescription: Three ways.\n---\n\nInstall\n=======\n\nPick one.\n",
		"docs/install/linux.md": "# On Linux { #linux }\n\n![Shot](../img/shot.png){ .shot }\n\n[Back](../index.md#start) and [the guide](../files/guide.pdf), [missing](../nope.png), [away](https://example.com/a.png).\n\n<img src=\"../img/shot.png\" alt=\"x\">\n\n```sh\n![not a link](../img/shot.png)\n```\n\n## On Linux & BSD\n\n:material-check: Done, {{ version }}. ++hash++\n",
		"docs/install/mac.md":   "No heading here.\n",
		"docs/img/shot.png":     "png",
		"docs/files/guide.pdf":  "pdf",
		"docs/.hidden/x.md":     "# Hidden\n",
	}
	for name, body := range files {
		p := filepath.Join(mk, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	dir := starterSite(t)
	if _, err := FromMkDocs(MkDocsOptions{From: mk, SiteDir: dir}); err == nil || !strings.Contains(err.Error(), "docs pack") {
		t.Fatalf("without the docs pack: %v", err)
	}
	if _, err := packs.Add(dir, "docs", false); err != nil {
		t.Fatal(err)
	}
	if _, err := FromMkDocs(MkDocsOptions{From: mk, SiteDir: dir}); err == nil || !strings.Contains(err.Error(), "content/index.md") {
		t.Fatalf("a home page where the docs' goes: %v", err)
	}
	os.Remove(filepath.Join(dir, "content", "index.md"))
	rep, err := FromMkDocs(MkDocsOptions{From: mk, SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Pages) != 4 || len(rep.Files) != 2 || rep.NavItems != 6 {
		t.Errorf("pages %v files %v nav %d", rep.Pages, rep.Files, rep.NavItems)
	}

	for file, wants := range map[string][]string{
		"content/docs/_index.md":         {"title: Example docs\nlayout: doc\n---\nStart with [installing](install/index.md)."},
		"content/docs/install/_index.md": {"title: Install\ndescription: Three ways.\nlayout: doc\n---\nPick one."},
		"content/docs/install/linux.md": {"---\ntitle: On Linux\n---\n", "![Shot](/img/shot.png){ .shot }", "[Back](../index.md#start)", "[the guide](/files/guide.pdf)",
			"[missing](../nope.png)", "(https://example.com/a.png)", `<img src="/img/shot.png"`, "![not a link](../img/shot.png)"},
		"content/docs/install/mac.md": {"title: Mac\n"},
		"static/img/shot.png":         {"png"},
		"data/docs-nav.yaml":          {"- title: Home\n  page: docs/_index.md\n- title: Install it\n  page: docs/install/_index.md\n  children:\n    - title: On Linux\n      page: docs/install/linux.md\n    - title: Elsewhere\n      children:\n        - docs/install/mac.md\n- title: Back to the site\n  url: https://example.org/\n"},
		"site.yaml":                   {"name: Example docs\n", "url: https://docs.example.org\n", "    path: /\n", "heading_ids: mkdocs", "hash: '#'", "unsafe_html: true"},
	} {
		got := read(t, filepath.Join(dir, file))
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("%s lacks %q:\n%s", file, w, got)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "content", "docs", ".hidden")); err == nil {
		t.Error("a dot folder came across")
	}
	md := rep.Markdown(mk)
	for _, w := range []string{"markdown_extensions: abbr", "markdown_extensions: pymdownx.emoji", "plugins: macros", "theme.custom_dir: overrides",
		"`install/linux.md`: icons and emoji (:material-…:): 1; template tags ({{ … }}, {% … %}): 1"} {
		if !strings.Contains(md, w) {
			t.Errorf("report lacks %q:\n%s", w, md)
		}
	}

	// The site builds, at MkDocs' addresses.
	s, err := site.Load(dir, site.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for src, want := range map[string]string{"docs/_index.md": "/", "docs/install/_index.md": "/install/", "docs/install/linux.md": "/install/linux/"} {
		if p := s.PageFrom(src); p == nil || p.Path != want {
			t.Errorf("%s: want %s, got %+v", src, want, p)
		}
	}
	linux := string(s.PageFrom("docs/install/linux.md").Content)
	for _, w := range []string{`href="/#start"`, `id="on-linux-bsd"`, `<kbd class="key-hash">#</kbd>`, `src="/img/shot.png"`} {
		if !strings.Contains(linux, w) {
			t.Errorf("linux page lacks %s:\n%s", w, linux)
		}
	}

	// A second import leaves what's there; -overwrite replaces it.
	rep, _ = FromMkDocs(MkDocsOptions{From: mk, SiteDir: dir})
	if len(rep.Pages) != 0 || len(rep.Skipped) != 7 {
		t.Errorf("again: wrote %v, skipped %v", rep.Pages, rep.Skipped)
	}
}

func TestPlainTitle(t *testing.T) {
	for in, want := range map[string]string{
		"ASN &amp; GeoIP":             "ASN & GeoIP",
		"The `serve` command":         "The serve command",
		"**Bold** and [a link](x.md)": "Bold and a link",
		"Use \\* wildcards":           "Use * wildcards",
		"Plain":                       "Plain",
	} {
		if got := plainTitle(in); got != want {
			t.Errorf("plainTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEscapeCodePipes(t *testing.T) {
	for in, want := range map[string]string{
		"| a | `(|(x=?)(y=?))` |":  "| a | `(\\|(x=?)(y=?))` |",
		"| `` a|b `` | c |":        "| `` a\\|b `` | c |",
		"| `a\\|b` | already |":    "| `a\\|b` | already |",
		"| \\` | not code | x` |":  "| \\` | not code | x` |",
		"| `open | never closed |": "| `open | never closed |",
	} {
		if got := escapeCodePipes(in); got != want {
			t.Errorf("escapeCodePipes(%q) = %q, want %q", in, got, want)
		}
	}
}
