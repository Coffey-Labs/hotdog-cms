package site

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
	"golang.org/x/text/unicode/norm"
)

// mkdocsIDs gives headings the anchors MkDocs gives them (Python-Markdown's
// toc slugify), so links into a site moved from MkDocs (page.md#a-heading)
// still land: "On Linux & BSD" is on-linux-bsd, not on-linux--bsd, and a
// second heading of the same name is _1, not -1.
type mkdocsIDs struct{ used map[string]bool }

var (
	slugDropRe = regexp.MustCompile(`[^\w\s-]`)
	slugJoinRe = regexp.MustCompile(`[-\s]+`)
	slugNumRe  = regexp.MustCompile(`^(.*)_([0-9]+)$`)
)

// mkdocsSlug is Python-Markdown's slugify: accents folded to ASCII and the
// rest dropped, then anything but letters, digits, _, - and spaces removed,
// lower case, and runs of spaces and dashes made one dash.
func mkdocsSlug(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if r < unicode.MaxASCII {
			b.WriteRune(r)
		}
	}
	v := strings.ToLower(strings.TrimSpace(slugDropRe.ReplaceAllString(b.String(), "")))
	return slugJoinRe.ReplaceAllString(v, "-")
}

func (m *mkdocsIDs) Generate(value []byte, _ ast.NodeKind) []byte {
	id := mkdocsSlug(string(value))
	// Python-Markdown's unique(): an empty or taken id gets _1, then _2...
	for id == "" || m.used[id] {
		if mt := slugNumRe.FindStringSubmatch(id); mt != nil {
			n, _ := strconv.Atoi(mt[2])
			id = mt[1] + "_" + strconv.Itoa(n+1)
		} else {
			id += "_1"
		}
	}
	m.used[id] = true
	return []byte(id)
}

func (m *mkdocsIDs) Put(value []byte) { m.used[string(value)] = true }
