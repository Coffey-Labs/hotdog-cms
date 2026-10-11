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

// setTopLevel replaces one top-level key of a YAML file (removing it when
// value is empty), leaving every other line exactly as written: site.yaml is
// a file people read and comment, and a diff should show only the change.
// SetTopLevel replaces one top-level key of a YAML file (removing it when
// value is empty), leaving every other line exactly as written.
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
