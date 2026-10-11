import { useState } from 'react'
import { BLUESKY_MAX, blueskyURL, graphemes, linkedinURL, mastodonHost, mastodonURL, shareText } from './share'

// Share a published page: the post text, editable, and buttons that open each
// network's share page with it filled in. Posting happens there, from the
// person's own account; the editor holds no tokens.

const KEY = 'hotdog-mastodon'

function storedHost(): string {
  try {
    return localStorage.getItem(KEY) || ''
  } catch {
    return ''
  }
}

export function SharePanel({ title, summary, url, onClose }: { title: string; summary: string; url: string; onClose: () => void }) {
  const [text, setText] = useState(() => shareText(title, summary, url))
  const [server, setServer] = useState(storedHost)
  const [copied, setCopied] = useState(false)
  const host = mastodonHost(server)
  const length = graphemes(text)

  const open = (href: string) => window.open(href, '_blank', 'noopener,noreferrer')
  const toMastodon = () => {
    try {
      localStorage.setItem(KEY, host)
    } catch {
      /* not remembered, still works */
    }
    open(mastodonURL(host, text))
  }
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      /* clipboard blocked: the text is there to select */
    }
  }

  return (
    <div className="share-panel form" role="group" aria-label="Share this page">
      <div className="library-head">
        <strong>Share</strong>
        <button type="button" className="ghost small" onClick={onClose}>Close</button>
      </div>
      <p className="muted small">Each button opens the network's own share page with this filled in; you post it from your account there. Share once the live site shows the change.</p>
      <label>
        Post
        <textarea rows={5} value={text} onChange={(e) => setText(e.target.value)} />
      </label>
      <p className={`small ${length > BLUESKY_MAX ? 'bad-text' : 'muted'}`}>{length} characters{length > BLUESKY_MAX ? `: Bluesky allows ${BLUESKY_MAX}` : ''}</p>
      <div className="share-row">
        <label className="share-server">
          Mastodon server
          <input value={server} placeholder="mastodon.social" onChange={(e) => setServer(e.target.value)} spellCheck={false} />
        </label>
        <button type="button" className="button" disabled={!host || !text.trim()} onClick={toMastodon}>Mastodon ↗</button>
      </div>
      <div className="actions">
        <button type="button" className="button" disabled={!text.trim() || length > BLUESKY_MAX} onClick={() => open(blueskyURL(text))}>Bluesky ↗</button>
        <button type="button" className="button" onClick={() => open(linkedinURL(url))} title="LinkedIn takes the link and reads the title and picture from the page; copy the text to add it">LinkedIn ↗</button>
        <button type="button" className="ghost" onClick={copy}>{copied ? 'Copied' : 'Copy text'}</button>
      </div>
    </div>
  )
}
