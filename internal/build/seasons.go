package build

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/seasons"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// addSeasons checks site.yaml's seasons: block and writes the script, its
// stylesheet and the props, when seasons are on.
func addSeasons(siteDir string, cfg *site.Config, a *Assets, out string) error {
	c := cfg.Seasons
	if !c.On() {
		return nil
	}
	for _, list := range [][]string{c.Only, c.Except} {
		for _, id := range list {
			if !seasons.Known(id) {
				return fmt.Errorf("site.yaml: seasons: no holiday called %q (they are: %s)", id, strings.Join(seasons.IDs, ", "))
			}
		}
	}
	// A site's own art needs a drawing for every prop it might show.
	for _, art := range c.Art {
		var missing []string
		for _, p := range seasons.PropNames() {
			rel := strings.ReplaceAll(strings.TrimPrefix(art.Art, "/"), "{prop}", p)
			if _, err := os.Stat(filepath.Join(siteDir, "static", filepath.FromSlash(rel))); err != nil {
				missing = append(missing, "static/"+rel)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("site.yaml: seasons: art for %q is missing %d of %d drawings: %s", art.Image, len(missing), len(seasons.Props), strings.Join(missing, ", "))
		}
	}
	if out != "" {
		if err := os.MkdirAll(filepath.Join(out, "hotdog-seasons"), 0o755); err != nil {
			return err
		}
	}
	if err := a.add(out, "hotdog-seasons.js", seasons.Script); err != nil {
		return err
	}
	if err := a.add(out, "hotdog-seasons.css", seasons.Style); err != nil {
		return err
	}
	for name, svg := range seasons.Props {
		if err := a.add(out, "hotdog-seasons/"+name+".svg", svg); err != nil {
			return err
		}
	}
	return nil
}

// seasonsTags is {{ seasons }}: the stylesheet and script tags, or nothing
// when seasons are off.
func seasonsTags(cfg *site.Config, a *Assets) (template.HTML, error) {
	c := cfg.Seasons
	if !c.On() {
		return "", nil
	}
	o := seasons.Options{Anchor: c.Anchor, Logo: c.Logo, Skip: c.Skip, Only: c.Only, Except: c.Except,
		Greetings: c.Greetings == nil || *c.Greetings, PropURLs: map[string]string{}}
	o.ScriptURL, _ = a.URL("hotdog-seasons.js")
	o.CSSURL, _ = a.URL("hotdog-seasons.css")
	for name := range seasons.Props {
		o.PropURLs[name], _ = a.URL("hotdog-seasons/" + name + ".svg")
	}
	for _, art := range c.Art {
		o.Art = append(o.Art, [2]string{art.Image, strings.ReplaceAll(art.Art, "{prop}", "{p}")})
	}
	return seasons.Tags(o)
}
