package editor

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/preview"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Quick posts: the things an operator needs up in minutes. A banner or
// alert across the top of the site, a news post, an event, the words on the
// front page. Each is one form and one button. Posting it makes the same
// commit on a draft, the same review and the same checks as any other
// change, then publishes it straight away if the platform lets this person
// merge. If it doesn't, the review is left open for someone who can.

// QuickInfo is what the quick post forms need to know about a site.
type QuickInfo struct {
	Timezone    string           `json:"timezone"`
	Now         string           `json:"now"` // in the site's time zone: 2026-11-07 18:00
	Banners     []QuickBanner    `json:"banners"`
	Collections []CollectionInfo `json:"collections"` // where a news post can go
	Calendars   []CollectionInfo `json:"calendars"`   // where an event can go
	Commit      string           `json:"commit"`
}

// QuickBanner is a banner as the form shows it.
type QuickBanner struct {
	site.Banner
	Showing bool `json:"showing"`
	Expired bool `json:"expired"`
}

func (s *Server) apiQuick(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	_, _, siteDir, commit, ok := s.siteFor(w, r, sess, f, id)
	if !ok {
		return
	}
	cfg, err := site.LoadConfig(siteDir)
	if err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't read site.yaml: "+trimErr(err))
		return
	}
	loc, err := cfg.Location()
	if err != nil {
		apiError(w, http.StatusBadGateway, err.Error())
		return
	}
	now := time.Now().In(loc)
	info := QuickInfo{Timezone: loc.String(), Now: now.Format("2006-01-02 15:04"), Banners: []QuickBanner{}, Collections: []CollectionInfo{}, Calendars: []CollectionInfo{}, Commit: commit}
	banners, err := site.LoadBanners(siteDir, loc)
	if err != nil {
		apiError(w, http.StatusBadGateway, err.Error())
		return
	}
	for _, b := range banners {
		info.Banners = append(info.Banners, QuickBanner{Banner: b, Showing: b.Active(now), Expired: b.Expired(now)})
	}
	for _, name := range collectionNames(siteDir, cfg) {
		ci := CollectionInfo{Name: name, Title: titleOf(name)}
		if cfg.Collections[name].Calendar {
			info.Calendars = append(info.Calendars, ci)
		} else {
			info.Collections = append(info.Collections, ci)
		}
	}
	writeJSON(w, http.StatusOK, info)
}

