# Editor roadmap

Draft, 2026-10-10; decisions taken the same day. The editor is the part of HotDog CMS that people who don't
live in git will use. It should do what people actually use WordPress for,
without what makes WordPress heavy and dangerous.

## The rules it is built on

These come from John's decisions so far, and every milestone below has to
keep them.

1. **The editor never lives with the sites.** It is its own deployment, on
   its own host or on the editor's own machine, like inbuxa-admin. The public
   host serves files (plus the forms and search endpoint) and nothing else:
   no `/wp-admin`, no login page, no admin API. Taking over a web server gives
   an attacker no way to edit content. Taking over the editor gives no way
   into a web server, because the editor holds no deploy credentials.
2. **Git is the delivery path for every change, the visual editor included.**
   - Saving is a commit, made as the person who saved.
   - A draft is a branch, and its preview is that branch's preview address.
   - Submitting for review opens a pull request on whichever platform the site
     lives on (Gitea, GitHub, GitLab, Forgejo, Bitbucket), or on plain git.
   - Publishing is that change reaching the branch the site builds from.
   - Undoing is a revert.

   There is no second, faster path to the live site.
3. **Operators decide how sites are published.** The editor hands a commit to
   git. What happens next (CI, a deploy script, the pull agent, a container
   registry) is the operator's choice, on bare metal, a VPS or containers.
4. **One safety check, three ways in.** Power users get a keyboard-driven,
   CLI-like environment. Novices get WYSIWYG. Everyone else gets Markdown with
   a live preview, and forms for structured parts. All three produce the same
   files and go through the same `hotdog-cms check`, preview and review. No mode
   skips a check, and no mode has a shortcut to the live site.
5. **Security and usability are never traded.** If a feature seems to need
   that trade, the design is wrong.
6. **No data store until something truly needs one.** Each milestone says
   whether it does.

## Milestones

Each milestone is usable on its own. They're in build order: the in-between
interface first, because the other two are built on its plumbing.

### E0 · Foundations

*Built 2026-10-10:*
- **Server:** `hotdog-cms editor` (Go), with the React + TypeScript interface
  embedded.
- **Sign-in:** OAuth with PKCE for all five platforms; branches, reviews and
  permissions read from Gitea, Forgejo and GitHub.
- **Sessions:** AES-GCM sealed cookies.
- **Workspaces:** per-person checkouts made with the person's own token, then
  built and checked.
- **Interface:** sites list, a site view (pages by collection, drafts,
  reviews, check results), and a page view with a live preview beside it.
- **Security:** a strict CSP on the editor itself, a CSRF guard on anything
  that changes state, and page sources confined to `content/`.
- **Tests:** a fake platform covers the whole sign-in and browsing flow.
- **Tried against the Coffey Labs Gitea** with an OAuth app, "cl-cms editor
  (local test)", whose only redirect is loopback. John authorized it, and
  every view worked against the real repository. For the page view's preview
  pane, `hotdog-cms preview -frame <editor origin>` lets the editor, and only the
  editor, frame previews.

- `hotdog-cms editor`: a separate server, by default listening on loopback, with
  its own strict CSP.
- **Sign-in through the git platform** (OAuth with Gitea, GitHub, GitLab,
  Forgejo or Bitbucket). Edits are made with the signed-in
  person's own platform token, so the platform enforces who may do what.
  Every commit is attributed to them, and the editor stores no accounts.
  **No data store:** sessions are signed, short-lived cookies.
- Open a site: browse its files, its pages by collection, and its open drafts
  and pull requests.
- Embedded previews (`hotdog-cms preview`), with each branch's check results.

### E1 · Write (the in-between interface)

*Built 2026-10-10:*
- **Page editor:** common front matter as a form, or all of it as YAML
  (edits keep key order and comments); the body as Markdown; the page's check
  findings inline.
- **Live preview:** rendered by the build engine as you type, served from its
  own origin (`<token>.localhost:8191`) so a site's scripts can't reach the
  editor's session.
