package isolate

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/starter"
)

// The test binary is its own build child.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == ChildArg {
		Child()
	}
	os.Exit(m.Run())
}

func site(t *testing.T, layout string) string {
	t.Helper()
	dir := t.TempDir()
	fs.WalkDir(starter.Files, "site", func(p string, d fs.DirEntry, err error) error {
		target := filepath.Join(dir, strings.TrimPrefix(p, "site"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, _ := starter.Files.ReadFile(p)
		return os.WriteFile(target, []byte(strings.ReplaceAll(string(data), "{{SITE_NAME}}", "Test")), 0o644)
	})
	if layout != "" {
		os.WriteFile(filepath.Join(dir, "layouts", "page.html"), []byte(layout), 0o644)
	}
	return dir
}

func TestIsolatedBuilds(t *testing.T) {
	Enabled = true
	defer func() { Enabled = false }()
	BuildTimeout = 20 * time.Second

	// An ordinary build works the same as in-process.
	dir := site(t, "")
	sum, err := Build(build.Options{SiteDir: dir, Out: filepath.Join(t.TempDir(), "out")})
	if err != nil || sum.Pages == 0 {
		t.Fatalf("plain build: %v %+v", err, sum)
	}
	html, p, err := RenderPage(dir, "content/about.md", []byte("---\ntitle: Changed\n---\nHi.\n"), build.Options{})
	if err != nil || p != "/about/" || !strings.Contains(string(html), "Changed") {
		t.Fatalf("render: %v %s", err, p)
	}

	// A template that loops forever is stopped at the deadline.
	BuildTimeout = 3 * time.Second
	loop := site(t, `{{ define "main" }}{{ range 100000000000000 }}{{ end }}{{ end }}`)
	start := time.Now()
	_, err = Build(build.Options{SiteDir: loop, Out: filepath.Join(t.TempDir(), "out")})
	if err == nil || !strings.Contains(err.Error(), "took longer") || time.Since(start) > 10*time.Second {
		t.Errorf("looping template: %v after %s", err, time.Since(start))
	}

	// One that eats memory without writing anything dies alone.
	BuildTimeout = 8 * time.Second
	hog := site(t, `{{ define "main" }}{{ $x := list }}{{ range 1000000000 }}{{ $x = append $x "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }}{{ end }}{{ end }}`)
	_, err = Build(build.Options{SiteDir: hog, Out: filepath.Join(t.TempDir(), "out")})
	if err == nil {
		t.Error("a memory-eating template built")
	}
	t.Logf("memory hog: %v", err)

	// The child sees none of the parent's secrets.
	t.Setenv("HOTDOG_EDITOR_SECRET", "do-not-leak")
	for _, kv := range childEnv() {
		if strings.Contains(kv, "do-not-leak") {
			t.Error("a secret reached the build child")
		}
	}
}
