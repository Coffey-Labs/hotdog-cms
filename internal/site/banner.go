package site

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Banner is a notice across the top of the site: holiday hours, a closure,
// an offer. Banners live in data/banners.yaml, which the editor's quick
// posts write, and show only between from and until, if given.
//
//   - text: Closed Thursday for Thanksgiving.
//     link: /news/holiday-hours/
//     link_text: Holiday hours
//     tone: alert          # info (the default) or alert
//     show: home           # all (the default) or home
//     from: 2026-11-20 00:00
//     until: 2026-11-27 00:00
type Banner struct {
	Text     string `yaml:"text" json:"text"`
	Link     string `yaml:"link,omitempty" json:"link,omitempty"`
	LinkText string `yaml:"link_text,omitempty" json:"linkText,omitempty"`
	Tone     string `yaml:"tone,omitempty" json:"tone,omitempty"`
	Show     string `yaml:"show,omitempty" json:"show,omitempty"`
	From     string `yaml:"from,omitempty" json:"from,omitempty"`
	Until    string `yaml:"until,omitempty" json:"until,omitempty"`

	Start, End time.Time `yaml:"-" json:"-"` // from and until, read in the site's time zone
}

// BannersFile is where banners are kept, in the site's folder.
const BannersFile = "data/banners.yaml"

// MaxBannerText keeps a banner a banner: one or two lines.
const MaxBannerText = 280

// LoadBanners reads data/banners.yaml, if there is one.
func LoadBanners(dir string, loc *time.Location) ([]Banner, error) {
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(BannersFile)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseBanners(raw, loc)
}

// ParseBanners reads and checks a banners file.
func ParseBanners(raw []byte, loc *time.Location) ([]Banner, error) {
	var bs []Banner
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&bs); err != nil && !errors.Is(err, io.EOF) { // io.EOF: an empty file
		return nil, fmt.Errorf("%s: %w", BannersFile, err)
	}
	for i := range bs {
		if err := bs[i].Resolve(loc); err != nil {
			return nil, fmt.Errorf("%s: banner %d: %w", BannersFile, i+1, err)
		}
	}
	return bs, nil
}

// Resolve checks a banner and reads its times.
func (b *Banner) Resolve(loc *time.Location) error {
	b.Text = strings.TrimSpace(b.Text)
	switch {
	case b.Text == "":
		return fmt.Errorf("text is empty")
	case len([]rune(b.Text)) > MaxBannerText:
		return fmt.Errorf("text is over %d characters; put the rest on a page and link to it", MaxBannerText)
	case strings.ContainsAny(b.Text, "\n\r"):
		return fmt.Errorf("text should be one line")
	}
	switch b.Tone {
	case "", "info", "alert":
	default:
		return fmt.Errorf("tone %q should be info or alert", b.Tone)
	}
	switch b.Show {
	case "", "all", "home":
	default:
		return fmt.Errorf("show %q should be all or home", b.Show)
	}
	if b.Link != "" && !strings.HasPrefix(b.Link, "/") && !strings.HasPrefix(b.Link, "https://") && !strings.HasPrefix(b.Link, "http://") {
		return fmt.Errorf("link %q should be a page on the site (/news/) or a web address", b.Link)
	}
	var err error
	if b.From != "" {
		if b.Start, _, err = eventTime(b.From, loc); err != nil {
			return fmt.Errorf("from: %w", err)
		}
	}
	if b.Until != "" {
		if b.End, _, err = eventTime(b.Until, loc); err != nil {
			return fmt.Errorf("until: %w", err)
		}
		if !b.Start.IsZero() && !b.End.After(b.Start) {
			return fmt.Errorf("until is before from")
		}
	}
	return nil
}

// Active says whether the banner shows at t.
func (b *Banner) Active(t time.Time) bool {
	return (b.Start.IsZero() || !t.Before(b.Start)) && (b.End.IsZero() || t.Before(b.End))
}

// Expired says whether the banner is over for good at t.
func (b *Banner) Expired(t time.Time) bool { return !b.End.IsZero() && !t.Before(b.End) }

// BannersFor is the banners showing on a page now.
func (s *Site) BannersFor(p *Page) []Banner {
	var out []Banner
	for _, b := range s.Banners {
		if b.Active(s.Now) && (b.Show != "home" || (p != nil && p.IsHome())) {
			out = append(out, b)
		}
	}
	return out
}
