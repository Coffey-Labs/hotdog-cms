// Command hotdog-cms builds a website from Markdown and templates.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/about"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/check"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/ci"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/editor"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/endpoint"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forge"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/gitx"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/importer"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/isolate"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/packs"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/preview"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/publish"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/serve"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/starter"
)

const usage = `hotdog-cms builds a website from Markdown files and templates.

Usage:
  hotdog-cms new <folder>          start a site from the starter template
  hotdog-cms serve [flags]         preview the site, rebuilding as you edit
  hotdog-cms build [flags]         build the site into public/
  hotdog-cms check [flags]         build, then look for broken links and forbidden strings
  hotdog-cms publish [target...]   build, then publish to targets in publish.yaml
  hotdog-cms pull -repo URL -out DIR   on a web server: publish from git when the branch moves
  hotdog-cms preview -repo URL     build every branch as it would be published, each at its own address
  hotdog-cms ci <platform>         write the CI file for github, gitea, forgejo, gitlab, bitbucket, woodpecker, or a git hook
  hotdog-cms endpoint -config FILE the small server for form submissions and search, behind your web server
  hotdog-cms pack add|remove|list|check   extensions as files: sections, layouts, styles (docs/packs.md)
  hotdog-cms import <from> ...     bring a site in: wordpress, wxr, ghost, hugo or jekyll (docs/importing.md)
  hotdog-cms editor -config FILE   the editor: sign in with your git platform and work on sites (never on a site's host)

Platforms: GitHub, GitLab, Gitea, Forgejo/Codeberg, Gogs, Bitbucket, or any git server.
Secrets come from the environment, never flags:
  HOTDOG_GIT_TOKEN, HOTDOG_GIT_USER   clone private repositories over HTTPS
  HOTDOG_HOOK_SECRET                 verify push webhooks (preview, pull -hook)
  HOTDOG_FORGE_TOKEN                 post preview links as commit statuses
  HOTDOG_FORM_SECRET                 sign form tokens (endpoint; same on every endpoint of a site)
  HOTDOG_SMTP_PASSWORD               the endpoint's SMTP password
  HOTDOG_TURNSTILE_SECRET            check Cloudflare Turnstile on forms that use it
  HOTDOG_EDITOR_SECRET               seal editor sessions (same on every editor serving the same people)
  hotdog-cms version

Run "hotdog-cms <command> -h" for a command's flags.
`

func main() {
	if len(os.Args) == 2 && os.Args[1] == isolate.ChildArg {
		isolate.Child() // a build the editor, preview or pull agent handed off
	}
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
		isolate.Enabled = true
		err = cmdPull(args)
	case "preview":
		isolate.Enabled = true
		err = cmdPreview(args)
	case "ci":
		err = cmdCI(args)
	case "endpoint":
		err = cmdEndpoint(args)
	case "editor":
		isolate.Enabled = true
		err = cmdEditor(args)
	case "import":
		err = cmdImport(args)
	case "pack":
		err = cmdPack(args)
	case "version", "-v", "--version":
		rev, mod := about.Revision()
		if len(rev) > 12 {
			rev = rev[:12]
		}
		if mod {
			rev += ", modified"
		}
		if rev != "" {
			rev = " (" + rev + ")"
		}
		fmt.Printf("hotdog-cms %s%s\n%s. %s; source: %s\n", about.Version, rev, about.Copyright, about.License, about.DefaultSource)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "hotdog-cms: no command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hotdog-cms:", err)
		os.Exit(1)
	}
}