// collectionNames lists the site's collections: folders in content/, and
// any configured in site.yaml.
func collectionNames(siteDir string, cfg *site.Config) []string {
	seen := map[string]bool{}
	entries, _ := os.ReadDir(filepath.Join(siteDir, "content"))
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			seen[e.Name()] = true
		}
	}
	for name := range cfg.Collections {
		seen[name] = true
	}
	var out []string
	for name := range seen {
		if slug(name) == name {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func titleOf(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + strings.ReplaceAll(name[1:], "-", " ")
}

type quickRequest struct {
	Kind    string `json:"kind"`    // banner, banner-remove, post, event, home
	Publish bool   `json:"publish"` // publish now if allowed, rather than leave it for review

	// banner
	Text     string `json:"text"`
	Link     string `json:"link"`
	LinkText string `json:"linkText"`
	Tone     string `json:"tone"`
	Show     string `json:"show"`
	From     string `json:"from"`
	Until    string `json:"until"`
	Index    int    `json:"index"` // banner-remove: which, with Text to be sure

	// post, event
	Collection string `json:"collection"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Body       string `json:"body"`
	Date       string `json:"date"`    // event: 2026-11-07
	Start      string `json:"start"`   // event: 18:00, or empty for all day
	End        string `json:"end"`     // event: 20:00
	EndDate    string `json:"endDate"` // event: the last day, if it runs over several
	Location   string `json:"location"`
	EventLink  string `json:"eventLink"`

	// home
	Edit       *Edit  `json:"edit"`
	BaseCommit string `json:"baseCommit"`
}

var (
	dateRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	clockRe = regexp.MustCompile(`^\d{2}:\d{2}$`)
	// dateTimeRe: 2026-11-07 or 2026-11-07 18:00, safe to write unquoted.
	dateTimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}( \d{2}:\d{2})?$`)
)

func (s *Server) quickPost(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req quickRequest
	if !decode(w, r, &req) {
		return
	}
	_, siteDir, base, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), st.Branch)
	if siteDir == "" {
		s.writeError(w, r, err)
		return
	}
	cfg, err := site.LoadConfig(siteDir)
	if err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't read site.yaml: "+trimErr(err))
		return
	}
	loc, err := cfg.Location()
	if err != nil {
		apiError(w, http.StatusBadGateway, err.Error())
		return
	}
	now := time.Now().In(loc)

	var source, title string
	var content []byte
	switch req.Kind {
	case "banner", "banner-remove":
		source, title = site.BannersFile, "Banner: "+oneLine(req.Text, 60)
		if req.Kind == "banner-remove" {
			title = "Take down banner: " + oneLine(req.Text, 50)
		}
		content, err = s.quickBanners(siteDir, loc, now, &req)
	case "post":
		title = strings.TrimSpace(req.Title)
		source, content, err = quickPage(siteDir, cfg, req.Collection, req.Title, false, func() ([][2]any, error) {
			fm := [][2]any{{"title", strings.TrimSpace(req.Title)}, {"date", now.Format("2006-01-02")}}
			if s := strings.TrimSpace(req.Summary); s != "" {
				fm = append(fm, [2]any{"summary", s})
			}
			return fm, nil
		}, req.Body)
	case "event":
		title = "Event: " + strings.TrimSpace(req.Title)
		source, content, err = quickPage(siteDir, cfg, req.Collection, req.Title, true, func() ([][2]any, error) { return eventFields(&req, loc) }, req.Body)
	case "home":
		source, title = "content/index.md", "Front page"
		if req.Edit == nil {
			err = fmt.Errorf("nothing to change")
			break
		}
		var original []byte
		if original, err = readSource(siteDir, source); err == nil {
			content, err = compose(original, *req.Edit)
		}
		if req.BaseCommit != "" {
			base = req.BaseCommit
		}
	default:
		err = fmt.Errorf("no such kind of quick post")
	}
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}

	target := s.draftName(r, sess, f, st, source)
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), st.Branch, target, true, base, map[string][]byte{repoFile(st, source): content}, title)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Printf("%s quick post (%s) %s on %s@%s (%s)", sess.Login, req.Kind, source, st.ID, target, shortSHA(commit))
	out := map[string]any{"branch": target, "commit": commit, "source": source, "preview": st.Preview.url(preview.Slug(target))}

	_, draftDir, draftCommit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), target)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	res := s.ws.buildAndCheck(draftDir, draftCommit)
	rv, err := s.openReview(r.Context(), f, sess.Token, st, target, "Quick post: "+title, reviewBody(st, target, res, "", sess.Login))
	if err != nil {
		out["message"] = "Saved on a draft, " + target + ", but opening its review didn't work: " + trimErr(err)
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["review"] = map[string]any{"number": rv.Number, "url": rv.URL}
	switch {
	case res.Failed != "" || res.Errors > 0:
		out["problems"] = res.Problems
		out["message"] = fmt.Sprintf("Not published: the checks found %d error(s). Review #%d is open; fix them there or in the editor.", res.Errors, rv.Number)
		if res.Failed != "" {
			out["message"] = "Not published: the site doesn't build with this change. " + res.Failed
		}
	case !req.Publish:
		out["message"] = fmt.Sprintf("Sent for review: #%d.", rv.Number)
	default:
		if err := s.merge(r.Context(), f, sess.Token, st, rv.Number); err != nil {
			out["message"] = fmt.Sprintf("Review #%d is open, but it couldn't be published from here (%s). Someone who can publish on this site can merge it.", rv.Number, trimErr(err))
			break
		}
		s.log.Printf("%s published quick post %s@%s (review #%d)", sess.Login, st.ID, target, rv.Number)
		out["published"] = true
		out["message"] = "Posted. The live site updates when its publishing pipeline runs."
	}
	writeJSON(w, http.StatusOK, out)
}

