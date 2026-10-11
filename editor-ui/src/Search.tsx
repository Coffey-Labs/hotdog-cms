import { useEffect, useState } from 'react'
import { api } from './api'
import { go } from './route'

// Search engines: proving to each console that the site is yours, and
// IndexNow, which tells search engines what changed every time the site is
// published. Codes are written into site.yaml (verify:, indexnow:) on a
// draft; the build turns them into meta tags and the key file.

const CONSOLES = [
  { k: 'google', name: 'Google Search Console', where: 'Add a property for the site, choose "HTML tag", and copy the content="…" value (or the whole tag; the code is taken out of it).' },
  { k: 'bing', name: 'Bing Webmaster Tools', where: 'Add the site, choose "HTML Meta Tag", and copy the content value of msvalidate.01.' },
  { k: 'yandex', name: 'Yandex Webmaster', where: 'Add the site, choose "Meta tag", and copy the content value.' },
] as const

function newKey() {
  const b = new Uint8Array(16)
  crypto.getRandomValues(b)
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
}

export function SearchView({ id, branch }: { id: string; branch?: string }) {
  const [info, setInfo] = useState<{ commit: string; branch: string; canWrite: boolean; sitemap: string; siteURL: string; start: string }>()
  const [verify, setVerify] = useState<Record<string, string>>({})
  const [key, setKey] = useState('')
  const [status, setStatus] = useState<{ kind: 'info' | 'good' | 'bad'; text: string }>()
  const [busy, setBusy] = useState(false)
  const load = () =>
    api.search(id, branch).then(
      (r) => {
        const v = { google: r.verify.google || '', bing: r.verify.bing || '', yandex: r.verify.yandex || '' }
        setVerify(v)
        setKey(r.indexnow || '')
        setInfo({ commit: r.commit, branch: r.branch, canWrite: r.canWrite, sitemap: r.sitemap, siteURL: r.siteURL, start: JSON.stringify([v, r.indexnow || '']) })
      },
      (e: Error) => setStatus({ kind: 'bad', text: e.message }),
    )
  useEffect(() => {
    load()
  }, [id, branch])
  const dirty = !!info && JSON.stringify([verify, key]) !== info.start
  const save = async () => {
    if (!info) return
    setBusy(true)
    setStatus({ kind: 'info', text: 'Saving…' })
    try {
      const r = await api.searchSave(id, { branch: info.branch, baseCommit: info.commit, verify, indexnow: key })
      setStatus({ kind: 'good', text: `Saved on ${r.created ? `a new draft, ${r.branch}` : r.branch}. Once it's published, press Verify in each console.` })
      if (r.created || r.branch !== branch) go({ view: 'search', id, branch: r.branch })
      else load()
    } catch (e) {
      setStatus({ kind: 'bad', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className="pad search-view">
      <a className="back" href={`/sites/${encodeURIComponent(id)}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id, branch }) }}>← Site</a>
      <h1>Search engines</h1>
      <p className="muted">Show each search console the site is yours, and tell search engines what changed whenever it's published.</p>
      {status && <p className={`notice ${status.kind}`} role="status">{status.text}</p>}
      {info && (
        <div className="form wizard">
          <fieldset className="field group">
            <legend>Verification</legend>
            {CONSOLES.map((c) => (
              <label key={c.k} className="field">
                {c.name}
                <input value={verify[c.k] || ''} disabled={!info.canWrite} placeholder="The code from the meta tag" onChange={(e) => setVerify({ ...verify, [c.k]: e.target.value })} />
                <span className="muted small">{c.where}</span>
              </label>
            ))}
            <p className="muted small">The codes become meta tags on the home page. The site's sitemap, to give each console: <span className="mono">{info.sitemap}</span></p>
          </fieldset>
          <fieldset className="field group">
            <legend>IndexNow</legend>
            <p className="muted small">IndexNow tells Bing, Yandex and the other search engines that share it which pages changed, each time the site is published. The key proves the message is the site's own; the build publishes it as <span className="mono">/{key || 'key'}.txt</span>.</p>
            <div className="image-row">
              <input value={key} disabled={!info.canWrite} placeholder="No key: IndexNow is off" onChange={(e) => setKey(e.target.value)} aria-label="IndexNow key" />
              {info.canWrite && <button type="button" className="ghost small" onClick={() => setKey(newKey())}>{key ? 'New key' : 'Make a key'}</button>}
            </div>
            <p className="muted small">Then, on the live site's publish target, set <span className="mono">indexnow: true</span> in publish.yaml (or run the pull agent with <span className="mono">-indexnow</span>). Only that target sends; staging never does.</p>
          </fieldset>
          {info.canWrite && <div className="actions-row"><button type="button" className="button primary" disabled={!dirty || busy} onClick={save}>{info.branch === branch ? 'Save' : 'Save as draft'}</button></div>}
        </div>
      )}
    </section>
  )
}
