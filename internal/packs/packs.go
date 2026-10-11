// Package packs installs extensions the safe way: as files. A pack is a
// folder, or a git repository, holding sections, partials, layouts and
// stylesheets, with a pack.yaml saying what it is. Installing copies its
// files into the site, where they're reviewed, versioned and changed like the
// site's own, and records it in site.yaml. A pack carries no code: no
// JavaScript, no SVG (which can hold scripts), and no template with a script
// tag, an event handler or a javascript: link. Its templates run through the
// same escaping, checks and Content-Security-Policy as everything else.
//
//	# pack.yaml
//	name: faq
//	version: 1.0.0
//	title: FAQ
//	description: Questions and answers that open and close.
//	license: MIT
//
// Its files keep the site's layout: sections/faq.html, partials/…,
// layouts/…, assets/…. Assets are installed under assets/packs/<name>/, so
// two packs never collide; a pack's templates refer to them that way:
// {{ asset "packs/faq/faq.css" }}.
package packs

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/siteyaml"
)

// Manifest is a pack's pack.yaml.
type Manifest struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	License     string `yaml:"license"`
	Author      string `yaml:"author"`
}

var (
	nameRe   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	assetExt = map[string]bool{".css": true, ".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".avif": true, ".woff2": true, ".woff": true, ".ttf": true, ".otf": true}
	// What would make a template run code in visitors' browsers.
	codeInTemplate = regexp.MustCompile(`(?i)<script\b|\son[a-z]+\s*=|javascript:|<iframe\b|<object\b|<embed\b`)
	metaFiles      = map[string]bool{"pack.yaml": true, "README.md": true, "LICENSE": true, "LICENSE.md": true, "LICENSE.txt": true, "CHANGELOG.md": true}
)

// File is one file a pack puts in a site.
type File struct {
	From string // in the pack
	To   string // in the site
}

// Check reads a pack and says what's wrong with it, or what it would install.
func Check(dir string) (*Manifest, []File, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "pack.yaml"))
	if err != nil {
		return nil, nil, fmt.Errorf("not a pack: no pack.yaml")
	}
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, nil, fmt.Errorf("pack.yaml: %w", err)
	}
	if !nameRe.MatchString(m.Name) {
		return nil, nil, fmt.Errorf("pack.yaml: name should be lowercase letters, digits and dashes")
	}
	if m.Version == "" {
		return nil, nil, fmt.Errorf("pack.yaml: version is required")
	}
	var files []File
	var problems []string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			problems = append(problems, rel+": a symbolic link")
			return nil
		}
		if metaFiles[rel] || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		top, _, _ := strings.Cut(rel, "/")
		ext := strings.ToLower(path.Ext(rel))
		switch {
		case (top == "sections" || top == "partials" || top == "layouts") && ext == ".html" && !strings.Contains(strings.TrimPrefix(rel, top+"/"), "/"):
			src, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if m := codeInTemplate.FindString(string(src)); m != "" {
				problems = append(problems, fmt.Sprintf("%s: %q: a pack can't carry code that runs in visitors' browsers", rel, strings.TrimSpace(m)))
				return nil
			}
			files = append(files, File{From: rel, To: rel})
		case top == "assets" && assetExt[ext]:
			files = append(files, File{From: rel, To: path.Join("assets", "packs", m.Name, strings.TrimPrefix(rel, "assets/"))})
		case ext == ".js" || ext == ".mjs":
			problems = append(problems, rel+": JavaScript; a pack can't carry code")
		case ext == ".svg":
			problems = append(problems, rel+": SVG, which can carry scripts; use PNG or WebP")
		default:
			problems = append(problems, rel+": not something a pack can install (sections/, partials/, layouts/ as .html; assets/ as stylesheets, pictures or fonts)")
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, nil, fmt.Errorf("the pack can't be installed:\n  %s", strings.Join(problems, "\n  "))
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("the pack installs nothing")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].To < files[j].To })
	return &m, files, nil
}