func cmdNew(args []string) error {
	fl := flag.NewFlagSet("new", flag.ExitOnError)
	name := fl.String("name", "", "the site's name (default: the folder's name)")
	_ = fl.Parse(args)
	if fl.NArg() != 1 {
		return errors.New("usage: hotdog-cms new <folder>")
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
	fmt.Printf("Started %s in %s.\n\n  cd %s\n  hotdog-cms serve\n", *name, dir, dir)
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
	csp := fl.Bool("csp", false, "also print the Content-Security-Policy the pages need")
	fix := fl.Bool("fix", false, "say under each finding what usually puts it right")
	_ = fl.Parse(args)
	res, err := build.Run(build.Options{SiteDir: *dir, Out: *out, Drafts: *drafts})
	if err != nil {
		return err
	}
	report(res)
	rep, err := check.Run(res.Site.Dir, res.Out, res.Site.Config)
	if err != nil {
		return err
	}
	for _, p := range rep.Problems {
		fmt.Println(" ", p)
		if *fix && p.Fix != "" {
			fmt.Println("           fix:", p.Fix)
		}
	}
	if *csp {
		fmt.Printf("\nThe Content-Security-Policy these pages need (add hosts your scripts fetch to connect-src):\n  %s\n", rep.CSP)
	}
	errs, warns := rep.Errors(), len(rep.Problems)-rep.Errors()
	if errs > 0 {
		return fmt.Errorf("%d error(s), %d warning(s)", errs, warns)
	}
	if warns > 0 {
		fmt.Printf("No errors; %d warning(s).\n", warns)
		return nil
	}
	fmt.Println("No problems found.")
	return nil
}

func cmdServe(args []string) error {
	fl := flag.NewFlagSet("serve", flag.ExitOnError)
	dir := fl.String("site", ".", "the site's folder")
	addr := fl.String("addr", "127.0.0.1:8140", "address to listen on")
	drafts := fl.Bool("drafts", true, "include pages marked draft: true")
	ep := fl.String("endpoint", "", "a running hotdog-cms endpoint to pass /_hotdog/ requests to, e.g. http://127.0.0.1:8180")
	reload := fl.Bool("reload", true, "reload the browser on changes (turn off for headless browsers and screenshots)")
	_ = fl.Parse(args)
	if _, err := site.LoadConfig(*dir); err != nil {
		return err
	}
	return serve.Run(serve.Options{SiteDir: *dir, Addr: *addr, Drafts: *drafts, Endpoint: *ep, NoReload: !*reload})
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
	// The same check whichever way a site is published, and no flag to skip
	// it: a site that means to break a rule says so in site.yaml (check.ignore).
	cfg, err := site.LoadConfig(*dir)
	if err != nil {
		return err
	}
	rep, err := check.Run(*dir, out, cfg)
	if err != nil {
		return err
	}
	for _, p := range rep.Problems {
		fmt.Println(" ", p)
	}
	if n := rep.Errors(); n > 0 {
		return fmt.Errorf("not published: %d check error(s); fix them, or switch the rule off in site.yaml (check.ignore)", n)
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
	platform := fl.String("platform", "", "github, gitlab, gitea, forgejo, gogs, bitbucket or git (default: guessed from the repository URL)")
	hook := fl.String("hook", "", "also listen here for push webhooks, e.g. 127.0.0.1:8170 behind a proxy (needs HOTDOG_HOOK_SECRET)")
	indexnow := fl.Bool("indexnow", false, "after each publish, tell search engines which pages changed (needs indexnow.key in site.yaml)")
	_ = fl.Parse(args)
	if *repo == "" || *out == "" {
		return errors.New("usage: hotdog-cms pull -repo URL -out DIR [-branch main] [-every 1m] [-hook ADDR]")
	}
	fr, err := forge.Parse(*repo, forge.Kind(*platform))
	if err != nil {
		return err
	}
	if *subdir != "" && (filepath.IsAbs(*subdir) || strings.Contains(*subdir, "..")) {
		return errors.New("-subdir must be a relative path inside the repository")
	}
	return publish.Pull(publish.PullOptions{IndexNow: *indexnow, Repo: *repo, Branch: *branch, Subdir: *subdir, Out: *out, Cache: *cache, Every: *every,
		Git: gitx.FromEnv(fr.Kind.TokenUser()), Log: os.Stdout,
		HookAddr: *hook, HookSecret: os.Getenv("HOTDOG_HOOK_SECRET"), HookKind: fr.Kind})
}

func cmdPreview(args []string) error {
	fl := flag.NewFlagSet("preview", flag.ExitOnError)
	repo := fl.String("repo", "", "the site's git repository (required)")
	subdir := fl.String("subdir", "", "the site's folder inside the repository")
	branches := fl.String("branches", "", "comma-separated branch patterns to preview, e.g. main,feature/* (default all)")
	root := fl.String("root", "", "where previews are built (default under the user cache folder)")
	addr := fl.String("addr", "127.0.0.1:8160", "address to listen on; anything but loopback needs HOTDOG_PREVIEW_AUTH")
	domain := fl.String("domain", "localhost", "previews are served at <branch>.<domain>")
	scheme := fl.String("scheme", "http", "the scheme browsers use (https behind a TLS proxy)")
	port := fl.String("port", "", "the port browsers use, if not the listening port on localhost or 80/443 elsewhere")
	every := fl.Duration("every", 30*time.Second, "how often to look for branches that moved")
	drafts := fl.Bool("drafts", true, "include draft pages")
	platform := fl.String("platform", "", "github, gitlab, gitea, forgejo, gogs, bitbucket or git (default: guessed from the repository URL)")
	frame := fl.String("frame", "", "comma-separated origins allowed to show previews in a frame, e.g. the editor: http://127.0.0.1:8190")
	_ = fl.Parse(args)
	if *repo == "" {
		return errors.New("usage: hotdog-cms preview -repo URL [-branches main,feature/*] [-addr 127.0.0.1:8160]")
	}
	var ancestors []string
	for _, o := range strings.Split(*frame, ",") {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o == "" {
			continue
		}
		if u, err := url.Parse(o); err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" {
			return fmt.Errorf("-frame %q is not an origin like https://editor.example.org", o)
		}
		ancestors = append(ancestors, o)
	}
	fr, err := forge.Parse(*repo, forge.Kind(*platform))
	if err != nil {
		return err
	}
	fc := &forge.Client{Repo: fr, Token: os.Getenv("HOTDOG_FORGE_TOKEN")}
	if *subdir != "" && (filepath.IsAbs(*subdir) || strings.Contains(*subdir, "..")) {
		return errors.New("-subdir must be a relative path inside the repository")
	}
	if *root == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		*root = filepath.Join(base, "hotdog-cms", "previews")
	}
	if *port == "" && *domain == "localhost" {
		if _, p, err := net.SplitHostPort(*addr); err == nil {
			*port = p
		}
	}
	var auth *preview.Auth
	if v := os.Getenv("HOTDOG_PREVIEW_AUTH"); v != "" {
		u, p, ok := strings.Cut(v, ":")
		if !ok || u == "" || len(p) < 12 {
			return errors.New("HOTDOG_PREVIEW_AUTH must be user:password, with a password of at least 12 characters")
		}
		auth = &preview.Auth{User: u, Pass: p}
	}
	var refs []string
	for _, r := range strings.Split(*branches, ",") {
		if r = strings.TrimSpace(r); r != "" {
			refs = append(refs, r)
		}
	}
	b, err := preview.NewBuilder(preview.Options{Repo: *repo, Subdir: *subdir, Refs: refs, Root: *root,
		Domain: strings.ToLower(*domain), Port: *port, Scheme: *scheme, Drafts: *drafts, Log: os.Stdout,
		Git: gitx.FromEnv(fr.Kind.TokenUser()), Forge: fc, HookSecret: os.Getenv("HOTDOG_HOOK_SECRET"), FrameAncestors: ancestors})
	if err != nil {
		return err
	}
	return b.Serve(*addr, auth, *every)
}

