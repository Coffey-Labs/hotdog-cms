package editor

import (
	"bytes"
	"fmt"
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
	// it), null removes it.
	Set map[string]any `json:"set,omitempty"`
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

func applySet(fmText string, set map[string]any) (string, error) {
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
		val := set[key]
		idx := -1
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == key {
				idx = i
				break
			}
		}
		if val == nil {
			if idx >= 0 {
				m.Content = append(m.Content[:idx], m.Content[idx+2:]...)
			}
			continue
		}
		var vn yaml.Node
		if err := vn.Encode(val); err != nil {
			return "", fmt.Errorf("field %s: %w", key, err)
		}
		if s, ok := val.(string); ok && strings.Contains(s, "\n") {
			vn.Style = yaml.LiteralStyle
		}
		if idx >= 0 {
			vn.HeadComment, vn.LineComment = m.Content[idx+1].HeadComment, m.Content[idx+1].LineComment
			m.Content[idx+1] = &vn
		} else {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &vn)
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

func sortedKeys(m map[string]any) []string {
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
