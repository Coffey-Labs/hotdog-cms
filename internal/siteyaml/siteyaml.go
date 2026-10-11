// Package siteyaml changes one top-level block of a YAML file (site.yaml)
// and leaves every other line exactly as it was written.
package siteyaml

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

func emptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	case []map[string]string:
		return len(x) == 0
	}
	return false
}

var topKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*\s*:`)

// SetTopLevel replaces one top-level key of a YAML file (removing it when
// value is empty), leaving every other line exactly as written: site.yaml is
// a file people read and comment, and a diff should show only the change.
func SetTopLevel(text, key string, value any) (string, error) {
	var block string
	if !emptyValue(value) {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(map[string]any{key: value}); err != nil {
			return "", err
		}
		block = buf.String()
	}
	lines := strings.SplitAfter(text, "\n")
	start, end := -1, len(lines)
	for i, l := range lines {
		if start < 0 {
			if strings.HasPrefix(l, key+":") {
				start = i
			}
			continue
		}
		// The block ends at the next top-level key or comment.
		if topKey.MatchString(l) || strings.HasPrefix(l, "#") {
			end = i
			break
		}
	}
	if start < 0 {
		if block == "" {
			return text, nil
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return text + block, nil
	}
	if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(lines[start], key+":")), "{") {
		return "", fmt.Errorf("site.yaml writes %s: on one line; split it over several lines first", key)
	}
	out := strings.Join(lines[:start], "") + block + strings.Join(lines[end:], "")
	var check map[string]any
	if err := yaml.Unmarshal([]byte(out), &check); err != nil {
		return "", fmt.Errorf("site.yaml wouldn't be valid YAML after the change: %w", err)
	}
	return out, nil
}

// AddEntry adds child: value to the top-level mapping key: (collections:,
// say), after its last entry, leaving every other line as written. The
// caller makes sure child isn't there already. If key: doesn't exist, it's
// added at the end.
func AddEntry(text, key, child string, value any) (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(map[string]any{child: value}); err != nil {
		return "", err
	}
	lines := strings.SplitAfter(text, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, key+":") {
			start = i
			break
		}
	}
	if start < 0 {
		return SetTopLevel(text, key, map[string]any{child: value})
	}
	if rest := strings.TrimSpace(strings.TrimPrefix(lines[start], key+":")); rest != "" && !strings.HasPrefix(rest, "#") {
		return "", fmt.Errorf("site.yaml writes %s: on one line; split it over several lines first", key)
	}
	// The block's entries are the indented lines after key:; new ones go
	// after the last of them, at the same indent.
	last, indent := start, "  "
	found := false
	for i := start + 1; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if l[0] != ' ' && l[0] != '\t' {
			break // the next top-level key or comment
		}
		if !found && !strings.HasPrefix(t, "#") {
			indent = l[:len(l)-len(strings.TrimLeft(l, " \t"))]
			found = true
		}
		last = i
	}
	var add strings.Builder
	for _, l := range strings.SplitAfter(buf.String(), "\n") {
		if l != "" {
			add.WriteString(indent + l)
		}
	}
	tail := strings.Join(lines[last+1:], "")
	head := strings.Join(lines[:last+1], "")
	if !strings.HasSuffix(head, "\n") {
		head += "\n"
	}
	out := head + add.String() + tail
	var check map[string]any
	if err := yaml.Unmarshal([]byte(out), &check); err != nil {
		return "", fmt.Errorf("site.yaml wouldn't be valid YAML after the change: %w", err)
	}
	return out, nil
}
