package build

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template/parse"

	"gopkg.in/yaml.v3"
)

// The section library: every section a site's pages can stack, with the
// fields each takes, so the editor can offer sections as cards and their
// fields as forms. A section template declares its fields in a comment at
// the top:
//
//	{{/* section
//	label: Hero
//	about: The big opening at the top of a page.
//	fields:
//	  - { name: heading, type: text, required: true }
//	  - { name: text, type: markdown }
//	  - name: buttons
//	    type: list
//	    fields:
//	      - { name: text, type: text }
//	      - { name: href, type: link }
//	      - { name: primary, type: bool, label: Main button }
//	*/}}
//
// A template without that comment still works: its fields are read from what
// the template uses (.Data.heading, range .Data.buttons, markdownify ...).

// SectionInfo is one section type.
type SectionInfo struct {
	Name     string  `json:"name"`
	Label    string  `json:"label"`
	About    string  `json:"about,omitempty"`
	Fields   []Field `json:"fields"`
	Declared bool    `json:"declared"`          // false: fields read from the template
	Problem  string  `json:"problem,omitempty"` // the declaration couldn't be read
}

// Field is one field of a section, or of an item in a list field.
type Field struct {
	Name     string   `json:"name" yaml:"name"`
	Label    string   `json:"label,omitempty" yaml:"label"`
	Type     string   `json:"type" yaml:"type"`
	Help     string   `json:"help,omitempty" yaml:"help"`
	Required bool     `json:"required,omitempty" yaml:"required"`
	Default  any      `json:"default,omitempty" yaml:"default"`
	Options  []string `json:"options,omitempty" yaml:"options"`
	Fields   []Field  `json:"fields,omitempty" yaml:"fields"`
}

// FieldTypes are the kinds of field a form can show.
var FieldTypes = map[string]string{
	"text":       "one line of text",
	"textarea":   "several lines of plain text",
	"markdown":   "Markdown",
	"link":       "an address, on the site or elsewhere",
	"image":      "a picture from media/",
	"bool":       "yes or no",
	"number":     "a number",
	"select":     "one of options",
	"list":       "a list of items, each with fields (or of text, without fields)",
	"group":      "a set of fields",
	"data":       "a file in data/",
	"collection": "a collection, such as articles",
}

var declRe = regexp.MustCompile(`(?s)^\s*\{\{-?\s*/\*\s*section\b(.*?)\*/\s*-?\}\}`)