- **Viewport picker** (John): desktop, tablet and mobile, both while editing
  and in a full-window **Present** mode for demos; desktop scales to fit; the
  choice is remembered. Present opens in its own tab and follows the edit as
  it's typed.
- **Light, dark or automatic** (John): a switch in the header, remembered
  per browser and applied before the page draws; automatic follows the device.
- **Resizable split** (John): the divider between editor and preview drags,
  moves with the arrow keys, and resets on double-click; the width is
  remembered per browser.
- **Save** commits as the person. The first save on the publishing branch
  starts a `draft/<page>` branch; nothing is ever written to the publishing
  branch. A save based on a commit the branch has moved past is refused.
- **New page** starts its own draft.
- **Submit for review** opens a pull request with the preview link and the
  check summary.
- **Publish** merges it only if the draft passes the checks, and the platform
  still decides who may merge.
- **Scheduled publishing:** `publish_at`, honored by the build, the pull agent
  (which rebuilds when a page comes due) and the CI templates (an hourly run).
- **Not yet:** GitLab and Bitbucket reviews (E0's platform note), and media
  (E2).

- Markdown with a live preview rendered by the same engine as the build, so
  what you see is exactly what publishes.
- Front matter as a form, driven by what the site's layouts use (title,
  summary, date, tags with suggestions from existing terms, image and alt
  text). Raw front matter is one click away.
- Save creates the draft branch (`draft/<slug>`) on first save; later saves
  add commits to it.
- Check results appear inline, next to the field or line they're about.
  Errors block "submit", and the reason is shown in words.
- **Submit for review** opens a pull request with a preview link (the commit
  status we already post). **Publish** merges it, for people the platform
  allows to merge.
- **Scheduled publishing**, kept in git: front matter `publish_at`, honored by
  the build and the pull agent; previews show what will appear when. **No
  data store.**

### Quick posts (wanted, not yet designed)

John, 2026-10-10: a "5 minute" tool for pushing something out in a hurry:
events, announcements, specials, whatever an operator needs up now. To be
workshopped; for now it only holds a place on the list. Whatever shape it
takes, it keeps the rules above: a quick post is still a commit, still checked,
still previewable, with no faster path to the live site.

### Cloudflare (wanted, not yet designed)

John, 2026-10-10: manage DNS, certificates and domain registration, and
deploy to Cloudflare (Workers with static assets, and Pages, which Cloudflare
is folding into Workers) directly from the CMS. To be designed. The idea, and
the rules it has to keep:
- **Deploying is a publish target.** `hotdog-cms publish` gets a
  `cloudflare` target: Workers static assets or Pages, with our redirects and
  headers (CSP included) written in Cloudflare's formats. Like every target,
  it runs where the operator publishes from (CI, the pull agent, a laptop),
  and only after the checks pass. From the editor, "Publish" merges, and the
  operator's pipeline deploys. **The editor holds no Cloudflare token:**
  taking over the editor still gives no way into the site.
- **Previews:** each draft branch as a Cloudflare preview deployment, as an
  alternative to `hotdog-cms preview`, linked from the review the same way.
- **DNS, certificates and domains:** an operator's tool rather than an
  editor's, run with a token scoped to the zones it manages:
  - point a site's domain at its target;
  - check the certificate and the SSL mode (Full (strict));
  - Always Use HTTPS, HSTS, DNSSEC;
  - purge only the changed addresses after a publish.

  Changes are shown before they're applied, like a diff. Domain registration
  goes through Cloudflare Registrar, as far as its API allows; check that
  before promising it.
- **The editor's part:** read-only status beside each site (domain resolves
  here, certificate valid until, last deploy). Two of `check`'s rules lean on
  Cloudflare: Email Address Obfuscation (`cloudflare-email`) and cache rules.
  They could read the zone's real settings instead of assuming.
