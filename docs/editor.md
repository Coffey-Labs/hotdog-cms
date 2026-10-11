# The editor

`hotdog-cms editor` is where people who don't live in git work on sites:
pages, pictures, drafts, reviews and publishing. It's a server of its own,
and it never runs on the host that serves the sites. People sign in with the
git platform the sites live on, and every change is a commit made as them.

    HOTDOG_EDITOR_SECRET=$(openssl rand -hex 32) \
    HOTDOG_GITEA_SECRET=… \
    hotdog-cms editor -config editor.yaml

It listens on two addresses: the editor itself, and the live preview. The
live preview is a separate origin, so a site's own scripts can't reach the
editor's session. Put both behind your web server with TLS, or keep them on
loopback for one person on one machine.

## editor.yaml

```yaml
listen: 127.0.0.1:8190               # the editor
public_url: https://edit.example.org # how browsers reach it; sign-in returns here
cache: /var/cache/hotdog-cms/editor   # people's checkouts (default: the user cache folder)

forges:                              # the platforms people sign in with
  - host: git.example.org
    kind: gitea                      # gitea, forgejo, github, gitlab or bitbucket
    client_id: 6a8b…
    client_secret_env: HOTDOG_GITEA_SECRET
    discover: true                   # also offer sites found in people's repositories

sites:                               # sites offered to everyone who can open them
  - name: Example
    repo: https://git.example.org/me/example.git
    branch: main                     # the branch the site publishes from
    preview: { domain: preview.example.org }  # where `hotdog-cms preview` serves branches

live:                                # the live preview of pages being edited
  listen: 127.0.0.1:8191
  domain: live.example.org           # previews are <token>.live.example.org
```

### Signing in

Each platform needs an OAuth application, registered by whoever runs the
editor:
- its redirect URL is `<public_url>/auth/callback`;
- its secret goes in the variable that `client_secret_env` names, never in
  the file.

Nobody has an account in the editor itself. What someone can open and change
is what the platform already lets them do. Sessions are encrypted cookies,
sealed with `HOTDOG_EDITOR_SECRET`; every editor instance serving the same
people needs the same secret.

**Several accounts:** a person can be signed in to up to eight accounts at
once, on one platform or several, from the menu in the header. When more than
one of their accounts can open a site, the site view says which one they're
working as, and lets them switch.

### Finding sites

`discover` offers, beside the sites listed under `sites`, every repository
the signed-in person can reach that holds a HotDog CMS site: a `site.yaml` at
the top of its default branch, with a `url` in it. Nothing is read that the
person couldn't read anyway.

```yaml
    discover: true                         # any repository they can reach
    discover: { owners: [acme, me] }       # only these users' and organizations' repositories
    discover: { owners: [acme], limit: 300 } # look at up to 300, most recently updated first
```

- It's off unless set. Leave it off to offer only the sites you list, as a
  host for customers might.
- Only Gitea, Forgejo and GitHub are searched for now.
- The editor looks at the 100 most recently updated repositories per account
  unless `limit` says otherwise (at most 1,000), and remembers what it found
  for ten minutes.
- It skips archived and empty repositories, and any whose clone address
  isn't on the platform itself.
- A repository listed under `sites` is never offered twice: the listed entry,
  with its preview settings, wins.
- A site in a subfolder of its repository isn't found; list it under `sites`
  with `subdir`.

## For people who'd rather type

- **Commands (Ctrl+K, ⌘K on a Mac):** every action by name, from anywhere:
  - open any page by typing part of its title;
  - switch drafts;
  - go to Files, Pictures or Look;
  - save, submit for review, publish;
  - change the theme, add an account.

  What's offered depends on where you are. Ctrl+S saves in the page editor,
  the Look panel and the Files view.
- **Files:** every file of the site, any text file edited as raw text.
  - Ctrl+S shows the diff first, and Ctrl+S again saves it, with a commit
    message you can change.
  - Files can be added and deleted. Each has its own history, and a draft can
    be compared with the published branch.
  - Like everything else, a save is a commit on a draft. `.git` and `public/`
    can't be written.
- **Console (Ctrl+`):** a command line that speaks HotDog CMS: `status`,
  `pages`, `edit site.yaml`, `cat`, `new articles "Title"`, `check`,
  `drafts`, `branch draft/about`, `diff`, `log`, `review "Title"`, `publish`,
  `as <login>`, `theme dark`. Type `help` for the rest.

  It runs nothing on the server: every command is a call the editor's buttons
  make too. The editor host is not a shell.

## Visual editing

The page body can be edited as Markdown or visually, with a switch above it
that's remembered per browser. The visual editor shows the page as it reads:
- headings, lists, quotes, code, links and pictures, with a toolbar;
- the usual shortcuts (Ctrl+B, Ctrl+I, Ctrl+Z);
- Markdown typing shortcuts (`## `, `- `, `1. `, `> `, three backticks).

It writes Markdown, never HTML, and only rewrites what you change:
- **Untouched paragraphs** are saved exactly as they were written, hand
  wrapping included, so a diff shows only your edit.
- **New and edited blocks** are written plainly: `-` bullets, `*` emphasis.
- **What it can't represent exactly** (tables, raw HTML, footnotes, task
  lists, `{#id}` attributes, strikethrough) is shown read-only, marked "Kept
  as written", and edited in the Markdown view.

Pictures dropped or pasted in still go through Insert picture, so they're
re-encoded and need alt text.

**Point and edit:** with Point & edit on in the preview bar, clicking text in
the preview puts the cursor on it, in either view. The editor says so when
the text comes from the front matter, a section or the template instead of
the body.

## Staying signed in

Some platforms' sign-ins expire: Gitea's and Forgejo's after an hour by
default, GitLab's after two. The editor renews them shortly before they
expire, using the refresh token the platform gave at sign-in (sealed in the
account's cookie like the rest). If the platform refuses, that account alone
is signed out.

## Search, social and the rest

- **Search and social,** on every page: the page as a search result and as a
  link preview, from what it actually renders, with what's missing or too
  long said plainly. Every check finding comes with how to fix it.
- **Move…:** renames a page's file and redirects its old address to the new
  one, in one commit. Redirects that pointed at the old address are updated,
  so there are no chains.
- **Redirects:** the site's redirects as a table.
- **Privacy & statistics:** the analytics wizard and who answers for
  visitors' data (see [privacy.md](privacy.md)).
- **Search engines:**
  - the verification codes for Google, Bing and Yandex;
  - an IndexNow key (see [publishing.md](publishing.md));
  - the sitemap address to give each console.

All of them save to `site.yaml` on a draft, changing only their own block.

## People and workflow

The platform is the one source of truth for who may do what. The editor
stores none of it.
- **Roles:** the site view says what you can do there (read, write drafts and
  open reviews, or maintain), from the platform's permissions. Who may
  publish (merge) is the platform's decision, branch protection included.
- **Asking for a review:** submitting a draft can ask people to review it,
  chosen from those the platform says can review. The platform notifies
  them, by its own notifications and email.
- **History:** every change that reached the published site, newest first,
  from git, so the record can't be edited by whoever it records. **Undo on a
  draft** makes a revert, which is reviewed and published like any change.
  If later changes overlap the one being undone, it says so instead of
  guessing.
- **Two-factor sign-in and passkeys** come with the platform: whatever it
  asks for at sign-in applies to the editor too.
