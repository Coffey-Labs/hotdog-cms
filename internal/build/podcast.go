package build

import (
	"encoding/xml"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// writePodcast writes a podcast feed for one collection: RSS 2.0 with the
// iTunes fields podcast apps and directories read, and each episode's audio
// file (audio: in its front matter, a path in static/) as an enclosure with
// its real size. Pages without audio aren't episodes, and are left out.
func writePodcast(s *site.Site, name string, cc site.CollectionConfig, out string) (string, error) {
	type enclosure struct {
		URL    string `xml:"url,attr"`
		Length int64  `xml:"length,attr"`
		Type   string `xml:"type,attr"`
	}
	type guid struct {
		IsPermaLink string `xml:"isPermaLink,attr"`
		Value       string `xml:",chardata"`
	}
	type itImage struct {
		Href string `xml:"href,attr"`
	}
	type item struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		GUID        guid      `xml:"guid"`
		PubDate     string    `xml:"pubDate,omitempty"`
		Description string    `xml:"description,omitempty"`
		Enclosure   enclosure `xml:"enclosure"`
		Duration    string    `xml:"itunes:duration,omitempty"`
		Episode     string    `xml:"itunes:episode,omitempty"`
		Season      string    `xml:"itunes:season,omitempty"`
		Explicit    string    `xml:"itunes:explicit"`
		Image       *itImage  `xml:"itunes:image,omitempty"`
	}
	type category struct {
		Text string `xml:"text,attr"`
	}
	type owner struct {
		Name  string `xml:"itunes:name"`
		Email string `xml:"itunes:email"`
	}
	type atomLink struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
		Type string `xml:"type,attr"`
	}
	type channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		AtomLink    atomLink  `xml:"atom:link"`
		Description string    `xml:"description"`
		Language    string    `xml:"language"`
		Author      string    `xml:"itunes:author,omitempty"`
		Owner       *owner    `xml:"itunes:owner,omitempty"`
		Image       *itImage  `xml:"itunes:image,omitempty"`
		Category    *category `xml:"itunes:category,omitempty"`
		Explicit    string    `xml:"itunes:explicit"`
		Items       []item    `xml:"item"`
	}
	type rss struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Atom    string   `xml:"xmlns:atom,attr"`
		ITunes  string   `xml:"xmlns:itunes,attr"`
		Channel channel  `xml:"channel"`
	}
	feedPath := cc.FeedPath
	if feedPath == "" {
		feedPath = "/" + name + "/feed.xml"
	}
	title := cc.FeedTitle
	if title == "" {
		if l := s.Lists[name]; l != nil && l.Title != "" {
			title = l.Title
		} else {
			title = s.Config.Name
		}
	}
	desc := cc.FeedDescription
	if desc == "" {
		desc = s.Config.Description
	}
	yesNo := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}
	rfc := func(t time.Time) string { return t.UTC().Format("Mon, 02 Jan 2006 15:04:05") + " GMT" }
	ch := channel{
		Title: title, Link: s.Config.URL + s.CollectionPath(name),
		AtomLink:    atomLink{Href: s.Config.URL + feedPath, Rel: "self", Type: "application/rss+xml"},
		Description: desc, Language: strings.ToLower(s.Config.Language),
		Author: s.Config.Author, Explicit: yesNo(cc.PodcastExplicit),
	}
	if cc.PodcastEmail != "" {
		ch.Owner = &owner{Name: s.Config.Author, Email: cc.PodcastEmail}
	}
	if cc.FeedImage != "" {
		ch.Image = &itImage{Href: abs(s, cc.FeedImage)}
	}
	if cc.PodcastCategory != "" {
		ch.Category = &category{Text: cc.PodcastCategory}
	}
	for _, p := range s.Collection(name) {
		if !indexable(p) {
			continue
		}
		audio := str(p.Params["audio"])
		if audio == "" {
			continue
		}
		var length int64
		if strings.HasPrefix(audio, "/") {
			if fi, err := os.Stat(filepath.Join(s.Dir, "static", filepath.FromSlash(path.Clean(audio)))); err == nil {
				length = fi.Size()
			}
		}
		typ := mime.TypeByExtension(strings.ToLower(path.Ext(audio)))
		switch strings.ToLower(path.Ext(audio)) {
		case ".mp3":
			typ = "audio/mpeg"
		case ".m4a":
			typ = "audio/x-m4a"
		}
		if typ == "" {
			typ = "audio/mpeg"
		}
		it := item{Title: p.Title, Link: link(s, p), GUID: guid{"true", link(s, p)}, Description: p.Summary,
			Enclosure: enclosure{URL: abs(s, audio), Length: length, Type: typ},
			Duration:  str(p.Params["duration"]), Episode: str(p.Params["episode"]), Season: str(p.Params["season"]),
			Explicit: yesNo(cc.PodcastExplicit || str(p.Params["explicit"]) == "true")}
		if !p.Date.IsZero() {
			it.PubDate = rfc(p.Date)
		}
		if v := str(p.Params["photo"]); v != "" {
			it.Image = &itImage{Href: abs(s, v)}
		}
		ch.Items = append(ch.Items, it)
	}
	dst := filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(feedPath, "/")))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	return feedPath, writeXML(dst, rss{Version: "2.0", Atom: "http://www.w3.org/2005/Atom", ITunes: "http://www.itunes.com/dtds/podcast-1.0.dtd", Channel: ch})
}