- **Not Cloudflare-only:** the same shape should fit other DNS hosts and CDNs
  later; Cloudflare first because we run on it.
- **Possible data-store point:** none expected. Tokens live in the operator's
  environment or CI secrets, as deploy credentials do today.

### Mail servers: inbuxa and others (wanted, not yet designed)

John, 2026-10-10: plug straight into inbuxa, or a similar mail server, to set
up email for forms and the rest, securely. Today the forms endpoint sends
through any SMTP server, with a password in its environment. The idea:
- **Connect once, least privilege.** The operator connects the endpoint to
  their mail server. What it holds is a credential that can only send, only
  from the site's own address (`forms@example.org`), only to the form's
  recipients, at a limited rate. It never reads a mailbox.
  - On inbuxa: an OAuth client (inbuxa only accepts registered clients)
    sending through JMAP. Where inbuxa lacks a send-only scope, that's an
    inbuxa feature to add (and a divergence-log entry).
  - Elsewhere: SMTP submission over TLS, and JMAP where the server speaks it.
- **The credential lives with the endpoint,** in the operator's environment,
  like every other secret: never in the editor, never in a repository.
- **Setup helper:**
  - create the sending address or identity;
  - check the domain's mail records (SPF, DKIM, DMARC), showing what's
    missing rather than changing DNS on its own;
  - send a test message.

  Where the mail server manages DNS itself, as inbuxa does for DKIM, defer to
  it.
- **Submissions into the mail server:** optionally, deliver each submission
  as a message filed into a mailbox (a Sieve rule files it into "Forms"). The
  mail server's spam filtering and search then do the work, and the E7
  submissions viewer could read that mailbox with a read-only grant instead
  of the JSON-lines files. **No data store:** the mail server is the store.
- **Other mail functions, later:** the "thanks, we got it" reply from the
  site's address, newsletter sign-up handed to the mail server's lists, and
  `security.txt` and privacy-page contacts checked against addresses that
  actually exist.
- **inbuxa first** because it's ours; then generic JMAP and SMTP servers, and
  maybe sending services (Postmark, SES) through the same narrow interface.

### Several accounts and repositories

*Built 2026-10-10* (John: operators may have several accounts, on one git
platform or several, with sites spread across them):
- **Several accounts at once:** each account is its own sealed cookie, up to
  eight, because a Gitea token alone can be much of a cookie's 4 KB.
  - Signing in again adds an account; the same account again just refreshes.
  - The header menu signs out one account or all of them.
  - **Still no data store:** the browser holds the accounts.
- **A second account on the same platform:** "Add an account" asks GitHub to
  let the person pick (`prompt=select_account`). Other platforms sign in
  whoever is signed in there, and the menu says so.
- **Sites from every account:** each configured site is checked against
  every signed-in account on its platform (answers cached for a minute). The
  sites list shows which accounts can edit and which can only read.
- **Working as:** when several accounts can open a site, the site view has a
  picker, remembered per browser and sent with every request about that site.
  The default is the first account that can write.
- **Separate per account:** commits, checkouts and the live preview. When a
  platform stops taking one account's token, only that account is signed out.
- **Finding sites** (built the same day, John: "let's try the auto finder"):
  with `discover` on for a platform, every repository a signed-in account can
  reach that holds a HotDog CMS `site.yaml` is offered beside the listed
  sites. It can be narrowed to some owners, and results are cached for ten
  minutes. It's off by default. See [editor.md](editor.md). Tried on the
  Coffey Labs Gitea with a throwaway repository (since deleted).

### E2 · Media

*Built 2026-10-10:*
- **Re-encoding** (`internal/media`):
  - uploads are decoded by Go's own decoders, never by C;
  - turned upright, then stripped of EXIF, location, comments and anything
    after the image data;
  - published at 480, 960 and 1600 px wide, each with a WebP copy when that's
    smaller (38 to 60% smaller on real photos). The WebP encoder is libwebp
    translated to pure Go and vendored in `internal/media/webpenc`: no system
    library, no cgo.
  - SVG is refused, and a file claiming to be enormous is refused before it's
    decoded.
