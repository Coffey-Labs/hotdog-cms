// Package ci writes the pipeline file each platform runs: check a site on
// every pull request, publish it when its branch moves. For plain git there is
// no CI, so it writes a post-receive hook that tells hotdog-cms a branch moved.
//
// Published targets come from publish.yaml; the values it reads from the
// environment (${DEPLOY_HOST} and the like) are the pipeline's secrets. For an
// rsync target the pipeline loads a deploy key and checks the server against a
// known_hosts entry kept as a secret, never with host key checking turned off.
package ci

import (
	"fmt"
	"sort"
	"strings"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/about"
)

// Template is one pipeline file.
type Template struct {
	Path string // where it goes, relative to the repository root
	Body string
	Note string // what to set up on the platform
}

// install puts hotdog-cms on the runner: the version that wrote the file, so
// a pipeline doesn't change under you, or the latest from a development
// build.
var install = func() string {
	v := about.Version
	if v == "" || v == "dev" {
		v = "latest"
	}
	return "go install git.coffeylabs.org/coffey-labs/hotdog-cms/cmd/hotdog-cms@" + v
}()

// sshSetup loads a deploy key for rsync targets; a no-op without one.
const sshSetup = `if [ -n "${DEPLOY_KEY:-}" ]; then
  install -d -m 700 ~/.ssh
  printf '%s\n' "$DEPLOY_KNOWN_HOSTS" > ~/.ssh/known_hosts
  eval "$(ssh-agent -s)"
  printf '%s\n' "$DEPLOY_KEY" | ssh-add -
fi`

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

// GitHub-syntax workflows (GitHub, Gitea and Forgejo Actions). Actions are
// pinned to commits; tags can be moved, commits can't.
func actions(platform, checkout, setupGo string) string {
	// On GitHub the deploy secrets live in an environment that only the
	// publishing branch can use: anyone who can push a branch can change
	// this file in it, so secrets the whole repository can use would be
	// theirs too.
	env := ""
	if platform == "github" {
		env = "\n    environment: production"
	}
	return `# Written by ` + "`hotdog-cms ci " + platform + "`" + `. Checks the site on every pull request and
# publishes it to the targets in publish.yaml when {{BRANCH}} moves.
name: site
on:
  pull_request:
  push:
    branches: [{{BRANCH}}]
  # Hourly, so pages with a publish_at go live when their time comes.
  schedule:
    - cron: "17 * * * *"
permissions:
  contents: read
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: ` + checkout + `
      - uses: ` + setupGo + `
        with:
          go-version: stable
      - name: Install hotdog-cms
        run: ` + install + `
      - name: Check
        run: hotdog-cms check -site {{SITE}}
  publish:
    needs: check
    if: (github.event_name == 'push' || github.event_name == 'schedule') && github.ref == 'refs/heads/{{BRANCH}}'
    runs-on: ubuntu-latest` + env + `
    steps:
      - uses: ` + checkout + `
      - uses: ` + setupGo + `
        with:
          go-version: stable
      - name: Install hotdog-cms
        run: ` + install + `
      - name: Build
        run: hotdog-cms check -site {{SITE}}
      - name: Publish
        env:
          DEPLOY_HOST: ${{ secrets.DEPLOY_HOST }}
          DEPLOY_KEY: ${{ secrets.DEPLOY_KEY }}
          DEPLOY_KNOWN_HOSTS: ${{ secrets.DEPLOY_KNOWN_HOSTS }}
        run: |
` + indent(sshSetup, 10) + `
          hotdog-cms publish -site {{SITE}} -no-build -all
`
}

const ghCheckout = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1"
const ghSetupGo = "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0"

