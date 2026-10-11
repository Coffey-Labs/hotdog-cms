import { api, type Seo } from './api'

// What a page looks like where people find it: a search result and a link
// preview, from the page as it renders, with what's missing or too long said
// plainly.

const TITLE_MAX = 60
const DESC_MAX = 160

export function SeoPanel({ id, branch, seo, siteURL, path }: { id: string; branch: string; seo: Seo; siteURL: string; path: string }) {
  const url = siteURL.replace(/\/$/, '') + path
  const img = seo.image
  const local = img && img.startsWith('/media/')
  const notes: string[] = []
  if (!seo.title) notes.push('No title.')
  else if (seo.title.length > TITLE_MAX) notes.push(`The title is ${seo.title.length} characters; search results show about ${TITLE_MAX}, so the end may be cut.`)
  if (!seo.description) notes.push('No description: search engines will pick some text from the page. Add a summary.')
  else if (seo.description.length > DESC_MAX) notes.push(`The description is ${seo.description.length} characters; results show about ${DESC_MAX}.`)
  if (!img) notes.push('No picture for link previews: set image in the front matter.')
  else if (!seo.imageAlt) notes.push('The preview picture has no description (image_alt).')
  return (
    <details className="seo">
      <summary>
        <strong>Search and social</strong>
        <span className="muted small"> {seo.noindex ? 'hidden from search engines' : notes.length ? `${notes.length} thing${notes.length === 1 ? '' : 's'} to look at` : 'looks good'}</span>
      </summary>
      {seo.noindex && <p className="notice warn small">This page asks search engines not to list it (noindex).</p>}
      <div className="serp" aria-label="As a search result">
        <span className="serp-url">{url}</span>
        <span className="serp-title">{seo.title.length > TITLE_MAX ? seo.title.slice(0, TITLE_MAX - 1) + '…' : seo.title || '(no title)'}</span>
        <span className="serp-desc">{seo.description ? (seo.description.length > DESC_MAX ? seo.description.slice(0, DESC_MAX - 1) + '…' : seo.description) : <em className="muted">No description</em>}</span>
      </div>
      <div className="card-preview" aria-label="As a link preview">
        <div className="card-img">
          {local ? <img src={api.mediaFile(id, img, branch)} alt="" /> : img ? <span className="muted small mono">{img}</span> : <span className="muted small">No picture</span>}
        </div>
        <div className="card-text">
          <span className="muted small">{new URL(url).host}</span>
          <strong>{seo.title || '(no title)'}</strong>
          {seo.description && <span className="small">{seo.description}</span>}
        </div>
      </div>
      <p className="muted small">Mastodon, LinkedIn and Bluesky show the picture above the title like this; a picture 1200×630 suits them all.</p>
      {notes.length > 0 && <ul className="seo-notes">{notes.map((n) => <li key={n}>{n}</li>)}</ul>}
    </details>
  )
}
