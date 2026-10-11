package editor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Composing a page file from an edit. Front matter changes are applied to the
// original YAML's node tree, so keys the form didn't touch keep their order,
// formatting and comments; only the fields that changed are rewritten.

// splitSource returns a page file's front matter (without the --- lines)
// and its body.
func splitSource(raw []byte) (string, string) {
	s := strings.TrimPrefix(string(raw), "\ufeff")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if strings.HasPrefix(s, "---\n") {
		rest := s[4:]
		if i := strings.Index(rest, "\n---"); i >= 0 {
			body := strings.TrimPrefix(rest[i+4:], "\n")
			return rest[:i], body
		}
		if strings.HasPrefix(rest, "---") { // empty front matter
			return "", strings.TrimPrefix(rest[3:], "\n")
		}
	}
	return "", s
}

// Edit is what the editor sends to save or preview a page.
type Edit struct {
	// Set changes front matter fields: a value replaces the field (or adds
	// it), null removes it. Values stay raw JSON until they're written, so an
	// object's keys keep the order they came in (a section's fields, say).
	Set map[string]json.RawMessage `json:"set,omitempty"`
	// FrontMatter, if not nil, replaces the whole front matter with this
	// YAML (the "edit as YAML" view). Set is then ignored.
	FrontMatter *string `json:"frontMatter,omitempty"`
	Body        string  `json:"body"`
}