// quickBanners writes data/banners.yaml with the change: a banner added, or
// one taken down. Banners that are over are tidied away at the same time.
func (s *Server) quickBanners(siteDir string, loc *time.Location, now time.Time, req *quickRequest) ([]byte, error) {
	existing, err := site.LoadBanners(siteDir, loc)
	if err != nil {
		return nil, err
	}
	var keep []site.Banner
	for i, b := range existing {
		if req.Kind == "banner-remove" && i == req.Index {
			if b.Text != strings.TrimSpace(req.Text) {
				return nil, fmt.Errorf("the banners changed since this page loaded; reload and try again")
			}
			continue
		}
		if !b.Expired(now) {
			keep = append(keep, b)
		}
	}
	if req.Kind == "banner-remove" && len(keep) == len(existing) {
		return nil, fmt.Errorf("that banner isn't there any more")
	}
	if req.Kind == "banner" {
		b := site.Banner{Text: req.Text, Link: strings.TrimSpace(req.Link), LinkText: strings.TrimSpace(req.LinkText), Tone: req.Tone, Show: req.Show, From: strings.TrimSpace(req.From), Until: strings.TrimSpace(req.Until)}
		if b.Tone == "info" {
			b.Tone = ""
		}
		if b.Show == "all" {
			b.Show = ""
		}
		if b.Link == "" {
			b.LinkText = ""
		}
		if err := b.Resolve(loc); err != nil {
			return nil, err
		}
		if !b.End.IsZero() && !b.End.After(now) {
			return nil, fmt.Errorf("until is already past")
		}
		keep = append(keep, b)
	}
	var buf bytes.Buffer
	buf.WriteString("# Notices across the top of the site, newest last. The editor's quick\n# posts write this file; times are in the site's timezone.\n")
	if len(keep) == 0 {
		buf.WriteString("[]\n")
		return buf.Bytes(), nil
	}
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(keep); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// quickPage writes a new page in a collection, with its front matter in the
// order given.
func quickPage(siteDir string, cfg *site.Config, collection, title string, calendar bool, fields func() ([][2]any, error), body string) (string, []byte, error) {
	title = strings.TrimSpace(title)
	if title == "" || slug(title) == "" {
		return "", nil, fmt.Errorf("give it a title with some letters or numbers in it")
	}
	if collection == "" || slug(collection) != collection {
		return "", nil, fmt.Errorf("choose where it goes")
	}
	known := false
	for _, n := range collectionNames(siteDir, cfg) {
		known = known || n == collection
	}
	if !known || cfg.Collections[collection].Calendar != calendar {
		return "", nil, fmt.Errorf("no such place for it: %s", collection)
	}
	fm, err := fields()
	if err != nil {
		return "", nil, err
	}
	name := slug(title)
	if len(name) > 60 {
		name = strings.Trim(name[:60], "-")
	}
	source := path.Join("content", collection, name+".md")
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(siteDir, filepath.FromSlash(source))); err != nil {
			break
		}
		source = path.Join("content", collection, fmt.Sprintf("%s-%d.md", name, i))
	}
	doc := &yaml.Node{Kind: yaml.MappingNode}
	for _, kv := range fm {
		var k, v yaml.Node
		k.SetString(kv[0].(string))
		if str, ok := kv[1].(string); ok && dateTimeRe.MatchString(str) {
			// Dates and times written plainly, as people write them by hand.
			v = yaml.Node{Kind: yaml.ScalarNode, Value: str}
		} else if err := v.Encode(kv[1]); err != nil {
			return "", nil, err
		}
		doc.Content = append(doc.Content, &k, &v)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", nil, err
	}
	buf.WriteString("---\n")
	if b := strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n")); b != "" {
		buf.WriteString(b + "\n")
	}
	return source, buf.Bytes(), nil
}

// eventFields turns the event form into front matter, checking it the way
// the build will.
func eventFields(req *quickRequest, loc *time.Location) ([][2]any, error) {
	if !dateRe.MatchString(req.Date) {
		return nil, fmt.Errorf("give the event a date")
	}
	start, end := req.Date, ""
	switch {
	case req.Start != "":
		if !clockRe.MatchString(req.Start) || (req.End != "" && !clockRe.MatchString(req.End)) {
			return nil, fmt.Errorf("times should be like 18:00")
		}
		start = req.Date + " " + req.Start
		if req.End != "" {
			endDay := req.Date
			if req.EndDate != "" {
				endDay = req.EndDate
			}
			end = endDay + " " + req.End
		}
	case req.EndDate != "" && req.EndDate != req.Date:
		end = req.EndDate
	}
	if req.EndDate != "" && !dateRe.MatchString(req.EndDate) {
		return nil, fmt.Errorf("the last day should be a date")
	}
	fm := [][2]any{{"title", strings.TrimSpace(req.Title)}}
	if s := strings.TrimSpace(req.Summary); s != "" {
		fm = append(fm, [2]any{"summary", s})
	}
	fm = append(fm, [2]any{"start", start})
	if end != "" {
		fm = append(fm, [2]any{"end", end})
	}
	if l := strings.TrimSpace(req.Location); l != "" {
		fm = append(fm, [2]any{"location", l})
	}
	if l := strings.TrimSpace(req.EventLink); l != "" {
		fm = append(fm, [2]any{"link", l})
	}
	// Check it as the build will, so a mistake shows here, not as a failed
	// publish.
	params := map[string]any{}
	for _, kv := range fm {
		params[kv[0].(string)] = kv[1]
	}
	if err := site.CheckEvent(params, loc); err != nil {
		return nil, err
	}
	return fm, nil
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
