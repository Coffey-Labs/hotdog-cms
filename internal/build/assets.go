package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Assets are the files under assets/: stylesheets, scripts, fonts, the
// images templates point at. Each is written with a hash of its content in
// its name (styles.css -> styles.3f9a1c2b07.css), so a browser or a CDN can
// keep it for a year and a changed file is a new address. Nobody bumps a
// version by hand, and no edge cache can serve last week's stylesheet.
type Assets struct {
	urls map[string]string // "styles.css" -> "/styles.3f9a1c2b07.css"
}

// URL returns the published address of an asset, by its path under assets/.
func (a *Assets) URL(name string) (string, error) {
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	u, ok := a.urls[name]
	if !ok {
		return "", fmt.Errorf("no asset %q in assets/", name)
	}
	return u, nil
}

var cssURLRe = regexp.MustCompile(`url\(\s*(['"]?)([^'")]+)(['"]?)\s*\)`)

// writeAssets copies assets/ into out with fingerprinted names. Stylesheets go
// last, because their url() references are rewritten to the fingerprinted
// names of the fonts and images they use first.
func writeAssets(srcDir, out string) (*Assets, error) {
	a := &Assets{urls: map[string]string{}}
	var files []string
	err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != srcDir {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		rel, _ := filepath.Rel(srcDir, p)
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if os.IsNotExist(err) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool {
		return !strings.HasSuffix(files[i], ".css") && strings.HasSuffix(files[j], ".css")
	})
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(srcDir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(rel, ".css") {
			data = []byte(cssURLRe.ReplaceAllStringFunc(string(data), func(m string) string {
				sub := cssURLRe.FindStringSubmatch(m)
				ref := sub[2]
				if strings.Contains(ref, ":") || strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "#") {
					return m
				}
				clean := ref
				suffix := ""
				if i := strings.IndexAny(clean, "?#"); i >= 0 {
					clean, suffix = clean[:i], clean[i:]
				}
				target := path.Clean(path.Join(path.Dir(rel), clean))
				if u, ok := a.urls[target]; ok {
					return "url(" + sub[1] + u + suffix + sub[3] + ")"
				}
				return m
			}))
		}
		sum := sha256.Sum256(data)
		ext := path.Ext(rel)
		hashed := strings.TrimSuffix(rel, ext) + "." + hex.EncodeToString(sum[:])[:10] + ext
		if out != "" { // "" only computes the names, for rendering one page
			dst := filepath.Join(out, filepath.FromSlash(hashed))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return nil, err
			}
		}
		a.urls[rel] = "/" + hashed
	}
	return a, nil
}

// copyTree copies a folder as it is, for static/ and the non-Markdown files
// that live beside pages in content/.
func copyTree(src, dst string, skip func(rel string, d fs.DirEntry) bool) error {
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if d.Name() == ".git" || d.Name() == ".DS_Store" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if skip != nil && skip(filepath.ToSlash(rel), d) {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink; copy the file in instead", p)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// add publishes a file cl-cms supplies itself (such as the forms script) as
// an asset, fingerprinted like the site's own. A site file of the same name
// wins, so a site can replace it.
func (a *Assets) add(out, rel string, data []byte) error {
	if _, ok := a.urls[rel]; ok {
		return nil
	}
	sum := sha256.Sum256(data)
	ext := path.Ext(rel)
	hashed := strings.TrimSuffix(rel, ext) + "." + hex.EncodeToString(sum[:])[:10] + ext
	if out != "" {
		if err := os.WriteFile(filepath.Join(out, filepath.FromSlash(hashed)), data, 0o644); err != nil {
			return err
		}
	}
	a.urls[rel] = "/" + hashed
	return nil
}