- **Upload commits to the draft,** as the person. From the publishing branch
  it starts the page's draft and carries unsaved text across.
- **Insert picture:** from the button, or by dropping or pasting a picture
  onto the body. Upload or pick from the library. Alt text is required, or an
  explicit "decorative" choice. A library picture brings the alt text it was
  given before.
- **Pictures library:** a page per site listing every picture, its sizes and
  weight, which files use it, and what's unused. Thumbnails come from the
  person's checkout, as sandboxed images only.
- **Responsive markup in the build:** a Markdown image of a `media/` picture
  becomes a `<picture>` with the WebP copies, `srcset`, `width` and `height`,
  and lazy loading (`markdown.image_sizes` sets `sizes`).
- **New checks:** `image-location` (a JPEG carrying GPS, an error) and
  `image-weight` (over 500 KB, a warning).
- **Tried end to end** on a throwaway repository on the Coffey Labs Gitea
  (since deleted).
- **Not yet:** deleting unused pictures from the library, and object storage
  for sites with a lot of media.

- Upload by drag and drop. Every image is re-encoded on the way in:
  - EXIF stripped (location included);
  - resized to sensible widths, with WebP and a fallback;
  - so a "picture" can't carry anything else.
- An image can't be inserted without alt text, or an explicit "decorative"
  choice.
- A media library, with what's used where and what's unused.
- **Storage:** committed to the site's repository by default, as today.
  **Possible data-store point:** sites with a lot of large media may want
  object storage (S3-compatible) or Git LFS instead. Decide when a real site
  needs it.

### E3 · Sections and look

- The section library, shown as cards. A section is added from the library,
  reordered by drag and drop, and its fields edited as a form. This is the
  cairnobs.org home page, editable without YAML.
- New section types are template files, so a developer adds one and every
  editor can use it.
- Palette, fonts and logo edited through `site.yaml` params with a live
  preview, inside limits the theme declares (contrast is checked).

### E4 · Power interface

- Keyboard-first throughout, with a command palette (`Ctrl K`): new article,
  open, preview, check, submit, publish.
- A command line in the browser that speaks HotDog CMS (`new article "Title"`,
  `check`, `publish`), plus git-shaped commands for branches, diffs and
  history. It doesn't run arbitrary shell commands: the editor host is not a
  shell.
- Any file in the site is editable as raw text, with diffs before saving.
- Everything the editor does is something `hotdog-cms` on a laptop can do too.
  The power interface is a convenience, never a requirement.

### E5 · WYSIWYG interface

- A visual editor built on a Markdown-native ProseMirror editor, so the
  Markdown that comes out is the Markdown a person would have written.
  Nothing becomes HTML soup.
- Editing in place on the preview: click a heading on the rendered page to
  edit it.
- Only what the site's Markdown settings allow. If raw HTML is off, the
  toolbar can't produce it.

### E6 · Helpers

- **SEO panel:**
  - a search-result and social-card preview per page;
  - the check's SEO findings as a checklist (title and description length,
    duplicates, missing alt text, canonical, noindex);
  - redirects managed in a table, with a redirect added automatically when a
    page moves.
- **Social:**
  - a social card generated for every page from a template, like coffeylabs'
    `cards.py`;
  - per-platform previews (Mastodon, LinkedIn, Bluesky);
  - optional share-on-publish, per post, with nothing automatic;
  - fediverse author tags.
- **Analytics wizard:**
  - Google Analytics, Plausible or Matomo, set up consent-gated by default:
    the script loads only after "Accept";
  - the privacy page updated to match;
  - ungated analytics needs an explicit, recorded choice, and `check` still
    warns about it.
