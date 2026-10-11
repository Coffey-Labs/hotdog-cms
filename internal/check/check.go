// Package check reads a built site and reports what would embarrass it in
// production, or break a promise it makes to its visitors: tracking that
// starts before anyone agreed to it, scripts from other sites, pages a
// strict security policy would break, links to nowhere, missing search and
// social metadata, a security.txt past its date. These are the gotchas that
// have bitten real sites, written down so they can't happen twice.
//
// Errors fail the check; warnings are reported. Every rule has an id, which
// site.yaml can switch off (check.ignore) when a site means it.
package check

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Level is how serious a finding is.
type Level string

const (
	Error   Level = "error"
	Warning Level = "warning"
)

// Problem is one finding.
type Problem struct {
	Level Level
	Rule  string
	File  string
	What  string
}

func (p Problem) String() string {
	return fmt.Sprintf("%-7s %-24s %s: %s", p.Level, p.Rule, p.File, p.What)
}

// Report is what a check found, and the Content-Security-Policy the built
// pages would need.
type Report struct {
	Problems []Problem
	CSP      string
}

// Errors counts the findings that fail the check.
func (r *Report) Errors() int {
	n := 0
	for _, p := range r.Problems {
		if p.Level == Error {
			n++
		}
	}
	return n
}

type checker struct {
	siteDir, out string
	cfg          *site.Config
	ignore       map[string]bool
	allow        map[string]bool
	now          time.Time
	report       Report
}

func (c *checker) add(level Level, rule, file, format string, args ...any) {
	if c.ignore[rule] {
		return
	}
	c.report.Problems = append(c.report.Problems, Problem{level, rule, file, fmt.Sprintf(format, args...)})
}

// Run checks the built site in out against the site in siteDir.
func Run(siteDir, out string, cfg *site.Config) (*Report, error) {
	c := &checker{siteDir: siteDir, out: out, cfg: cfg, ignore: map[string]bool{}, allow: map[string]bool{}, now: time.Now()}
	for _, r := range cfg.Check.Ignore {
		c.ignore[r] = true
	}
	for _, h := range cfg.Check.AllowThirdParty {
		c.allow[strings.ToLower(h)] = true
	}
	if err := c.forbidden(); err != nil {
		return nil, err
	}
	pages, err := c.readPages()
	if err != nil {
		return nil, err
	}
	for _, pg := range pages {
		c.checkPage(pg)
	}
	c.sitewide(pages)
	c.images()
	c.look()
	c.report.CSP = policy(pages)
	sort.SliceStable(c.report.Problems, func(i, j int) bool {
		a, b := c.report.Problems[i], c.report.Problems[j]
		if a.Level != b.Level {
			return a.Level == Error
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.File < b.File
	})
	return &c.report, nil
}

// forbidden looks for the site's banned strings in what people write as well
// as in what gets built: a template or data file can leak a host name as
// easily as a page. site.yaml is left out because it is where the patterns
// are written.
func (c *checker) forbidden() error {
	var res []*regexp.Regexp
	for _, f := range c.cfg.Check.Forbid {
		re, err := regexp.Compile(f)
		if err != nil {
			return fmt.Errorf("site.yaml check.forbid %q: %w", f, err)
		}
		res = append(res, re)
	}
	if len(res) == 0 {
		return nil
	}
	scan := func(root, label string) {
		_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !textFile(p) {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(c.siteDir, p)
			if label != "" {
				rel, _ = filepath.Rel(c.out, p)
				rel = label + filepath.ToSlash(rel)
			}
			for _, re := range res {
				if loc := re.FindIndex(data); loc != nil {
					line := 1 + strings.Count(string(data[:loc[0]]), "\n")
					c.add(Error, "forbidden-string", fmt.Sprintf("%s:%d", filepath.ToSlash(rel), line), "matches forbidden pattern %q", re.String())
				}
			}
			return nil
		})
	}
	for _, d := range []string{"content", "data", "layouts", "partials", "sections", "icons", "assets", "static", "forms"} {
		scan(filepath.Join(c.siteDir, d), "")
	}
	scan(c.out, "public/")
	return nil
}

func textFile(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".md", ".html", ".htm", ".css", ".js", ".json", ".yaml", ".yml", ".txt", ".xml", ".svg", ".webmanifest":
		return true
	}
	return false
}
