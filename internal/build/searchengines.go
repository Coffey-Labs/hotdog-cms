package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Search engines: verification meta tags, and what IndexNow needs. Every
// build writes a manifest of its pages and a hash of each, so publishing can
// tell search engines exactly which pages changed (internal/publish).

// PagesManifest is the file listing a build's pages: "address<TAB>hash".
const PagesManifest = ".hotdog-cms-pages"

// IndexNowFile is the build's note of its IndexNow key and address.
const IndexNowFile = ".hotdog-cms-indexnow"

// IndexNowInfo is what IndexNowFile holds.
type IndexNowInfo struct {
	Key string `json:"key"`
	URL string `json:"url"`
}

func verificationTags(cfg *site.Config) template.HTML {
	var b strings.Builder
	for _, kv := range [][2]string{{"google-site-verification", cfg.Verify.Google}, {"msvalidate.01", cfg.Verify.Bing}, {"yandex-verification", cfg.Verify.Yandex}} {
		if kv[1] != "" {
			b.WriteString(`<meta name="` + kv[0] + `" content="` + template.HTMLEscapeString(kv[1]) + `">`)
		}
	}
	return template.HTML(b.String())
}

// writeSearchFiles writes the pages manifest, and the IndexNow key file and
// note when the site has a key.
func writeSearchFiles(out string, cfg *site.Config) error {
	var lines []string
	err := filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(data[:min(len(data), 512)]), `http-equiv="refresh"`) {
			return nil // a redirect, not a page
		}
		rel, _ := filepath.Rel(out, p)
		u := "/" + strings.TrimSuffix(filepath.ToSlash(rel), "index.html")
		sum := sha256.Sum256(data)
		lines = append(lines, u+"\t"+hex.EncodeToString(sum[:12]))
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(lines)
	if err := os.WriteFile(filepath.Join(out, PagesManifest), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	if k := cfg.IndexNow.Key; k != "" {
		if err := os.WriteFile(filepath.Join(out, k+".txt"), []byte(k), 0o644); err != nil {
			return err
		}
		note, _ := json.Marshal(IndexNowInfo{Key: k, URL: cfg.URL})
		return os.WriteFile(filepath.Join(out, IndexNowFile), note, 0o644)
	}
	return nil
}