- **GDPR and cookie helper:**
  - the consent banner our sites use, as a built-in: in the accessibility tree
    and not a wall, Decline first and equally prominent, a "Cookie settings"
    reopen link (Article 7(3)), and declining deletes the cookies;
  - a privacy page generator that lists what the site actually loads, the
    lawful basis for each, the controller contact and the date, regenerated
    when that list changes.
- **Search consoles:**
  - Google Search Console, Bing Webmaster Tools and Yandex: guided
    verification by meta tag, file or DNS TXT record (shown, not changed);
  - sitemap submission;
  - IndexNow pings on publish.
- **The gotchas:** every `check` rule shown where it applies, with its fix.
  Examples: an address Cloudflare will mangle, an image missing alt text, a
  third-party script with no consent, a `security.txt` about to expire.

### E7 · Forms

- A form builder over the YAML (fields, validation, recipient, success
  page), with a live preview of the form in the site's styling.
- **Submissions viewer**, reading the endpoint's JSON-lines files over an
  authenticated, editor-only channel. It never goes through the public
  endpoint, which stays write-only.
  **Possible data-store point:** searching or filtering many submissions.
  The files are fine for reading recent ones.

### E8 · People and workflow

- Roles (writer, editor, publisher) mapped to the platform's own
  permissions: who can push to drafts, who can merge. One source of truth,
  and the editor stores none of it.
- Notifications for review requests and published changes, sent by the
  platform's own notifications or email.
- History is `git log`. An audit trail can't be edited by whoever it
  records.
- Passkeys and two-factor sign-in, provided by the platform or the OIDC
  provider.

### E9 · Importers

- **WordPress** (`hotdog-cms import wordpress`):
  - connects to a live site with an Application Password (read-only, over the
    REST API), or reads a WXR export file;
  - posts and pages become Markdown, with `format: html` kept where
    conversion would lose something;
  - authors, dates, slugs, tags and categories are kept;
  - media is downloaded and re-encoded;
  - every old permalink becomes a redirect;
  - a report lists what didn't convert cleanly (shortcodes, embeds,
    page-builder markup).

  Credentials are used for the import and never stored.
- **Hugo, Jekyll, Eleventy:** mostly a front matter mapping. **Ghost:** its
  JSON export.

### E10 · Extensions without the plugin problem

- Sections, templates and build-time steps that run in a sandbox (WASI, no
  network, no filesystem outside the site), never in the page path or the
  editor's process.
- An extension declares what it reads and writes, and the editor shows that
  before it's enabled.
- To be designed carefully before it's built. This is where WordPress's
  security problems came from.

## Decisions (John, 2026-10-10)

| Question | Decided | Why |
|---|---|---|
| How people sign in | Through the git platform (OAuth with Gitea, GitHub, GitLab, Forgejo, Bitbucket); no local accounts | No user store to secure; the platform's permissions and attribution come for free |
| The editor's front end | React + TypeScript | The stack inbuxa-admin and the webmail already use |
| The visual editor | A Markdown-native ProseMirror editor | Markdown in, Markdown out, with no lossy HTML step |
| Media storage | In the site's repository, re-encoded; object storage as an option later | No new service until a site needs one |
| Editor for hosting customers | One editor per customer first; multi-tenant later | Multi-tenant is where a real data store first becomes necessary |

## Where a data store comes in

Nothing up to E5 needs one: auth is delegated, drafts and schedules live in
git, and sessions are signed cookies. The first real candidates are, in
order:

1. **Multi-tenant hosting.** Knowing which customer owns which sites, across
   many editors.
2. **Large media.** Object storage rather than git, for sites with a lot of
   video or images.
3. **Searching form submissions.** Past what reading files comfortably does.
4. **A shared rate limit across endpoints.** From the forms work.

Each will be raised when a feature reaches it, not before.

## What exists already

Everything the editor builds on is done and tested:
- the build, with sections, collections, data files and forms;
- previews per branch;
- the platform layer (webhooks, commit statuses, CI);
- publishing to any target;
- the endpoint;
- the check that gates every route.

The editor is a new front end over these, not a new system.