// fetch makes a local copy of a pack: a folder as it is, or a git repository
// (url, or url@ref) cloned at that point. It returns the folder and the commit.
func fetch(src, tmp string) (string, string, error) {
	if fi, err := os.Stat(src); err == nil && fi.IsDir() {
		return src, "", nil
	}
	repo, ref := src, ""
	if i := strings.LastIndex(src, "@"); i > strings.Index(src, "://")+3 && !strings.Contains(src[i:], "/") {
		repo, ref = src[:i], src[i+1:]
	}
	if !strings.HasPrefix(repo, "https://") && !strings.HasPrefix(repo, "ssh://") && !strings.HasPrefix(repo, "git@") {
		return "", "", fmt.Errorf("%s is neither a folder nor a git repository address", src)
	}
	dir := filepath.Join(tmp, "pack")
	args := []string{"clone", "--quiet", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	cmd := exec.Command("git", append(args, repo, dir)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git clone %s: %s", src, strings.TrimSpace(string(out)))
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", "", err
	}
	return dir, strings.TrimSpace(string(out)), nil
}

// Add installs a pack into a site, or upgrades it. A file already in the
// site that the pack didn't put there is never overwritten, unless force.
func Add(siteDir, src string, force bool) (*site.PackRef, error) {
	tmp, err := os.MkdirTemp("", "hotdog-cms-pack-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	dir, commit, err := fetch(src, tmp)
	if err != nil {
		return nil, err
	}
	m, files, err := Check(dir)
	if err != nil {
		return nil, err
	}
	cfg, err := site.LoadConfig(siteDir)
	if err != nil {
		return nil, err
	}
	mine := map[string]bool{} // files an older version of this pack put here
	var others []site.PackRef
	for _, p := range cfg.Packs {
		if p.Name == m.Name {
			for _, f := range p.Files {
				mine[f] = true
			}
			continue
		}
		others = append(others, p)
		for _, f := range p.Files {
			for _, nf := range files {
				if nf.To == f {
					return nil, fmt.Errorf("%s is already installed by the %s pack", f, p.Name)
				}
			}
		}
	}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(siteDir, filepath.FromSlash(f.To))); err == nil && !mine[f.To] && !force {
			return nil, fmt.Errorf("%s is already in the site (not from this pack); move it, or add with -force to replace it", f.To)
		}
	}
	ref := site.PackRef{Name: m.Name, Version: m.Version, Source: src, Commit: commit}
	if fi, err := os.Stat(src); err == nil && fi.IsDir() {
		ref.Source = "" // a local folder's path means nothing to anyone else
	}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.From)))
		if err != nil {
			return nil, err
		}
		dst := filepath.Join(siteDir, filepath.FromSlash(f.To))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, err
		}
		ref.Files = append(ref.Files, f.To)
		delete(mine, f.To)
	}
	for f := range mine { // dropped by the new version
		_ = os.Remove(filepath.Join(siteDir, filepath.FromSlash(f)))
	}
	return &ref, writeRefs(siteDir, append(others, ref))
}

// Remove takes a pack's files out of a site and forgets it.
func Remove(siteDir, name string) error {
	cfg, err := site.LoadConfig(siteDir)
	if err != nil {
		return err
	}
	var keep []site.PackRef
	found := false
	for _, p := range cfg.Packs {
		if p.Name != name {
			keep = append(keep, p)
			continue
		}
		found = true
		for _, f := range p.Files {
			_ = os.Remove(filepath.Join(siteDir, filepath.FromSlash(f)))
		}
	}
	if !found {
		return fmt.Errorf("no pack %q in this site", name)
	}
	return writeRefs(siteDir, keep)
}

func writeRefs(siteDir string, refs []site.PackRef) error {
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	p := filepath.Join(siteDir, "site.yaml")
	raw, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var v any
	if len(refs) > 0 {
		v = refs
	}
	out, err := siteyaml.SetTopLevel(string(raw), "packs", v)
	if err != nil {
		return err
	}
	return os.WriteFile(p, []byte(out), 0o644)
}
