// Package isolate runs builds in a child process. A site's templates are
// files anyone who can write to the repository can change, and the editor,
// the preview server and the pull agent build them on a shared machine. A
// template can't read files or run commands, but it can loop. In a child
// process with a deadline and a memory limit, a loop costs one build, not
// the service: the child is killed and the build reported as failed.
//
// The child is this same program, started with ChildArg. It gets a minimal
// environment, so none of the parent's secrets (HOTDOG_EDITOR_SECRET, tokens,
// client secrets) are in reach of the build at all.
package isolate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
)

// ChildArg is the hidden first argument that makes the program a build child.
const ChildArg = "__hotdog-build-child"

// Enabled turns isolation on. The command line sets it for the services;
// tests and a person's own builds run in-process.
var Enabled = false

// Limits for one child.
var (
	BuildTimeout  = 5 * time.Minute
	RenderTimeout = 30 * time.Second
	MemoryLimit   = int64(2 << 30)
)

// Summary is what a build reports back.
type Summary struct {
	Out           string
	Pages, Files  int
	Feeds         []string
	NextScheduled time.Time
	Duration      time.Duration
}

type request struct {
	Mode    string // build or render
	Opt     build.Options
	Source  string
	Content []byte
}

type response struct {
	Summary *Summary
	HTML    []byte
	Path    string
	Err     string
}

// Build builds a site, in a child process when Enabled.
func Build(opt build.Options) (*Summary, error) {
	if !Enabled {
		return summarize(build.Run(opt))
	}
	res, err := run(request{Mode: "build", Opt: opt}, BuildTimeout)
	if err != nil {
		return nil, err
	}
	return res.Summary, nil
}

// RenderPage renders one page as edited, in a child process when Enabled.
func RenderPage(siteDir, source string, content []byte, opt build.Options) ([]byte, string, error) {
	if !Enabled {
		return build.RenderPage(siteDir, source, content, opt)
	}
	opt.SiteDir = siteDir
	res, err := run(request{Mode: "render", Opt: opt, Source: source, Content: content}, RenderTimeout)
	if err != nil {
		return nil, "", err
	}
	return res.HTML, res.Path, nil
}

func summarize(res *build.Result, err error) (*Summary, error) {
	if err != nil {
		return nil, err
	}
	return &Summary{Out: res.Out, Pages: res.Pages, Files: res.Files, Feeds: res.Feeds, NextScheduled: res.Site.NextScheduled, Duration: res.Duration}, nil
}

func run(req request, timeout time.Duration) (*response, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, ChildArg)
	cmd.Env = childEnv()
	cmd.Stdin = bytes.NewReader(body)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &limitedWriter{w: &errb, n: 64 << 10}
	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("the build took longer than %s and was stopped; is a template looping?", timeout)
	}
	var res response
	if jerr := json.Unmarshal(out.Bytes(), &res); jerr != nil {
		if strings.Contains(errb.String(), "out of memory") || strings.Contains(errb.String(), "cannot allocate memory") {
			return nil, fmt.Errorf("the build ran out of memory (over %d MB) and was stopped; is a template looping?", MemoryLimit>>20)
		}
		if err != nil {
			return nil, fmt.Errorf("the build stopped: %v", err)
		}
		return nil, fmt.Errorf("the build's answer couldn't be read: %v", jerr)
	}
	if res.Err != "" {
		return nil, errors.New(res.Err)
	}
	return &res, nil
}

// childEnv passes on only what a build needs: nothing secret.
func childEnv() []string {
	var env []string
	for _, k := range []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "TZ", "XDG_CACHE_HOME", "SYSTEMROOT"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// Child is the child's side: read one request, build, answer, exit.
func Child() {
	debug.SetMemoryLimit(MemoryLimit * 3 / 4)
	limitMemory(uint64(MemoryLimit))
	var req request
	res := response{}
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 64<<20)).Decode(&req); err != nil {
		res.Err = "bad request: " + err.Error()
	} else {
		switch req.Mode {
		case "build":
			s, err := summarize(build.Run(req.Opt))
			res.Summary = s
			if err != nil {
				res.Err = err.Error()
			}
		case "render":
			html, p, err := build.RenderPage(req.Opt.SiteDir, req.Source, req.Content, req.Opt)
			res.HTML, res.Path = html, p
			if err != nil {
				res.Err = err.Error()
			}
		default:
			res.Err = "unknown mode"
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(res)
	os.Exit(0)
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	if len(p) > l.n {
		p = p[:l.n]
	}
	l.n -= len(p)
	return l.w.Write(p)
}