var templates = map[string]Template{
	"github": {
		Path: ".github/workflows/site.yml",
		Body: actions("github", ghCheckout, ghSetupGo),
		Note: "Create an environment named production under Settings › Environments, limit it to the {{BRANCH}} branch (Deployment branches), and add the secrets publish.yaml reads there (DEPLOY_HOST, and DEPLOY_KEY + DEPLOY_KNOWN_HOSTS for rsync), not as repository secrets: anyone who can push a branch can read repository secrets from it.",
	},
	"gitea": {
		Path: ".gitea/workflows/site.yml",
		Body: actions("gitea", "https://github.com/"+ghCheckout, "https://github.com/"+ghSetupGo),
		Note: "Needs Actions enabled on the repository and a runner with the ubuntu-latest label. Add the secrets under Settings › Actions › Secrets. Anyone who can push a branch to this repository can read those secrets from it; if people who aren't maintainers can push, publish with the pull agent instead (no secrets in CI).",
	},
	"forgejo": {
		Path: ".forgejo/workflows/site.yml",
		Body: actions("forgejo", "https://github.com/"+ghCheckout, "https://github.com/"+ghSetupGo),
		Note: "Needs Actions enabled on the repository and a runner with the ubuntu-latest label (on Codeberg, request access to its runners). Add the secrets under Settings › Actions › Secrets. Anyone who can push a branch to this repository can read those secrets from it; if people who aren't maintainers can push, publish with the pull agent instead (no secrets in CI).",
	},
	"gitlab": {
		Path: ".gitlab-ci.yml",
		Body: `# Written by ` + "`hotdog-cms ci gitlab`" + `. Checks the site on every merge request and
# publishes it to the targets in publish.yaml when {{BRANCH}} moves.
default:
  image: golang:1.26-alpine
  before_script:
    - apk add --no-cache git rsync openssh-client
    - ` + install + `

site:check:
  stage: test
  script:
    - hotdog-cms check -site {{SITE}}
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_BRANCH == "{{BRANCH}}"

# Add a pipeline schedule (hourly) under Build › Pipeline schedules so pages
# with a publish_at go live on time.
site:publish:
  stage: deploy
  needs: [site:check]
  script:
    - |
` + indent(sshSetup, 6) + `
    - hotdog-cms publish -site {{SITE}} -all
  rules:
    - if: $CI_COMMIT_BRANCH == "{{BRANCH}}"
`,
		Note: "Add DEPLOY_HOST (and DEPLOY_KEY + DEPLOY_KNOWN_HOSTS for rsync) as masked, protected variables under Settings › CI/CD › Variables, and protect {{BRANCH}}: protected variables reach only pipelines on protected branches, not branches anyone can push.",
	},
	"bitbucket": {
		Path: "bitbucket-pipelines.yml",
		Body: `# Written by ` + "`hotdog-cms ci bitbucket`" + `. Checks the site on every pull request and
# publishes it to the targets in publish.yaml when {{BRANCH}} moves.
image: golang:1.26
definitions:
  steps:
    - step: &check
        name: Check
        script:
          - apt-get update -qq && apt-get install -y -qq rsync openssh-client >/dev/null
          - ` + install + `
          - hotdog-cms check -site {{SITE}}
pipelines:
  pull-requests:
    '**':
      - step: *check
  branches:
    {{BRANCH}}:
      - step:
          name: Publish
          deployment: production
          script:
            - apt-get update -qq && apt-get install -y -qq rsync openssh-client >/dev/null
            - ` + install + `
            - hotdog-cms check -site {{SITE}}
            - hotdog-cms publish -site {{SITE}} -no-build -all
`,
		Note: "Enable Pipelines, add DEPLOY_HOST as a secured deployment variable, and for rsync use Repository settings › SSH keys (Pipelines loads that key and its known hosts itself).",
	},
	"woodpecker": {
		Path: ".woodpecker/site.yaml",
		Body: `# Written by ` + "`hotdog-cms ci woodpecker`" + `. Checks the site on every pull request and
# publishes it to the targets in publish.yaml when {{BRANCH}} moves.
when:
  - event: pull_request
  - event: push
    branch: {{BRANCH}}
  # A cron job named in Woodpecker's settings, so scheduled pages go live on time.
  - event: cron
steps:
  - name: check
    image: golang:1.26-alpine
    commands:
      - apk add --no-cache git
      - ` + install + `
      - hotdog-cms check -site {{SITE}}
  - name: publish
    image: golang:1.26-alpine
    when:
      - event: [push, cron]
        branch: {{BRANCH}}
    environment:
      DEPLOY_HOST: { from_secret: deploy_host }
      DEPLOY_KEY: { from_secret: deploy_key }
      DEPLOY_KNOWN_HOSTS: { from_secret: deploy_known_hosts }
    commands:
      - apk add --no-cache git rsync openssh-client
      - ` + install + `
      - |
` + indent(sshSetup, 8) + `
      - hotdog-cms publish -site {{SITE}} -all
`,
		Note: "Add deploy_host (and deploy_key + deploy_known_hosts for rsync) as repository secrets in Woodpecker, limited to the push and cron events on {{BRANCH}}, and leave them unavailable to pull requests.",
	},
	"git": {
		Path: "post-receive",
		Body: `#!/bin/sh
# Written by ` + "`hotdog-cms ci git`" + `. Install it as hooks/post-receive in the bare repository
# (chmod +x). On every push it tells hotdog-cms which branch moved, so previews
# (or a pull agent) rebuild at once instead of at their next check.
#
# The token is read from a header file, not passed on the command line, so it
# never appears in the process list. Create it once, readable only by git:
#   printf 'X-HotDog-Token: %s\n' "$(openssl rand -hex 32)" > /etc/hotdog-cms/hook-header
# and give hotdog-cms the same token as HOTDOG_HOOK_SECRET.
URL="${HOTDOG_HOOK_URL:-https://preview.example.org/_hotdog/hook}"
HEADER=/etc/hotdog-cms/hook-header
while read -r old new ref; do
  curl -fsS --max-time 10 -X POST -H "@$HEADER" \
    --data-urlencode "ref=$ref" --data-urlencode "after=$new" "$URL" >/dev/null ||
    echo "hotdog-cms: could not reach $URL; the next scheduled check will catch up" >&2
done
`,
		Note: "Copy it to <repo>.git/hooks/post-receive on the git server and make it executable. Previews and pull agents keep their scheduled check as well, so a missed hook only delays a rebuild.",
	},
}

// Platforms lists what Get knows.
func Platforms() []string {
	out := make([]string, 0, len(templates))
	for k := range templates {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Get returns a platform's file for a site folder and branch.
func Get(platform, site, branch string) (Template, error) {
	t, ok := templates[platform]
	if !ok {
		return Template{}, fmt.Errorf("no template for %q (one of %s)", platform, strings.Join(Platforms(), ", "))
	}
	if site == "" {
		site = "."
	}
	if branch == "" {
		branch = "main"
	}
	r := strings.NewReplacer("{{SITE}}", site, "{{BRANCH}}", branch)
	t.Body = r.Replace(t.Body)
	t.Note = r.Replace(t.Note)
	return t, nil
}
