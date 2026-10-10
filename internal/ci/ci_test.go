package ci

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTemplates(t *testing.T) {
	for _, p := range Platforms() {
		tpl, err := Get(p, "sites/www", "live")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(tpl.Body, "{{SITE}}") || strings.Contains(tpl.Body, "{{BRANCH}}") {
			t.Errorf("%s: placeholders left in", p)
		}
		if p != "git" && (!strings.Contains(tpl.Body, "sites/www") || !strings.Contains(tpl.Body, "live")) {
			t.Errorf("%s: site or branch not filled in", p)
		}
		if strings.Contains(tpl.Body, "StrictHostKeyChecking") {
			t.Errorf("%s: turns host key checking off", p)
		}
		if p == "git" {
			f := filepath.Join(t.TempDir(), "post-receive")
			os.WriteFile(f, []byte(tpl.Body), 0o755)
			if out, err := exec.Command("sh", "-n", f).CombinedOutput(); err != nil {
				t.Errorf("git hook: %v %s", err, out)
			}
			continue
		}
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(tpl.Body), &doc); err != nil {
			t.Errorf("%s: not valid YAML: %v", p, err)
		}
	}
	if _, err := Get("svn", ".", "main"); err == nil {
		t.Error("accepted an unknown platform")
	}
}
