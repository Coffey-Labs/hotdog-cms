// Package publish takes a finished build to wherever the operator serves the
// site from. hotdog-cms doesn't decide that: a target is a folder on this machine,
// a server reached over SSH, a container image, or a command of the operator's
// own, and an operator can run any of them from CI, from a deploy script or by
// hand. (The pull agent, for servers that fetch their own updates, is in
// pull.go.)
//
// Targets are read from a file the operator owns, by default publish.yaml in
// the site's folder. Values may name environment variables (${DEPLOY_HOST}),
// so host names and registries can stay out of a public repository.
// Credentials never go in the file: SSH uses the operator's keys and agent,
// registries use the container tool's own login.
package publish

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
)

// Target is one place a site can be published to.
type Target struct {
	Type string `yaml:"type"` // dir, rsync, container or command

	// dir: a local folder, typically a web server's root.
	// rsync: the folder on the remote server.
	Path string `yaml:"path"`

	// rsync: [user@]host, and an SSH port if it isn't 22.
	Host string `yaml:"host"`
	Port int    `yaml:"port"`

	// container: the image to build, its web server, and whether to push it.
	Image   string `yaml:"image"`
	Server  string `yaml:"server"`  // nginx (default) or caddy
	Push    bool   `yaml:"push"`    // push to the registry named in image
	Tool    string `yaml:"tool"`    // docker or podman; default whichever is installed
	Context string `yaml:"context"` // write the build context here and stop, for a pipeline that builds images itself
	CSP     bool   `yaml:"csp"`     // send the Content-Security-Policy the pages need (as hotdog-cms check -csp prints it)

	// command: argv, run without a shell. {out} is replaced with the build folder.
	Command []string `yaml:"command"`
}

// Load reads a targets file.
func Load(file string) (map[string]Target, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Targets map[string]Target `yaml:"targets"`
	}
	dec := yaml.NewDecoder(strings.NewReader(os.ExpandEnv(string(raw))))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if len(doc.Targets) == 0 {
		return nil, fmt.Errorf("%s: no targets", file)
	}
	for name, t := range doc.Targets {
		if err := t.validate(); err != nil {
			return nil, fmt.Errorf("%s: target %s: %w", file, name, err)
		}
	}
	return doc.Targets, nil
}

// Names returns target names in order.
func Names(ts map[string]Target) []string {
	out := make([]string, 0, len(ts))
	for n := range ts {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

var hostRe = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?[A-Za-z0-9.-]+$|^(?:[A-Za-z0-9._-]+@)?\[[0-9A-Fa-f:]+\]$`)

func (t Target) validate() error {
	switch t.Type {
	case "dir":
		if t.Path == "" {
			return fmt.Errorf("dir needs path")
		}
	case "rsync":
		if !hostRe.MatchString(t.Host) {
			return fmt.Errorf("rsync needs host as [user@]host, got %q", t.Host)
		}
		if err := safeRemotePath(t.Path); err != nil {
			return err
		}
	case "container":
		if t.Image == "" && t.Context == "" {
			return fmt.Errorf("container needs image, or context to only write the build context")
		}
		if t.Server != "" && t.Server != "nginx" && t.Server != "caddy" {
			return fmt.Errorf("server must be nginx or caddy")
		}
		if t.Tool != "" && t.Tool != "docker" && t.Tool != "podman" {
			return fmt.Errorf("tool must be docker or podman")
		}
	case "command":
		if len(t.Command) == 0 {
			return fmt.Errorf("command needs a command")
		}
	default:
		return fmt.Errorf("type %q is not dir, rsync, container or command", t.Type)
	}
	return nil
}

// safeRemotePath keeps a typo from aiming a delete-and-replace at / or /var.
func safeRemotePath(p string) error {
	if !path.IsAbs(p) || path.Clean(p) != p {
		return fmt.Errorf("path %q must be an absolute, clean path", p)
	}
	if strings.Count(p, "/") < 2 {
		return fmt.Errorf("path %q is too close to / to replace", p)
	}
	if strings.ContainsAny(p, "\x00\n") {
		return fmt.Errorf("path %q has control characters", p)
	}
	return nil
}

// Run publishes the build in out to one target.
func Run(name string, t Target, out string, log io.Writer) error {
	if _, err := os.Stat(filepath.Join(out, build.Marker)); err != nil {
		return fmt.Errorf("%s is not a hotdog-cms build; run hotdog-cms build first", out)
	}
	fmt.Fprintf(log, "publishing to %s (%s)\n", name, t.Type)
	switch t.Type {
	case "dir":
		return build.Install(out, t.Path)
	case "rsync":
		return rsync(t, out, log)
	case "container":
		return container(t, out, log)
	case "command":
		args := make([]string, len(t.Command))
		for i, a := range t.Command {
			args[i] = strings.ReplaceAll(a, "{out}", out)
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "HOTDOG_OUT="+out)
		cmd.Stdout, cmd.Stderr = log, log
		return cmd.Run()
	}
	return fmt.Errorf("unknown target type %q", t.Type)
}

// shq quotes a string for the remote shell.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// rsync uploads into a staging folder beside the live one, hard-linking every
// unchanged file from the live copy so only changes cross the network, then
// swaps the two on the server with renames. Visitors see the old site or the
// new one, never a half-uploaded mix, and the live folder is only replaced if
// it is empty, missing, or an earlier hotdog-cms build.
func rsync(t Target, out string, log io.Writer) error {
	for _, tool := range []string{"rsync", "ssh"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("rsync target needs %s installed", tool)
		}
	}
	sshArgs := []string{"-o", "BatchMode=yes"}
	if t.Port != 0 {
		sshArgs = append(sshArgs, "-p", strconv.Itoa(t.Port))
	}
	remote := func(script string) error {
		cmd := exec.Command("ssh", append(append([]string{}, sshArgs...), t.Host, script)...)
		cmd.Stdout, cmd.Stderr = log, log
		return cmd.Run()
	}
	live, staging, old := t.Path, t.Path+".hotdog-cms-staging", t.Path+".hotdog-cms-old"
	check := fmt.Sprintf(`if [ -e %[1]s ] && [ ! -e %[1]s/%[2]s ] && [ -n "$(ls -A %[1]s 2>/dev/null)" ]; then echo "%[1]s has files hotdog-cms did not write; refusing to replace it" >&2; exit 3; fi; mkdir -p %[3]s`,
		shq(live), build.Marker, shq(staging))
	if err := remote(check); err != nil {
		return fmt.Errorf("checking %s:%s: %w", t.Host, live, err)
	}
	rs := []string{"-a", "--delete", "--link-dest=" + live + "/", "-e", "ssh " + strings.Join(sshArgs, " "),
		strings.TrimRight(out, "/") + "/", t.Host + ":" + staging + "/"}
	cmd := exec.Command("rsync", rs...)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync: %w", err)
	}
	swap := fmt.Sprintf(`set -e; rm -rf %[3]s; if [ -e %[1]s ]; then mv %[1]s %[3]s; fi; mv %[2]s %[1]s; rm -rf %[3]s`,
		shq(live), shq(staging), shq(old))
	if err := remote(swap); err != nil {
		return fmt.Errorf("swapping on %s: %w", t.Host, err)
	}
	return nil
}
