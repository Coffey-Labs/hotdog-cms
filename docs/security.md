# Security

HotDog CMS exists to be as easy as WordPress without its security problems.
This page says what it protects, from whom, and how. To report a problem,
see [SECURITY.md](../SECURITY.md).

## The shape

- **The public site is files.** No login page, no admin, no database, no
  code running per visit. The only moving part a site may have is the forms
  endpoint (forms and search), and that's optional.
- **The editor never runs on the site's host.** It changes a site only by
  committing to its git repository, as the person signed in, with their own
  token. It holds no publishing credentials.
- **The git platform decides who may do what.** Nobody has an account in the
  editor. Reading, writing and publishing are whatever the platform allows
  that person, and every change is a commit with their name on it.

## Who we defend against

| Who | What they can't do |
|---|---|
| Anyone on the internet | Reach an editor without signing in through the platform; send mail through the forms endpoint to anyone but the site's own recipients; flood it past the rate limits. |
| Someone who can read a site | Change it, or read anything on the editor's machine beyond the site. |
| Someone who can write to a site (not a maintainer) | Publish without the checks passing; change publish targets or CI pipelines through the editor, or publish such a change; make a build read or write files outside the site; take the editor down with a template. |
| Someone who controls a repository you open | Read your session, act as you, or reach the editor's machine. |
| A pack | Carry code: packs are templates and styles, checked as HTML before they're installed. |

## How

**Site content is data.** Anyone who can write to a site can change its
templates, so builds treat everything in a site as untrusted:
- No template function reads outside the site folder or reaches the
  network. The only HTML that reaches a page unescaped comes through the
  Markdown renderer, under the site's raw-HTML setting.
- Symbolic links are never followed. Git checks them out as plain files, and
  a build refuses a site that contains one.
- Page addresses, feed paths and redirects are validated, so a build writes
  only inside its output folder, and redirects can't inject web server
  configuration.
- A rendered page is capped in size.
- The editor, the preview server and the pull agent build in a child
  process with a time limit, a memory limit and none of the parent's
  secrets. A runaway template costs one build, not the service.

**The editor:**
- Sessions are encrypted cookies (AES-GCM). Over https they're `Secure`,
  `HttpOnly`, `SameSite=Lax` and `__Host-` prefixed.
- Every request that changes something must come from the editor's own
  pages (Origin check plus a custom header).
- Sign-in uses OAuth with PKCE, and returns only to a path on the editor.
- Pages are served with a strict Content-Security-Policy. The live preview
  runs on its own origin.
- Every file read or write in a checkout goes through `os.Root`, so nothing
  outside it is reachable.
- Branch names that git could read as options are refused.
- Only maintainers may change `publish.yaml` or CI pipelines, in the editor
  or by publishing a draft that does.

**The forms endpoint:**
- Signed, time-limited tokens; a trap field; an Origin check; and rate
  limits per address (IPv6 counted per /64).
- No header injection.
- Recipients can be limited to an operator-owned list.
- Subjects are plain field substitution.
- Searches are rate-limited and bounded.

**Publishing:**
- `dir` and `rsync` replace only folders HotDog CMS wrote.
- Operator commands run without a shell and live only in the operator's
  targets file.
- The CI files it writes keep deploy secrets away from branches anyone can
  push (an environment limited to the publishing branch on GitHub, protected
  variables on GitLab). On Gitea and Forgejo, where that isn't possible,
  they say so and point to the pull agent.

**Reviewed:** before the first release, the editor, the build engine, the
forms endpoint, publishing and the preview server were reviewed with
attacks tried against each. Every finding was fixed and has a test.
