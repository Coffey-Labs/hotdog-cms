package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCheckoutLeavesNoLinks(t *testing.T) {
	src := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", src, "-c", "user.name=t", "-c", "user.email=t@example.org", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	os.MkdirAll(filepath.Join(src, "static"), 0o755)
	os.Symlink("/etc/passwd", filepath.Join(src, "static", "x"))
	os.Symlink("/tmp", filepath.Join(src, "data"))
	run("add", "-A")
	run("commit", "-qm", "links")

	dir := filepath.Join(t.TempDir(), "co")
	for i := 0; i < 2; i++ { // the clone, then a fetch into it
		if _, err := Checkout(Auth{}, src, "main", dir); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{"static/x", "data"} {
			fi, err := os.Lstat(filepath.Join(dir, p))
			if err != nil || !fi.Mode().IsRegular() {
				t.Errorf("%s is %v, not a plain file", p, fi.Mode())
			}
		}
	}
}

func TestValidBranch(t *testing.T) {
	for name, want := range map[string]bool{
		"main": true, "draft/about": true, "draft/quick-1": true, "release-1.0": true,
		"-x": false, "--upload-pack=x": false, "draft/-x": false, "a..b": false, "a b": false, "a~1": false,
		"a^": false, "a:b": false, ".hidden": false, "draft/.x": false, "x.lock": false, "x/": false, "": false, "a@{1}": false,
	} {
		if ValidBranch(name) != want {
			t.Errorf("%q: want %v", name, want)
		}
	}
	if _, err := Checkout(Auth{}, t.TempDir(), "--upload-pack=touch /tmp/pwned", t.TempDir()); err == nil {
		t.Error("checked out a branch named like an option")
	}
}