// SectionLibrary reads every section in a site's sections/ folder.
func SectionLibrary(siteDir string) ([]SectionInfo, error) {
	files, err := htmlFiles(filepath.Join(siteDir, "sections"))
	if err != nil {
		return nil, err
	}
	partials := map[string]*parse.Tree{}
	pfiles, _ := htmlFiles(filepath.Join(siteDir, "partials"))
	for _, name := range pfiles {
		src, err := os.ReadFile(filepath.Join(siteDir, "partials", name+".html"))
		if err != nil {
			return nil, err
		}
		t := parse.New(name)
		t.Mode = parse.SkipFuncCheck
		_, _ = t.Parse(string(src), "", "", partials)
	}
	out := []SectionInfo{}
	for _, name := range files {
		src, err := os.ReadFile(filepath.Join(siteDir, "sections", name+".html"))
		if err != nil {
			return nil, err
		}
		out = append(out, describeSection(name, string(src), partials))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out, nil
}

func describeSection(name, src string, partials map[string]*parse.Tree) SectionInfo {
	info := SectionInfo{Name: name, Label: labelFor(name)}
	if m := declRe.FindStringSubmatch(src); m != nil {
		var decl struct {
			Label  string  `yaml:"label"`
			About  string  `yaml:"about"`
			Fields []Field `yaml:"fields"`
		}
		if err := yaml.Unmarshal([]byte(m[1]), &decl); err != nil {
			info.Problem = "the section comment isn't valid YAML: " + err.Error()
		} else if err := checkFields(decl.Fields, ""); err != nil {
			info.Problem = err.Error()
		} else {
			if decl.Label != "" {
				info.Label = decl.Label
			}
			info.About = strings.TrimSpace(decl.About)
			info.Fields = decl.Fields
			info.Declared = true
			return info
		}
	}
	info.Fields = inferFields(name, src, partials)
	return info
}

func checkFields(fs []Field, in string) error {
	seen := map[string]bool{}
	for i := range fs {
		f := &fs[i]
		where := in + f.Name
		if f.Name == "" {
			return fmt.Errorf("a field in %q has no name", strings.TrimSuffix(in, "."))
		}
		if seen[f.Name] {
			return fmt.Errorf("field %s is declared twice", where)
		}
		seen[f.Name] = true
		if f.Type == "" {
			f.Type = "text"
		}
		if _, ok := FieldTypes[f.Type]; !ok {
			return fmt.Errorf("field %s: type %q isn't one of text, textarea, markdown, link, image, bool, number, select, list, group, data, collection", where, f.Type)
		}
		if f.Type == "select" && len(f.Options) == 0 {
			return fmt.Errorf("field %s: a select needs options", where)
		}
		if len(f.Fields) > 0 && f.Type != "list" && f.Type != "group" {
			return fmt.Errorf("field %s: only a list or a group has fields", where)
		}
		if err := checkFields(f.Fields, where+"."); err != nil {
			return err
		}
	}
	return nil
}

func labelFor(name string) string {
	s := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// inferFields reads the fields a section template uses. It understands the
// usual shapes: .Data.x, $.Data.x, a variable holding .Data or .Data.x, with
// and range over a field (a group or a list), markdownify (Markdown), default
// with a number (a number), a field only ever tested (yes or no), and names
// that say what they hold (href, image).
func inferFields(name, src string, partials map[string]*parse.Tree) []Field {
	t := parse.New(name)
	t.Mode = parse.SkipFuncCheck // the site's functions aren't known here
	trees := map[string]*parse.Tree{}
	if _, err := t.Parse(src, "", "", trees); err != nil {
		return []Field{}
	}
	root := &node{f: Field{Type: "group"}}
	r := &reader{vars: map[string]*node{}, partials: partials}
	for _, t := range trees {
		if t.Root != nil {
			r.walk(t.Root, nil, root)
		}
	}
	return root.fields()
}

// node is a field while it's being worked out; pointers stay put as fields
// are added, so variables can hold them.
type node struct {
	f             Field
	kids          []*node
	printed, cond bool
}

func (n *node) child(name string) *node {
	for _, k := range n.kids {
		if k.f.Name == name {
			return k
		}
	}
	k := &node{f: Field{Name: name, Type: "text", Label: labelFor(name)}}
	n.kids = append(n.kids, k)
	return k
}

// fields settles each field's type from how it was used and what it's
// called, and returns them as Fields.
func (n *node) fields() []Field {
	out := []Field{}
	for _, k := range n.kids {
		f := k.f
		if f.Type == "text" && len(k.kids) == 0 {
			switch {
			case k.cond && !k.printed:
				f.Type = "bool"
			case f.Name == "href" || f.Name == "url" || f.Name == "link":
				f.Type = "link"
			case f.Name == "image" || f.Name == "src" || f.Name == "logo" || strings.HasSuffix(f.Name, "_image"):
				f.Type = "image"
			}
		}
		if len(k.kids) > 0 {
			f.Fields = k.fields()
		}
		out = append(out, f)
	}
	return out
}

func (n *node) markUsed() {
	n.printed = true
	for _, k := range n.kids {
		k.markUsed()
	}
}

type reader struct {
	vars     map[string]*node
	partials map[string]*parse.Tree // partials/, followed into with what they're handed
	depth    int
}

// resolve maps an argument to the field it reads, if it reads one. dot is the
// field the template's dot currently is (nil: the section's own context).
// The root itself comes back for .Data, so a variable can hold it.
func (r *reader) resolve(a parse.Node, dot, root *node) *node {
	var path []string
	base := root
	switch a := a.(type) {
	case *parse.FieldNode:
		path = a.Ident
		switch {
		case dot != nil:
			base = dot
		case len(path) > 0 && path[0] == "Data":
			path = path[1:]
		default:
			return nil
		}
	case *parse.VariableNode:
		switch {
		case a.Ident[0] == "$":
			if len(a.Ident) < 2 || a.Ident[1] != "Data" {
				return nil
			}
			path = a.Ident[2:]
		case r.vars[a.Ident[0]] != nil:
			base, path = r.vars[a.Ident[0]], a.Ident[1:]
		default:
			return nil
		}
	case *parse.DotNode:
		return dot
	default:
		return nil
	}
	n := base
	for _, p := range path {
		if n.f.Type != "list" {
			n.f.Type = "group"
		}
		n = n.child(p)
	}
	return n
}

// pipe notes every field a pipeline reads. It returns the field when the
// pipeline is nothing but that field, which is what with and range need.
func (r *reader) pipe(p *parse.PipeNode, dot, root *node, cond bool) *node {
	if p == nil {
		return nil
	}
	var direct *node
	for _, cmd := range p.Cmds {
		fn := ""
		if len(cmd.Args) > 0 {
			if id, ok := cmd.Args[0].(*parse.IdentifierNode); ok {
				fn = id.Ident
			}
		}
		for i, a := range cmd.Args {
			if sub, ok := a.(*parse.PipeNode); ok {
				r.pipe(sub, dot, root, false)
				continue
			}
			n := r.resolve(a, dot, root)
			if n == nil {
				continue
			}
			if len(p.Cmds) == 1 && len(cmd.Args) == 1 {
				direct = n
			}
			if n == root {
				continue
			}
			switch {
			case fn == "markdownify":
				n.f.Type = "markdown"
			case fn == "default" && i == 2:
				if num, ok := cmd.Args[1].(*parse.NumberNode); ok {
					n.f.Type, n.f.Default = "number", num.Text
				} else if str, ok := cmd.Args[1].(*parse.StringNode); ok {
					n.f.Default = str.Text
				}
			}
			if cond && fn == "" {
				n.cond = true
			} else {
				n.printed = true
			}
		}
	}
	if len(p.Decl) == 1 && !p.IsAssign && direct != nil {
		r.vars[p.Decl[0].Ident[0]] = direct
	}
	if direct == root {
		return nil
	}
	return direct
}

func (r *reader) walk(n parse.Node, dot, root *node) {
	switch x := n.(type) {
	case *parse.ListNode:
		if x == nil {
			return
		}
		for _, c := range x.Nodes {
			r.walk(c, dot, root)
		}
	case *parse.ActionNode:
		r.pipe(x.Pipe, dot, root, false)
	case *parse.TemplateNode:
		// Handed to a partial: whatever it holds is used there, so nothing
		// in it is a yes-or-no just because it was tested first.
		if x.Pipe != nil && len(x.Pipe.Cmds) == 1 && len(x.Pipe.Cmds[0].Args) == 1 {
			if n := r.resolve(x.Pipe.Cmds[0].Args[0], dot, root); n != nil {
				if t := r.partials[x.Name]; t != nil && t.Root != nil && r.depth < 4 {
					r.depth++
					r.walk(t.Root, n, root)
					r.depth--
					return
				}
				n.markUsed()
			}
		}
		r.pipe(x.Pipe, dot, root, false)
	case *parse.IfNode:
		r.pipe(x.Pipe, dot, root, true)
		r.walk(x.List, dot, root)
		r.walk(x.ElseList, dot, root)
	case *parse.WithNode:
		inner := dot
		if f := r.pipe(x.Pipe, dot, root, false); f != nil {
			inner = f
		}
		r.walk(x.List, inner, root)
		r.walk(x.ElseList, dot, root)
	case *parse.RangeNode:
		inner := dot
		if f := r.pipe(x.Pipe, dot, root, false); f != nil {
			f.f.Type = "list"
			inner = f
		}
		r.walk(x.List, inner, root)
		r.walk(x.ElseList, dot, root)
	}
}
