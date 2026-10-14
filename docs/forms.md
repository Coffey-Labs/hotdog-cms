# Forms and search: the endpoint

A HotDog CMS site is static files. Two things need a server to answer them: form
submissions and, if you want it, search. Both go to `hotdog-cms endpoint`, a
small server you run beside your web server, which passes `/_hotdog/` to it. It
holds no content, edits nothing, and needs no database.

## A form

`forms/contact.yaml`:

```yaml
title: Contact
to: hello@example.org
subject: "Website message from {{ .name }}"
reply_to: email
fields:
  - { name: name, label: Your name, required: true, max: 120 }
  - { name: email, type: email, label: Your email, required: true }
  - { name: topic, type: select, label: About, options: [A question, Feedback] }
  - { name: message, type: textarea, label: Message, required: true }
submit: Send message
success: /contact/thanks/   # optional; otherwise a thank-you appears in place
mailto: hello@example.org   # shown to visitors without JavaScript
store: true                 # optional; also append to a file (see below)
```

Field types are `text`, `email`, `url`, `tel`, `textarea`, `select` and
`checkbox`. Put a form in a page with `form: contact` in its front matter (the
starter's page layout renders it), or `{{ form "contact" }}` in any template.
The build fails on a form that doesn't make sense: no recipient, a select with
no options, an unknown key.

The contact-forms pack (`hotdog-cms pack add contact-forms`) brings three
more, a quote request, a booking request and a mailing list sign-up, and a
section that shows any form beside your contact details.

## What the endpoint checks

In this order, and a submission that fails any of them is not sent:

1. **It came from the site's own page.** The browser's `Origin` must be one
   of the site's hosts.
2. **Rate limit,** per visitor address: 5 in 10 minutes by default.
3. **Text fields only.** No file uploads, and at most 64 KB.
4. **The trap field is empty.** It's hidden from people; bots fill it in.
   They're told the message was sent.
5. **A signed token, 3 seconds to 24 hours old.** The page fetches it when it
   loads. That rejects instant, scripted posts and replays, and it needs no
   storage. Every endpoint for a site shares `HOTDOG_FORM_SECRET`.
6. **Turnstile,** if the form sets `turnstile_sitekey`.
7. **The fields themselves:**
   - only the declared ones are accepted;
   - required fields must be filled;
   - each field has a length limit;
   - email addresses must parse, and select values must be among the options;
   - single-line fields may not contain line breaks, so nothing typed can
     become an email header.

Then it emails the submission (Reply-To is the sender's checked address) and,
for forms with `store: true`, appends it to `<store>/<form>.jsonl` (mode
0600). If the email can't go out, a stored submission is kept and the visitor
is told it worked; otherwise they're told to try again, or to email the
`mailto` address.

With JavaScript, problems appear next to the fields they're about. Without
it, the visitor gets a plain page saying what went wrong.

## Search, two ways

Collections with `search: true` get `/<collection>/search.json`. A site can
use it either way, or both:

- **In the browser:** the page downloads the index when someone searches and
  scores it there. Works on any host, including a plain CDN; costs the visitor
  the index (about 1 MB, 330 KB gzipped, for 135 long articles).
- **From the endpoint:** `GET /_hotdog/search/<collection>?q=...&page=N` returns
  one page of results as JSON, scored the same way. The visitor downloads
  only the results.

The jcoffey.dev prototype picks with `params.search: endpoint` in `site.yaml`.

## Running it

`endpoint.yaml`:

```yaml
listen: 127.0.0.1:8180
trusted_proxies: [127.0.0.1/32]      # whose X-Forwarded-For to believe
smtp:
  host: smtp.example.org
  port: 587
  username: forms@example.org
  from: "Example <forms@example.org>"
  tls: starttls                      # or tls (465); none only to localhost
rate_limit: { count: 5, window: 10m }
# source: https://git.example.org/me/hotdog-cms  # only if you've changed the code
sites:
  - dir: /srv/sites/example.org      # the site's source: site.yaml, forms/
    public: /var/www/example.org     # the built site, for search
    store: /var/lib/hotdog-cms/forms/example.org
    recipients: [hello@example.org, "@team.example.org"]   # the only addresses its forms may email
```

```sh
HOTDOG_FORM_SECRET=$(openssl rand -hex 32) HOTDOG_SMTP_PASSWORD=... hotdog-cms endpoint -config endpoint.yaml
```

In nginx, pass `/_hotdog/` to it:

```nginx
location /_hotdog/ {
    proxy_pass http://127.0.0.1:8180;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

For local work, `hotdog-cms serve -endpoint http://127.0.0.1:8180` does the same.

`/_hotdog/source` sends people to the endpoint's source code, as the AGPL
asks of software people use over a network. Leave `source` out when running
HotDog CMS as released; if you've changed the code, publish your changes and
set `source` to where they are.

## Where a data store would come in

Nothing above needs one. If any of these is ever wanted, it's the point to
decide on one:

- **A rate limit shared across endpoints.** Today each endpoint counts on its
  own, so three behind a load balancer allow three times the limit.
- **Single-use form tokens.** Today a token can be reused within its 24
  hours, from the same page.
- **Searching or filtering stored submissions,** beyond reading the JSONL
  file.

## Reading stored submissions in the editor

Forms with `store: true` keep each submission in a JSON-lines file on the
endpoint's server. The editor can show them, without the public endpoint
ever serving them. The endpoint opens a second listener for that, the
**submissions viewer**, only when asked:

```yaml
# endpoint.yaml
viewer:
  listen: 10.0.0.5:8182            # a loopback or private address only
  token_env: HOTDOG_VIEWER_TOKEN   # at least 32 characters
```

- **Private addresses only:** the endpoint refuses to start the viewer on a
  public address, since submissions are people's personal data. Put it on
  the private network or VPN the editor reaches.
- **Token:** it answers only with the token, compared in constant time.
- **Read-only:** it only reads.

Then, in the editor's configuration:

```yaml
# editor.yaml
sites:
  - name: Example
    repo: https://git.example.org/me/example.git
    submissions:
      url: http://10.0.0.5:8182
      token_env: HOTDOG_VIEWER_TOKEN   # the same token, in the editor's environment
```

The editor's **Forms** page shows the submissions, newest first, only to
people the platform lets edit the site. Each read is logged.

## Building forms in the editor

The **Forms** page edits `forms/*.yaml` as forms:
- the recipient, subject, reply-to field, button text, success page, mailto
  fallback, Turnstile and storage;
- the fields, with labels, kinds, choices, help, required and length.

A preview shows the form on the page that uses it, in the site's own
styling. Saving goes to a draft. The file's opening comments are kept, and
only the settings that are set are written.
