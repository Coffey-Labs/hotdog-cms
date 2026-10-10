import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, CheckFailed, Conflict, type Edit, type PageDetail, type Problem, type SiteDetail } from './api'
import { go, href } from './route'
import { liveChannel } from './Present'
import { Split } from './Split'
import { DevicePicker, PreviewFrame, useDevice } from './Viewport'

// Editing one page: the common front matter as a form (or all of it as
// YAML), the body as Markdown, and the page rendered live beside it as you
// type. Saving commits as you; the first save of a change starts a draft.

type Fields = {
  title: string
  summary: string
  date: string
  tags: string
  image: string
  image_alt: string
  draft: boolean
  publish_at: string
}

const KNOWN: (keyof Fields)[] = ['title', 'summary', 'date', 'tags', 'image', 'image_alt', 'draft', 'publish_at']

function str(v: unknown): string {
  if (v === undefined || v === null) return ''
  return typeof v === 'string' ? v : String(v)
}

function dateOnly(v: unknown): string {
  const s = str(v)
  return /^\d{4}-\d{2}-\d{2}/.test(s) ? s.slice(0, 10) : s
}

function toLocalInput(v: unknown): string {
  const s = str(v)
  if (!s) return ''
  const d = new Date(s)
  if (isNaN(d.getTime())) return ''
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}

function fieldsFrom(fm: Record<string, unknown>): Fields {
  const tags = Array.isArray(fm.tags) ? fm.tags.map(str).join(', ') : str(fm.tags)
  return {
    title: str(fm.title), summary: str(fm.summary), date: dateOnly(fm.date), tags,
    image: str(fm.image), image_alt: str(fm.image_alt), draft: fm.draft === true, publish_at: toLocalInput(fm.publish_at),
  }
}

// The changes to send: only fields that differ from the file, null to remove.
function changes(orig: Fields, now: Fields, fm: Record<string, unknown>): Record<string, unknown> {
  const set: Record<string, unknown> = {}
  for (const k of KNOWN) {
    if (orig[k] === now[k]) continue
    const v = now[k]
    if (k === 'draft') set.draft = v ? true : null
    else if (k === 'tags') set.tags = (v as string).trim() ? (v as string).split(',').map((t) => t.trim()).filter(Boolean) : null
    else if (k === 'publish_at') set.publish_at = v ? new Date(v as string).toISOString().replace('.000Z', 'Z') : null
    else set[k] = (v as string).trim() ? v : k in fm ? null : undefined
  }
  for (const k of Object.keys(set)) if (set[k] === undefined) delete set[k]
  return set
}