var safeArgRe = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

func cmdCI(args []string) error {
	fl := flag.NewFlagSet("ci", flag.ExitOnError)
	dir := fl.String("site", ".", "the site's folder, relative to the repository root")
	branch := fl.String("branch", "main", "the branch that publishes")
	write := fl.Bool("write", false, "write the file into the repository (the current folder) instead of printing it")
	_ = fl.Parse(args)
	if fl.NArg() != 1 {
		return fmt.Errorf("usage: hotdog-cms ci <%s> [-site .] [-branch main] [-write]", strings.Join(ci.Platforms(), "|"))
	}
	if !safeArgRe.MatchString(*dir) || !safeArgRe.MatchString(*branch) || strings.Contains(*dir, "..") {
		return errors.New("-site and -branch may only use letters, digits, ., _, / and -")
	}
	t, err := ci.Get(fl.Arg(0), *dir, *branch)
	if err != nil {
		return err
	}
	if !*write {
		fmt.Printf("# %s\n%s\n# %s\n", t.Path, t.Body, t.Note)
		return nil
	}
	if _, err := os.Stat(t.Path); err == nil {
		return fmt.Errorf("%s already exists; not replacing it", t.Path)
	}
	if err := os.MkdirAll(filepath.Dir(t.Path), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if t.Path == "post-receive" {
		mode = 0o755
	}
	if err := os.WriteFile(t.Path, []byte(t.Body), mode); err != nil {
		return err
	}
	fmt.Printf("Wrote %s.\n%s\n", t.Path, t.Note)
	return nil
}

func cmdEndpoint(args []string) error {
	fl := flag.NewFlagSet("endpoint", flag.ExitOnError)
	config := fl.String("config", "endpoint.yaml", "the endpoint's configuration")
	_ = fl.Parse(args)
	srv, err := endpoint.New(*config, os.Stdout)
	if err != nil {
		return err
	}
	fmt.Printf("endpoint listening on %s\n", srv.Listen())
	viewer, viewerAddr, err := srv.Viewer()
	if err != nil {
		return err
	}
	errc := make(chan error, 2)
	if viewer != nil {
		fmt.Printf("submissions viewer on %s\n", viewerAddr)
		go func() {
			vs := &http.Server{Addr: viewerAddr, Handler: viewer, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
			errc <- vs.ListenAndServe()
		}()
	}
	go func() {
		hs := &http.Server{Addr: srv.Listen(), Handler: srv, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
		errc <- hs.ListenAndServe()
	}()
	return <-errc
}

func cmdEditor(args []string) error {
	fl := flag.NewFlagSet("editor", flag.ExitOnError)
	config := fl.String("config", "editor.yaml", "the editor's configuration")
	_ = fl.Parse(args)
	srv, err := editor.New(*config, os.Stdout)
	if err != nil {
		return err
	}
	live, liveAddr := srv.Live()
	fmt.Printf("editor at %s, live preview at %s\n", srv.Listen(), liveAddr)
	errc := make(chan error, 2)
	go func() {
		ls := &http.Server{Addr: liveAddr, Handler: live, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10}
		errc <- ls.ListenAndServe()
	}()
	go func() {
		hs := &http.Server{Addr: srv.Listen(), Handler: srv, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 120 * time.Second, MaxHeaderBytes: 32 << 10}
		errc <- hs.ListenAndServe()
	}()
	return <-errc
}

func report(r *build.Result) {
	fmt.Printf("Built %d pages (%d files) into %s in %s.\n", r.Pages, r.Files, r.Out, r.Duration.Round(time.Millisecond))
	for _, f := range r.Feeds {
		fmt.Printf("  feed: %s\n", f)
	}
}

func cmdImport(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: hotdog-cms import <wordpress|wxr|ghost|hugo|jekyll> [flags] (hotdog-cms import <from> -h for each)")
	}
	from, args := args[0], args[1:]
	fl := flag.NewFlagSet("import "+from, flag.ExitOnError)
	siteDir := fl.String("site", ".", "the HotDog CMS site to import into (made with hotdog-cms new)")
	posts := fl.String("posts", "articles", "the collection posts go into")
	overwrite := fl.Bool("overwrite", false, "replace pages that are already there")
	var siteURL, user, file, dir, rename *string
	switch from {
	case "wordpress":
		siteURL = fl.String("url", "", "the WordPress site, e.g. https://example.com (required)")
		user = fl.String("user", "", "a WordPress user, for drafts and private posts; the Application Password goes in HOTDOG_WP_PASSWORD")
	case "wxr":
		file = fl.String("file", "", "the export file from Tools → Export (required)")
		siteURL = fl.String("url", "", "the old site's address, if pictures are linked relative to it")
	case "ghost":
		file = fl.String("file", "", "the export file from Settings → Labs → Export (required)")
		siteURL = fl.String("url", "", "the old site's address, for its pictures (required)")
	case "hugo":
		dir = fl.String("from", "", "the Hugo site's folder (required)")
		rename = fl.String("rename", "posts=articles", "sections to rename, as from=to,from=to")
	case "jekyll":
		dir = fl.String("from", "", "the Jekyll site's folder (required)")
	default:
		return fmt.Errorf("can't import from %q: wordpress, wxr, ghost, hugo or jekyll", from)
	}
	_ = fl.Parse(args)
	var items []importer.Item
	var err error
	opt := importer.Options{SiteDir: *siteDir, Overwrite: *overwrite, Log: os.Stdout}
	source := from
	switch from {
	case "wordpress":
		if *siteURL == "" {
			return errors.New("-url is required")
		}
		pass := os.Getenv("HOTDOG_WP_PASSWORD")
		if *user != "" && pass == "" {
			return errors.New("set HOTDOG_WP_PASSWORD to an Application Password for that user (Users → Profile → Application Passwords)")
		}
		fmt.Printf("reading %s…\n", *siteURL)
		items, err = importer.FromWordPress(importer.WordPressOptions{URL: *siteURL, User: *user, Password: pass, Posts: *posts})
		opt.Base, source = *siteURL, *siteURL
	case "wxr", "ghost":
		if *file == "" {
			return errors.New("-file is required")
		}
		f, ferr := os.Open(*file)
		if ferr != nil {
			return ferr
		}
		defer f.Close()
		if from == "wxr" {
			var link string
			items, link, err = importer.FromWXR(f, *posts)
			opt.Base = *siteURL
			if opt.Base == "" {
				opt.Base = link
			}
		} else {
			if *siteURL == "" {
				return errors.New("-url is required, for the posts' pictures")
			}
			items, err = importer.FromGhost(f, *siteURL, *posts)
			opt.Base = *siteURL
		}
		source = *file
	case "hugo", "jekyll":
		if *dir == "" {
			return errors.New("-from is required")
		}
		opt.LocalRoot, source = *dir, *dir
		if from == "hugo" {
			ren := map[string]string{}
			for _, pair := range strings.Split(*rename, ",") {
				if a, b, ok := strings.Cut(pair, "="); ok {
					ren[strings.TrimSpace(a)] = strings.TrimSpace(b)
				}
			}
			items, err = importer.FromHugo(*dir, ren)
		} else {
			items, err = importer.FromJekyll(*dir, *posts)
		}
	}
	if err != nil {
		return err
	}
	fmt.Printf("%d page(s) found; writing…\n", len(items))
	rep, err := importer.Write(items, opt)
	if err != nil {
		return err
	}
	report := filepath.Join(*siteDir, "IMPORT-REPORT.md")
	if err := os.WriteFile(report, []byte(rep.Markdown(source)), 0o644); err != nil {
		return err
	}
	fmt.Printf("\n%d written, %d skipped, %d kept as HTML, %d picture(s), %d redirect(s). Details: %s\nThen: hotdog-cms check\n",
		len(rep.Written), len(rep.Skipped), len(rep.KeptHTML), rep.Pictures, len(rep.Redirects), report)
	return nil
}

func cmdPack(args []string) error {
	usage := errors.New("usage: hotdog-cms pack add [-force] <name|folder|git-url[@tag]> | remove <name> | list | check <name|folder|git-url>\n(each takes -site DIR before its argument; default .)")
	if len(args) == 0 {
		return usage
	}
	verb, args := args[0], args[1:]
	fl := flag.NewFlagSet("pack "+verb, flag.ExitOnError)
	siteDir := fl.String("site", ".", "the site")
	force := fl.Bool("force", false, "add: replace files already in the site")
	_ = fl.Parse(args)
	switch verb {
	case "add":
		if fl.NArg() != 1 {
			return usage
		}
		ref, err := packs.Add(*siteDir, fl.Arg(0), *force)
		if err != nil {
			return err
		}
		fmt.Printf("installed %s %s: %d file(s)\n", ref.Name, ref.Version, len(ref.Files))
		for _, f := range ref.Files {
			fmt.Println(" ", f)
		}
		if len(ref.AddedCollections) > 0 {
			fmt.Printf("site.yaml: added collections settings for %s\n", strings.Join(ref.AddedCollections, ", "))
		}
		fmt.Println("They're the site's files now: commit them, and review them like any change.")
		if ref.Notes != "" {
			fmt.Println()
			fmt.Print(strings.TrimRight(ref.Notes, "\n") + "\n")
		}
	case "remove":
		if fl.NArg() != 1 {
			return usage
		}
		if err := packs.Remove(*siteDir, fl.Arg(0)); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", fl.Arg(0))
	case "list":
		cfg, err := site.LoadConfig(*siteDir)
		if err != nil {
			return err
		}
		if len(cfg.Packs) == 0 {
			fmt.Println("no packs installed")
		}
		for _, p := range cfg.Packs {
			src := p.Source
			if src == "" {
				src = "a local folder"
			}
			fmt.Printf("%s %s (%d files, from %s)\n", p.Name, p.Version, len(p.Files), src)
		}
		fmt.Printf("\nPacks that come with HotDog CMS (hotdog-cms pack add <name>): %s\n", strings.Join(packs.BundledNames(), ", "))
	case "check":
		if fl.NArg() != 1 {
			return usage
		}
		m, files, err := packs.CheckSource(fl.Arg(0))
		if err != nil {
			return err
		}
		fmt.Printf("%s %s: installs %d file(s)\n", m.Name, m.Version, len(files))
		for _, f := range files {
			fmt.Printf("  %s -> %s\n", f.From, f.To)
		}
	default:
		return usage
	}
	return nil
}
