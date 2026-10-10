import { useEffect, useRef, useState } from 'react'
import { api, type MediaItem, type Uploaded } from './api'
import { go } from './route'

// Pictures: the library of what's in the site's media/ folder, and the
// dialog that puts one into a page. Every picture needs alt text, or an
// explicit "decorative" choice; there is no way to insert one without
// deciding.

export const ACCEPT = 'image/jpeg,image/png,image/gif,image/webp'

function kb(n: number) {
  return n > 1 << 20 ? `${(n / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`
}

export function MediaLibrary({ id, branch }: { id: string; branch?: string }) {
  const [items, setItems] = useState<MediaItem[]>()
  const [error, setError] = useState<string>()
  useEffect(() => {
    api.media(id, branch).then(setItems, (e: Error) => setError(e.message))
  }, [id, branch])
  return (
    <section className="pad">
      <a className="back" href={`/sites/${encodeURIComponent(id)}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id, branch }) }}>← Site</a>
      <h1>Pictures</h1>
      <p className="muted">
        Everything in <span className="mono">static/media/</span>{branch ? <> on <span className="mono">{branch}</span></> : null}. Pictures are added from a
        page, with Insert picture; each is re-encoded on the way in, with its location and camera details removed.
      </p>
      {error && <p className="notice bad" role="alert">{error}</p>}
      {!items && !error && <p className="muted">Looking through the site…</p>}
      {items && items.length === 0 && <p className="muted">No pictures yet.</p>}
      <ul className="media-grid">
        {items?.map((m) => (
          <li key={m.url} className="media-card">
            <img src={api.mediaFile(id, m.thumb, branch)} alt={m.alt} width={m.width} height={m.height} loading="lazy" />
            <div className="media-meta">
              <span className="mono small">{m.url}</span>
              <span className="muted small">{m.width}×{m.height} · {m.files} files · {kb(m.bytes)}</span>
              {m.usedBy.length > 0 ? (
                <span className="small">Used in {m.usedBy.map((u) => <span key={u} className="mono"> {u}</span>)}</span>
              ) : (
                <span className="pill warn">Not used</span>
              )}
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}

type Props = {
  id: string
  branch: string
  baseCommit: string
  source: string
  onDraft: boolean
  file?: File
  onUploaded: (r: Uploaded) => void // the commit landed, before inserting
  onInsert: (markdown: string) => void
  onClose: () => void
}

export function InsertPicture({ id, branch, baseCommit, source, onDraft, file: initial, onUploaded, onInsert, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [tab, setTab] = useState<'upload' | 'library'>('upload')
  const [file, setFile] = useState<File | undefined>(initial)
  const [local, setLocal] = useState<string>()
  const [picked, setPicked] = useState<MediaItem>()
  const [items, setItems] = useState<MediaItem[]>()
  const [alt, setAlt] = useState('')
  const [decorative, setDecorative] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  useEffect(() => {
    dialog.current?.showModal()
  }, [])
  useEffect(() => {
    if (!file) return setLocal(undefined)
    const u = URL.createObjectURL(file)
    setLocal(u)
    return () => URL.revokeObjectURL(u)
  }, [file])
  useEffect(() => {
    if (tab === 'library' && !items) api.media(id, branch).then(setItems, (e: Error) => setError(e.message))
  }, [tab])

  const ready = (decorative || alt.trim() !== '') && (tab === 'upload' ? !!file : !!picked) && !busy
  const markdown = (url: string) => `![${decorative ? '' : alt.trim().replace(/[[\]\n]/g, ' ')}](${url})`

  const insert = async () => {
    setError(undefined)
    if (tab === 'library' && picked) {
      onInsert(markdown(picked.url))
      return
    }
    if (!file) return
    setBusy(true)
    try {
      const r = await api.upload(id, file, { branch, baseCommit, source })
      onUploaded(r)
      onInsert(markdown(r.url))
    } catch (e) {
      setError((e as Error).message)
      setBusy(false)
    }
  }

  return (
    <dialog ref={dialog} className="picture-dialog form" aria-labelledby="pic-title" onClose={onClose} onCancel={(e) => busy && e.preventDefault()}>
      <h2 id="pic-title">Insert picture</h2>
      <div className="seg" role="tablist" aria-label="Where the picture comes from">
        <button type="button" role="tab" aria-selected={tab === 'upload'} aria-pressed={tab === 'upload'} onClick={() => setTab('upload')}>Upload</button>
        <button type="button" role="tab" aria-selected={tab === 'library'} aria-pressed={tab === 'library'} onClick={() => setTab('library')}>From the library</button>
      </div>

      {tab === 'upload' && (
        <div
          className={`drop ${file ? 'has' : ''}`}
          onDragOver={(e) => e.preventDefault()}
          onDrop={(e) => {
            e.preventDefault()
            const f = e.dataTransfer.files[0]
            if (f) setFile(f)
          }}
        >
          {local ? <img src={local} alt="" /> : <p className="muted">Drop a picture here, or choose one.</p>}
          <label className="button small">
            {file ? 'Choose another' : 'Choose a picture'}
            <input type="file" accept={ACCEPT} className="visually-hidden" onChange={(e) => e.target.files?.[0] && setFile(e.target.files[0])} />
          </label>
          {file && <span className="muted small">{file.name} · {kb(file.size)}</span>}
        </div>
      )}

      {tab === 'library' && (
        <div className="library-pick">
          {!items && !error && <p className="muted">Loading the library…</p>}
          {items && items.length === 0 && <p className="muted">No pictures in this site yet.</p>}
          <ul className="media-grid small">
            {items?.map((m) => (
              <li key={m.url}>
                <button
                  type="button"
                  className={`media-pick ${picked?.url === m.url ? 'on' : ''}`}
                  aria-pressed={picked?.url === m.url}
                  onClick={() => {
                    setPicked(m)
                    if (!alt && m.alt) setAlt(m.alt)
                  }}
                >
                  <img src={api.mediaFile(id, m.thumb, branch)} alt={m.alt || m.url} loading="lazy" />
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}

      <label className="alt-field">
        Alt text <span className="muted small">what the picture shows, for people who can't see it</span>
        <input value={alt} disabled={decorative} onChange={(e) => setAlt(e.target.value)} placeholder="e.g. The harbor at dawn, boats tied up along the dock" />
      </label>
      <label className="check">
        <input type="checkbox" checked={decorative} onChange={(e) => setDecorative(e.target.checked)} /> Decorative: it adds nothing the text doesn't say
      </label>
      {!decorative && !alt.trim() && <p className="muted small">A picture needs alt text, or to be marked decorative.</p>}
      {tab === 'upload' && <p className="muted small">It's re-encoded on the way in: its location and camera details are removed, and it's resized for the web. It's committed to {onDraft ? 'this draft' : 'a new draft; the published site is unchanged'}.</p>}
      {error && <p className="notice bad" role="alert">{error}</p>}
      <div className="actions">
        <button type="button" className="button primary" disabled={!ready} onClick={insert}>{busy ? 'Uploading…' : 'Insert'}</button>
        <button type="button" className="ghost" disabled={busy} onClick={() => dialog.current?.close()}>Cancel</button>
      </div>
    </dialog>
  )
}