export function PageEditor({ id, source, branch, site, onSiteChanged }: { id: string; source: string; branch?: string; site?: SiteDetail; onSiteChanged: () => void }) {
  const [page, setPage] = useState<PageDetail>()
  const [loadError, setLoadError] = useState<string>()
  const [fields, setFields] = useState<Fields>()
  const [body, setBody] = useState('')
  const [yamlMode, setYamlMode] = useState(false)
  const [yaml, setYaml] = useState('')
  const [status, setStatus] = useState<{ kind: 'info' | 'good' | 'bad'; text: string }>()
  const [busy, setBusy] = useState(false)
  const [previewURL, setPreviewURL] = useState<string>()
  const [previewNote, setPreviewNote] = useState<string>()
  const [reload, setReload] = useState(0)
  const [device, setDevice] = useDevice()
  const [reviewOpen, setReviewOpen] = useState(false)
  const [problems, setProblems] = useState<Problem[]>()
  const cur = branch || site?.publishBranch || ''

  useEffect(() => {
    setPage(undefined)
    api.page(id, source, branch).then(
      (p) => {
        setPage(p)
        setFields(fieldsFrom(p.frontMatter))
        setBody(p.body)
        setYaml(p.frontMatterRaw)
        setYamlMode(false)
      },
      (e: Error) => setLoadError(e.message),
    )
  }, [id, source, branch])

  const orig = useMemo(() => (page ? fieldsFrom(page.frontMatter) : undefined), [page])
  const edit: Edit | null = useMemo(() => {
    if (!page || !fields || !orig) return null
    if (yamlMode) return yaml === page.frontMatterRaw && body === page.body ? null : { frontMatter: yaml, body }
    const set = changes(orig, fields, page.frontMatter)
    return Object.keys(set).length === 0 && body === page.body ? null : { set, body }
  }, [page, fields, orig, body, yaml, yamlMode])
  const dirty = edit !== null

  // The live preview follows the edit, a moment after typing stops, here and
  // in any Present tab showing it.
  const timer = useRef<number>(0)
  const channel = useRef<BroadcastChannel>(undefined)
  useEffect(() => {
    channel.current = liveChannel()
    return () => channel.current?.close()
  }, [])
  const renderLive = useCallback(
    (e: Edit | null) => {
      if (!page) return
      api.live(id, { source, branch: cur, edit: e }).then(
        (r) => {
          if (r.error) setPreviewNote(`The preview shows the last version that rendered. ${r.error}`)
          else {
            setPreviewNote(undefined)
            if (r.url !== previewURL) setPreviewURL(r.url)
            else setReload((n) => n + 1)
            channel.current?.postMessage({ url: r.url })
          }
        },
        (err: Error) => setPreviewNote(err.message),
      )
    },
    [id, source, cur, page, previewURL],
  )
  useEffect(() => {
    if (!page) return
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => renderLive(edit), edit ? 600 : 0)
    return () => window.clearTimeout(timer.current)
  }, [edit, page])

  // Warn before leaving with unsaved work.
  useEffect(() => {
    const on = (e: BeforeUnloadEvent) => {
      if (dirty) e.preventDefault()
    }
    addEventListener('beforeunload', on)
    return () => removeEventListener('beforeunload', on)
  }, [dirty])

  const pagePath = site?.pages?.find((p) => p.source === source)?.path
  const output = pagePath ? (pagePath.endsWith('/') ? `public${pagePath}index.html` : `public${pagePath}`) : ''
  const pageProblems = (site?.build.problems || []).filter((p) => p.File === output || p.File.startsWith(source))
  const review = site?.reviews?.find((r) => r.branch === cur)
  const onDraft = site ? cur !== site.publishBranch : false
  const canWrite = site?.canWrite

  const save = async () => {
    if (!page || !edit) return
    setBusy(true)
    setStatus({ kind: 'info', text: 'Saving…' })
    try {
      const r = await api.save(id, { source, branch: cur, baseCommit: page.commit, edit })
      setStatus({ kind: 'good', text: r.created ? `Saved. Your change is on a new draft, ${r.branch}; the published site is unchanged.` : `Saved to ${r.branch}.` })
      onSiteChanged()
      if (r.created || r.branch !== branch) go({ view: 'page', id, source, branch: r.branch })
      else {
        const p = await api.page(id, source, r.branch)
        setPage(p)
        setFields(fieldsFrom(p.frontMatter))
        setBody(p.body)
        setYaml(p.frontMatterRaw)
      }
    } catch (e) {
      setStatus({ kind: 'bad', text: e instanceof Conflict ? e.message : `Not saved: ${(e as Error).message}` })
    } finally {
      setBusy(false)
    }
  }

  const publish = async () => {
    setBusy(true)
    setStatus({ kind: 'info', text: 'Checking the draft and publishing…' })
    setProblems(undefined)
    try {
      await api.publish(id, { branch: cur })
      setStatus({ kind: 'good', text: 'Published: the change is merged. The live site updates when its publishing pipeline runs.' })
      // The merge may have removed the draft branch; carry on from the
      // published one.
      if (site) go({ view: 'page', id, source, branch: site.publishBranch })
      onSiteChanged()
    } catch (e) {
      if (e instanceof CheckFailed) setProblems(e.problems)
      setStatus({ kind: 'bad', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  if (loadError) return <div className="pad notice bad" role="alert">{loadError}</div>
  if (!page || !fields) return <p className="pad muted">Loading the page…</p>
  const set = (k: keyof Fields) => (e: { target: { value: string; checked?: boolean; type?: string } }) =>
    setFields({ ...fields, [k]: e.target.type === 'checkbox' ? !!e.target.checked : e.target.value })

  return (
    <Split
      left={
      <section className="editor-pane">
        <div className="pane-head">
          <a className="back" href={`/sites/${encodeURIComponent(id)}${branch ? `?branch=${encodeURIComponent(branch)}` : ''}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id, branch }) }}>← {site?.name || 'Site'}</a>
          <span className={`pill ${onDraft ? 'warn' : ''}`}>{onDraft ? `draft: ${cur}` : `${cur} · published`}</span>
        </div>
        <h1>{fields.title || source}</h1>
        <p className="muted mono small">{source}{pagePath ? ` → ${pagePath}` : ''}</p>

        <div className="actions">
          <button type="button" className="button primary" disabled={!dirty || busy || !canWrite} onClick={save}>{onDraft ? 'Save' : 'Save as draft'}</button>
          {onDraft && !review && <button type="button" className="button" disabled={busy || dirty || !canWrite} onClick={() => setReviewOpen(!reviewOpen)}>Submit for review</button>}
          {onDraft && review && <a className="button" href={review.url} target="_blank" rel="noopener noreferrer">Review #{review.number} ↗</a>}
          {onDraft && review && <button type="button" className="button" disabled={busy || dirty || !canWrite} onClick={publish} title="Merges the review, if the draft passes the checks">Publish</button>}
          {!canWrite && <span className="muted small">Read only: the platform doesn't let you change this site.</span>}
          {dirty && <span className="muted small">Unsaved changes</span>}
        </div>
        {status && <p className={`notice ${status.kind}`} role="status">{status.text}</p>}
        {problems && problems.length > 0 && (
          <ul className="problems">{problems.filter((p) => p.Level === 'error').map((p, i) => <li key={i} className="error"><span className="pill">{p.Rule}</span> <span className="mono small">{p.File}</span> {p.What}</li>)}</ul>
        )}
        {reviewOpen && <ReviewForm id={id} branch={cur} title={fields.title} onDone={(msg) => { setReviewOpen(false); setStatus({ kind: 'good', text: msg }); onSiteChanged() }} onError={(m) => setStatus({ kind: 'bad', text: m })} />}

        {pageProblems.length > 0 && (
          <div className="notice warn">
            <strong>Checks on this page</strong>
            <ul className="problems">{pageProblems.map((p, i) => <li key={i} className={p.Level}><span className="pill">{p.Rule}</span> {p.What}</li>)}</ul>
          </div>
        )}

        <div className="fm-head">
          <h2 className="side-label">Front matter</h2>
          <button type="button" className="ghost small" onClick={() => setYamlMode(!yamlMode)}>{yamlMode ? 'Edit as fields' : 'Edit as YAML'}</button>
        </div>
        {yamlMode ? (
          <textarea className="code" rows={10} value={yaml} onChange={(e) => setYaml(e.target.value)} spellCheck={false} aria-label="Front matter as YAML" />
        ) : (
          <div className="form">
            <label>Title<input value={fields.title} onChange={set('title')} /></label>
            <label>Summary<textarea rows={2} value={fields.summary} onChange={set('summary')} /></label>
            <div className="row2">
              <label>Date<input type="date" value={fields.date} onChange={set('date')} /></label>
              <label>Publish at<input type="datetime-local" value={fields.publish_at} onChange={set('publish_at')} /></label>
            </div>
            <label>Tags <span className="muted small">comma-separated</span><input value={fields.tags} onChange={set('tags')} /></label>
            <div className="row2">
              <label>Image<input value={fields.image} onChange={set('image')} placeholder="/img/…" /></label>
              <label>Image alt text<input value={fields.image_alt} onChange={set('image_alt')} /></label>
            </div>
            <label className="check"><input type="checkbox" checked={fields.draft} onChange={set('draft')} /> Keep out of the published site (draft)</label>
            {Object.keys(page.frontMatter).some((k) => !KNOWN.includes(k as keyof Fields)) && (
              <p className="muted small">Other fields ({Object.keys(page.frontMatter).filter((k) => !KNOWN.includes(k as keyof Fields)).join(', ')}) are kept as they are; edit them as YAML.</p>
            )}
          </div>
        )}
        <h2 className="side-label">Body <span className="muted small">Markdown</span></h2>
        <textarea className="code body-edit" value={body} onChange={(e) => setBody(e.target.value)} spellCheck aria-label="Page body, Markdown" />
      </section>
      }
      right={
      <section className="preview-pane" aria-label="Preview">
        <div className="preview-bar">
          <DevicePicker device={device} onChange={setDevice} />
          <span className="spacer" />
          {/* A new tab, so the edit in progress stays open here; the preview
              of the unsaved text lives on the server, so it shows there too. */}
          {previewURL && <a className="ghost small linkbtn" href={href({ view: 'present', id, url: previewURL, branch })} target="_blank" rel="noopener">Present ↗</a>}
          {previewURL && <a className="ghost small linkbtn" href={previewURL} target="_blank" rel="noopener noreferrer">Open ↗</a>}
        </div>
        {previewNote && <p className="notice warn small preview-note">{previewNote}</p>}
        <PreviewFrame src={previewURL} device={device} title={`Preview of ${fields.title}`} reloadKey={reload} />
      </section>
      }
    />
  )
}

function ReviewForm({ id, branch, title, onDone, onError }: { id: string; branch: string; title: string; onDone: (msg: string) => void; onError: (m: string) => void }) {
  const [t, setT] = useState(`Update ${title}`)
  const [d, setD] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async () => {
    setBusy(true)
    try {
      const r = await api.review(id, { branch, title: t, description: d })
      onDone(`Submitted for review as #${r.number}. The review links to this draft's preview.`)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="form review-form">
      <label>Review title<input value={t} onChange={(e) => setT(e.target.value)} /></label>
      <label>What changed, for the reviewer<textarea rows={3} value={d} onChange={(e) => setD(e.target.value)} /></label>
      <div className="actions"><button type="button" className="button primary" disabled={busy || !t.trim()} onClick={submit}>Open the review</button></div>
    </div>
  )
}