// compose builds the new file from the original and an edit.
func compose(original []byte, e Edit) ([]byte, error) {
	fmText, _ := splitSource(original)
	var out string
	switch {
	case e.FrontMatter != nil:
		var check map[string]any
		if err := yaml.Unmarshal([]byte(*e.FrontMatter), &check); err != nil {
			return nil, fmt.Errorf("front matter isn't valid YAML: %w", err)
		}
		out = strings.TrimRight(*e.FrontMatter, "\n")
	default:
		var err error
		if out, err = applySet(fmText, e.Set); err != nil {
			return nil, err
		}
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	if out != "" {
		b.WriteString(out)
		b.WriteString("\n")
	}
	b.WriteString("---\n")
	body := strings.ReplaceAll(e.Body, "\r\n", "\n")
	if body != "" {
		if !strings.HasPrefix(body, "\n") {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimRight(body, "\n"))
		b.WriteString("\n")
	}
	return b.Bytes(), nil
}

func applySet(fmText string, set map[string]json.RawMessage) (string, error) {
	var doc yaml.Node
	if strings.TrimSpace(fmText) != "" {
		if err := yaml.Unmarshal([]byte(fmText), &doc); err != nil {
			// Front matter that isn't strict YAML (tags: [#Linux], say): read
			// it the way the build does and write it back as YAML.
			var m map[string]any
			if err2 := yaml.Unmarshal([]byte(fmText), &m); err2 != nil {
				return "", fmt.Errorf("the existing front matter can't be edited as fields; use the YAML view: %w", err)
			}
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return "", fmt.Errorf("front matter isn't a list of fields")
	}
	for _, key := range sortedKeys(set) {
		raw := bytes.TrimSpace(set[key])
		idx := -1
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == key {
				idx = i
				break
			}
		}
		if len(raw) == 0 || string(raw) == "null" {
			if idx >= 0 {
				m.Content = append(m.Content[:idx], m.Content[idx+2:]...)
			}
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		vn, err := jsonNode(dec)
		if err != nil {
			return "", fmt.Errorf("field %s: %w", key, err)
		}
		if idx >= 0 {
			vn = keep(m.Content[idx+1], vn)
			vn.HeadComment, vn.LineComment = m.Content[idx+1].HeadComment, m.Content[idx+1].LineComment
			m.Content[idx+1] = vn
		} else {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, vn)
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", err
	}
	out := strings.TrimRight(buf.String(), "\n")
	if out == "{}" {
		out = ""
	}
	return out, nil
}

// jsonNode turns JSON into YAML nodes, keeping objects' key order. Scalars
// are encoded by the YAML library itself, so a string that looks like a date
// or a number is quoted exactly as it would be anywhere else.
func jsonNode(dec *json.Decoder) (*yaml.Node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	scalar := func(v any) (*yaml.Node, error) {
		var n yaml.Node
		if err := n.Encode(v); err != nil {
			return nil, err
		}
		if s, ok := v.(string); ok && strings.Contains(s, "\n") {
			n.Style = yaml.LiteralStyle
		}
		return &n, nil
	}
	switch t := tok.(type) {
	case json.Delim:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		if t == '{' {
			n = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		for dec.More() {
			if t == '{' {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, _ := kt.(string)
				kn, err := scalar(key)
				if err != nil {
					return nil, err
				}
				n.Content = append(n.Content, kn)
			}
			v, err := jsonNode(dec)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, v)
		}
		if _, err := dec.Token(); err != nil { // the closing } or ]
			return nil, err
		}
		return n, nil
	case json.Number:
		if i, err := strconv.ParseInt(string(t), 10, 64); err == nil {
			return scalar(i)
		}
		f, err := t.Float64()
		if err != nil {
			return nil, err
		}
		return scalar(f)
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	default: // string, bool
		return scalar(t)
	}
}

// keep returns the new value, reusing the original's nodes wherever they say
// the same thing, so what didn't change keeps its style (a one-line
// { text: …, href: … }) and its comments, and a diff shows only the change.
// Items of a list are matched by content, so moving one doesn't rewrite it.
func keep(orig, nw *yaml.Node) *yaml.Node {
	if orig == nil || nw == nil {
		return nw
	}
	same := func(a, b *yaml.Node) bool {
		x, err1 := orderedJSON(a)
		y, err2 := orderedJSON(b)
		return err1 == nil && err2 == nil && bytes.Equal(x, y)
	}
	if same(orig, nw) {
		return orig
	}
	switch {
	case orig.Kind == yaml.MappingNode && nw.Kind == yaml.MappingNode:
		for i := 0; i+1 < len(nw.Content); i += 2 {
			for j := 0; j+1 < len(orig.Content); j += 2 {
				if orig.Content[j].Value == nw.Content[i].Value {
					nw.Content[i] = orig.Content[j] // the key, with its comments
					nw.Content[i+1] = keep(orig.Content[j+1], nw.Content[i+1])
					break
				}
			}
		}
		nw.Style = orig.Style
		nw.HeadComment, nw.LineComment, nw.FootComment = orig.HeadComment, orig.LineComment, orig.FootComment
	case orig.Kind == yaml.SequenceNode && nw.Kind == yaml.SequenceNode:
		used := map[int]bool{}
		for i, item := range nw.Content {
			found := false
			for j, o := range orig.Content {
				if !used[j] && same(o, item) {
					nw.Content[i], used[j], found = o, true, true
					break
				}
			}
			if found || item.Kind != yaml.MappingNode || len(item.Content) < 2 {
				continue
			}
			// An item that changed, and perhaps moved: the original with the
			// same first key (a section's type) keeps what's unchanged inside.
			for j, o := range orig.Content {
				if !used[j] && o.Kind == yaml.MappingNode && len(o.Content) >= 2 && o.Content[0].Value == item.Content[0].Value {
					nw.Content[i], used[j] = keep(o, item), true
					break
				}
			}
		}
		nw.Style = orig.Style
		nw.HeadComment, nw.LineComment, nw.FootComment = orig.HeadComment, orig.LineComment, orig.FootComment
	}
	return nw
}

// orderedJSON writes YAML as JSON with every mapping's keys in the order the
// file has them, so the editor shows and returns fields as they were written.
func orderedJSON(n *yaml.Node) (json.RawMessage, error) {
	for n.Kind == yaml.DocumentNode && len(n.Content) == 1 {
		n = n.Content[0]
	}
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		return orderedJSON(n.Alias)
	}
	var b bytes.Buffer
	switch n.Kind {
	case yaml.MappingNode:
		b.WriteByte('{')
		for i := 0; i+1 < len(n.Content); i += 2 {
			if i > 0 {
				b.WriteByte(',')
			}
			k, _ := json.Marshal(n.Content[i].Value)
			b.Write(k)
			b.WriteByte(':')
			v, err := orderedJSON(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			b.Write(v)
		}
		b.WriteByte('}')
	case yaml.SequenceNode:
		b.WriteByte('[')
		for i, c := range n.Content {
			if i > 0 {
				b.WriteByte(',')
			}
			v, err := orderedJSON(c)
			if err != nil {
				return nil, err
			}
			b.Write(v)
		}
		b.WriteByte(']')
	default:
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		out, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		b.Write(out)
	}
	return b.Bytes(), nil
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Stable output: the order fields were added doesn't depend on map order.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// rawSet turns Go values into an edit's Set.
func rawSet(m map[string]any) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range m {
		b, _ := json.Marshal(v)
		out[k] = b
	}
	return out
}
