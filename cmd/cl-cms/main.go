// Command cl-cms builds a website from Markdown and templates.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/check"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/publish"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/serve"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/site"
	"git.coffeylabs.org/coffey-labs/cl-cms/starter"
)

var version = "dev"

const usage = `cl-cms builds a website from Markdown files and templates.

Usage:
  cl-cms new <folder>          start a site from the starter template
  cl-cms serve [flags]         preview the site, rebuilding as you edit
  cl-cms build [flags]         build the site into public/
  cl-cms check [flags]         build, then look for broken links and forbidden strings
  cl-cms publish [target...]   build, then publish to targets in publish.yaml
  cl-cms pull -repo URL -out DIR   on a web server: publish from git when the branch moves
  cl-cms version

Run "cl-cms <command> -h" for a command's flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "new":
		err = cmdNew(args)
	case "serve":
		err = cmdServe(args)
	case "build":
		err = cmdBuild(args)
	case "check":
		err = cmdCheck(args)
	case "publish":
		err = cmdPublish(args)
	case "pull":
		err = cmdPull(args)
	case "version", "-v", "--version":
		fmt.Println("cl-cms", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "cl-cms: no command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cl-cms:", err)
		os.Exit(1)
	}
}

func cmdNew(args []string) error {
	fl := flag.NewFlagSet("new", flag.ExitOnError)
	name := fl.String("name", "", "the site's name (default: the folder's name)")
	_ = fl.Parse(args)
	if fl.NArg() != 1 {
		return errors.New("usage: cl-cms new <folder>")
	}
	dir := fl.Arg(0)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s already has files in it", dir)
	}
	if *name == "" {
		abs, _ := filepath.Abs(dir)
		*name = filepath.Base(abs)
	}
	err := fs.WalkDir(starter.Files, "site", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, "site"), "/")
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := starter.Files.ReadFile(p)
		if err != nil {
			return err
		}
		if rel == "site.yaml" {
			data = []byte(strings.ReplaceAll(string(data), "{{SITE_NAME}}", *name))
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return err
	}
	fmt.Printf("Started %s in %s.\n\n  cd %s\n  cl-cms serve\n", *name, dir, dir)
	return nil
}

func buildFlags(fl *flag.FlagSet) (*string, *string, *bool) {
	dir := fl.String("site", ".", "the site's folder")
	out := fl.String("out", "", "where to write the site (default <site>/public)")
	drafts := fl.Bool("drafts", false, "include pages marked draft: true")
	return dir, out, drafts
}

func cmdBuild(args []string) error {
	fl := flag.NewFlagSet("build", flag.ExitOnError)
	dir, out, drafts := buildFlags(fl)
	_ = fl.Parse(args)
	res, err := build.Run(build.Options{SiteDir: *dir, Out: *out, Drafts: *drafts})
	if err != nil {
		return err
	}
	report(res)
	return nil
}

func cmdCheck(args []string) error {
	fl := flag.NewFlagSet("check", flag.ExitOnError)
	dir, out, drafts := buildFlags(fl)
	_ = fl.Parse(args)
	res, err := build.Run(build.Options{SiteDir: *dir, Out: *out, Drafts: *drafts})
	if err != nil {
		return err
	}
	report(res)
	problems, err := check.Run(res.Site.Dir, res.Out, res.Site.Config)
	if err != nil {
		return err
	}
	for _, p := range problems {
		fmt.Println("  ✗", p)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d problem(s)", len(problems))
	}
	fmt.Println("No problems found.")
	return nil
}

func cmdServe(args []string) error {
	fl := flag.NewFlagSet("serve", flag.ExitOnError)
	dir := fl.String("site", ".", "the site's folder")
	addr := fl.String("addr", "127.0.0.1:8140", "address to listen on")
	drafts := fl.Bool("drafts", true, "include pages marked draft: true")
	_ = fl.Parse(args)
	if _, err := site.LoadConfig(*dir); err != nil {
		return err
	}
	return serve.Run(serve.Options{SiteDir: *dir, Addr: *addr, Drafts: *drafts})
}

func cmdPublish(args []string) error {
	fl := flag.NewFlagSet("publish", flag.ExitOnError)
	dir := fl.String("site", ".", "the site's folder")
	targetsFile := fl.String("targets", "", "targets file (default <site>/publish.yaml); keep it outside the repository if it names hosts")
	noBuild := fl.Bool("no-build", false, "publish the existing build in <site>/public instead of building first")
	all := fl.Bool("all", false, "publish to every target in the file")
	_ = fl.Parse(args)
	file := *targetsFile
	if file == "" {
		file = filepath.Join(*dir, "publish.yaml")
	}
	targets, err := publish.Load(file)
	if err != nil {
		return err
	}
	names := fl.Args()
	switch {
	case *all:
		names = publish.Names(targets)
	case len(names) == 0 && len(targets) == 1:
		names = publish.Names(targets)
	case len(names) == 0:
		return fmt.Errorf("name a target (%s) or pass -all", strings.Join(publish.Names(targets), ", "))
	}
	for _, n := range names {
		if _, ok := targets[n]; !ok {
			return fmt.Errorf("no target %q in %s", n, file)
		}
	}
	out := filepath.Join(*dir, "public")
	if !*noBuild {
		res, err := build.Run(build.Options{SiteDir: *dir})
		if err != nil {
			return err
		}
		report(res)
		out = res.Out
	}
	for _, n := range names {
		if err := publish.Run(n, targets[n], out, os.Stdout); err != nil {
			return fmt.Errorf("target %s: %w", n, err)
		}
	}
	return nil
}

func cmdPull(args []string) error {
	fl := flag.NewFlagSet("pull", flag.ExitOnError)
	repo := fl.String("repo", "", "the site's git repository (required)")
	branch := fl.String("branch", "main", "the branch to publish")
	subdir := fl.String("subdir", "", "the site's folder inside the repository")
	out := fl.String("out", "", "the web root to publish into (required)")
	cache := fl.String("cache", "", "where to keep the clone (default under the user cache folder)")
	every := fl.Duration("every", 0, "keep checking at this interval (e.g. 1m); 0 checks once and exits")
	_ = fl.Parse(args)
	if *repo == "" || *out == "" {
		return errors.New("usage: cl-cms pull -repo URL -out DIR [-branch main] [-every 1m]")
	}
	if *subdir != "" && (filepath.IsAbs(*subdir) || strings.Contains(*subdir, "..")) {
		return errors.New("-subdir must be a relative path inside the repository")
	}
	return publish.Pull(publish.PullOptions{Repo: *repo, Branch: *branch, Subdir: *subdir, Out: *out, Cache: *cache, Every: *every, Log: os.Stdout})
}

func report(r *build.Result) {
	fmt.Printf("Built %d pages (%d files) into %s in %s.\n", r.Pages, r.Files, r.Out, r.Duration.Round(time.Millisecond))
	for _, f := range r.Feeds {
		fmt.Printf("  feed: %s\n", f)
	}
}
