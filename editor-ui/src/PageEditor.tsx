import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, CheckFailed, Conflict, type Edit, type PageDetail, type Problem, type Seo, type SiteDetail, type Uploaded } from './api'
import { go, href } from './route'
import { useCommands, useShortcut } from './Commands'
import { ACCEPT, InsertPicture } from './Media'
import { liveChannel } from './Present'
import { SeoPanel } from './Seo'
import { SharePanel } from './Share'
import { fromFrontMatter, SectionsEditor, toFrontMatter, type Sec } from './Sections'
import type { VisualHandle } from './VisualEditor'

// The visual editor (ProseMirror and a Markdown parser) loads only when
// someone switches to it.
const VisualEditor = lazy(() => import('./VisualEditor').then((m) => ({ default: m.VisualEditor })))
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
  const [sharing, setSharing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [previewURL, setPreviewURL] = useState<string>()
  const [previewNote, setPreviewNote] = useState<string>()
  const [reload, setReload] = useState(0)
  const [device, setDevice] = useDevice()
  const [reviewOpen, setReviewOpen] = useState(false)
  const [problems, setProblems] = useState<Problem[]>()
  const [picture, setPicture] = useState<{ file?: File }>()
  const [secs, setSecs] = useState<Sec[]>()
  // The body as Markdown or as it reads (visual), remembered per browser.
  const [bodyMode, setBodyModeState] = useState<'markdown' | 'visual'>(() => {
    try {
      return localStorage.getItem('hotdog-body-mode') === 'visual' ? 'visual' : 'markdown'
    } catch {
      return 'markdown'
    }
  })
  const setBodyMode = (m: 'markdown' | 'visual') => {
    setBodyModeState(m)
    try {
      localStorage.setItem('hotdog-body-mode', m)
    } catch {
      /* not remembered */
    }
  }
  const visualRef = useRef<VisualHandle>(null)
  const [pointing, setPointing] = useState(false)
  const [moving, setMoving] = useState<string>()
  const [seo, setSeo] = useState<{ seo: Seo; siteURL: string; path: string }>()
  const bodyRef = useRef<HTMLTextAreaElement>(null)
  // Edits carried across a branch change that the editor made itself (an
  // upload that started a draft), so nothing typed is lost.
  const carry = useRef<{ fields: Fields; body: string; yaml: string; yamlMode: boolean; secs?: Sec[] }>(undefined)
  const uploaded = useRef<Uploaded>(undefined)
  const cur = branch || site?.publishBranch || ''

  useEffect(() => {
    setPage(undefined)
    api.page(id, source, branch).then(
      (p) => {
        setPage(p)
        const c = carry.current
        carry.current = undefined
        setFields(c ? c.fields : fieldsFrom(p.frontMatter))
        setBody(c ? c.body : p.body)
        setYaml(c ? c.yaml : p.frontMatterRaw)
        setYamlMode(c ? c.yamlMode : false)
        setSecs(c ? c.secs : fromFrontMatter(p.frontMatter.sections))
      },
      (e: Error) => setLoadError(e.message),
    )
  }, [id, source, branch])

  const orig = useMemo(() => (page ? fieldsFrom(page.frontMatter) : undefined), [page])
  const edit: Edit | null = useMemo(() => {
    if (!page || !fields || !orig) return null
    if (yamlMode) return yaml === page.frontMatterRaw && body === page.body ? null : { frontMatter: yaml, body }
    const set = changes(orig, fields, page.frontMatter)
    if (secs) {
      const next = toFrontMatter(secs)
      if (JSON.stringify(next) !== JSON.stringify(page.frontMatter.sections ?? null)) set.sections = next.length ? next : null
    }
    return Object.keys(set).length === 0 && body === page.body ? null : { set, body }
  }, [page, fields, orig, body, yaml, yamlMode, secs])
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
            if (r.seo && r.siteURL && r.path) setSeo({ seo: r.seo, siteURL: r.siteURL, path: r.path })
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
  const liveURL = site?.siteURL && pagePath ? site.siteURL.replace(/\/+$/, '') + pagePath : ''
  const canShare = !onDraft && !!fields && !fields.draft && !!liveURL

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
        setSecs(fromFrontMatter(p.frontMatter.sections))
      }
    } catch (e) {
      setStatus({ kind: 'bad', text: e instanceof Conflict ? e.message : `Not saved: ${(e as Error).message}` })
    } finally {
      setBusy(false)
    }
  }

  // A picture goes in where the cursor was, on a line of its own.
  const insertPicture = (md: string) => {
    let next: string
    const vis = bodyMode === 'visual' && visualRef.current
    const img = /^!\[([^\]]*)\]\(([^)]+)\)$/.exec(md)
    if (vis && img) {
      next = visualRef.current!.insertImage(img[2], img[1])
    } else {
      const el = bodyRef.current
      const at = el ? el.selectionStart : body.length
      const before = body.slice(0, at)
      const after = body.slice(at)
      const lead = before === '' || before.endsWith('\n\n') ? '' : before.endsWith('\n') ? '\n' : '\n\n'
      const trail = after.startsWith('\n\n') ? '' : after.startsWith('\n') ? '\n' : after === '' ? '\n' : '\n\n'
      next = before + lead + md + trail + after
    }
    setBody(next)
    setPicture(undefined)
    const r = uploaded.current
    uploaded.current = undefined
    if (!r) return
    const notes = (r.notes || []).join(' ')
    setStatus({ kind: 'good', text: `Added ${r.url}${r.created ? ` on a new draft, ${r.branch}` : ''}. ${notes} Save to keep it in the page.`.trim() })
    onSiteChanged()
    if (r.created || r.branch !== cur) {
      if (fields) carry.current = { fields, body: next, yaml, yamlMode, secs }
      go({ view: 'page', id, source, branch: r.branch })
    } else if (page) {
      setPage({ ...page, commit: r.commit })
    }
  }

  // A picture uploaded into a section's field: like Insert picture, it may
  // start a draft, and the editor moves there with the unsaved edits.
  const sectionUpload = useRef<Uploaded>(undefined)
  const uploadForSection = async (file: File) => {
    const r = await api.upload(id, file, { branch: cur, baseCommit: page?.commit || '', source })
    sectionUpload.current = r
    return r.url
  }
  useEffect(() => {
    const r = sectionUpload.current
    if (!r) return
    sectionUpload.current = undefined
    const notes = (r.notes || []).join(' ')
    setStatus({ kind: 'good', text: `Added ${r.url}${r.created ? ` on a new draft, ${r.branch}` : ''}. ${notes} Save to keep it in the page.`.trim() })
    onSiteChanged()
    if (r.created || r.branch !== cur) {
      if (fields) carry.current = { fields, body, yaml, yamlMode, secs }
      go({ view: 'page', id, source, branch: r.branch })
    } else if (page) {
      setPage({ ...page, commit: r.commit })
    }
  }, [secs])

  const publish = async () => {
    setBusy(true)
    setStatus({ kind: 'info', text: 'Checking the draft and publishing…' })
    setProblems(undefined)
    try {
      await api.publish(id, { branch: cur })
      setStatus({ kind: 'good', text: 'Published: the change is merged. The live site updates when its publishing pipeline runs.' })
      setSharing(true)
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

  // Point and edit: the preview tells us what text was clicked; the cursor
  // goes there, in whichever view the body is in.
  const previewOrigin = previewURL ? new URL(previewURL).origin : ''
  const tellPreview = (on: boolean) => {
    const frame = document.querySelector<HTMLIFrameElement>('.preview-pane iframe')
    if (frame?.contentWindow && previewOrigin) frame.contentWindow.postMessage({ type: 'hotdog:point', on }, previewOrigin)
  }
  useEffect(() => {
    const on = (e: MessageEvent) => {
      if (!previewOrigin || e.origin !== previewOrigin || !e.data || typeof e.data !== 'object') return
      if (e.data.type === 'hotdog:ready') tellPreview(pointing)
      if (e.data.type !== 'hotdog:jump' || typeof e.data.text !== 'string') return
      const text = e.data.text.slice(0, 200)
      if (bodyMode === 'visual' && visualRef.current) {
        if (!visualRef.current.jumpTo(text)) setStatus({ kind: 'info', text: `"${text.slice(0, 60)}" isn't in the body; it comes from the front matter, a section or the template.` })
        return
      }
      const el = bodyRef.current
      if (!el) return
      const hay = body.toLowerCase()
      const words = text.toLowerCase().split(' ')
      let at = -1
      for (const probe of [text.toLowerCase().slice(0, 40), words.slice(0, 4).join(' '), words.slice(0, 2).join(' ')]) {
        if (probe.length >= 3 && (at = hay.indexOf(probe)) >= 0) break
      }
      if (at < 0) {
        setStatus({ kind: 'info', text: `"${text.slice(0, 60)}" isn't in the body; it comes from the front matter, a section or the template.` })
        return
      }
      el.focus()
      el.setSelectionRange(at, at + Math.min(text.length, body.length - at))
      const line = body.slice(0, at).split('\n').length
      el.scrollTop = Math.max(0, (line - 3) * parseFloat(getComputedStyle(el).lineHeight || '20'))
    }
    window.addEventListener('message', on)
    return () => window.removeEventListener('message', on)
  }, [previewOrigin, pointing, bodyMode, body])
  useEffect(() => tellPreview(pointing), [pointing, previewURL])

  // Keyboard first: Ctrl+S saves, and the palette has the rest.
  useShortcut('mod+s', () => dirty && !busy && canWrite && save(), !!page)
  useCommands(() =>
    !page
      ? []
      : [
          ...(canWrite && dirty ? [{ id: 'page-save', title: onDraft ? 'Save' : 'Save as draft', group: 'Page', keys: 'Ctrl+S', run: save }] : []),
          ...(canWrite ? [{ id: 'page-picture', title: 'Insert picture', group: 'Page', run: () => setPicture({}) }] : []),
          { id: 'page-mode', title: bodyMode === 'visual' ? 'Edit the body as Markdown' : 'Edit the body visually', group: 'Page', run: () => setBodyMode(bodyMode === 'visual' ? 'markdown' : 'visual') },
          ...(previewURL ? [{ id: 'page-point', title: pointing ? 'Stop pointing in the preview' : 'Point and edit: click text in the preview', group: 'Page', run: () => setPointing(!pointing) }] : []),
          { id: 'page-yaml', title: yamlMode ? 'Edit front matter as fields' : 'Edit front matter as YAML', group: 'Page', run: () => setYamlMode(!yamlMode) },
          ...(onDraft && !review && canWrite && !dirty ? [{ id: 'page-review', title: 'Submit for review', group: 'Page', run: () => setReviewOpen(true) }] : []),
          ...(onDraft && review && canWrite && !dirty ? [{ id: 'page-publish', title: 'Publish', group: 'Page', run: publish }] : []),
          ...(canShare && !dirty ? [{ id: 'page-share', title: 'Share this page', group: 'Page', run: () => setSharing(true) }] : []),
          ...(previewURL ? [{ id: 'page-present', title: 'Present in a new tab', group: 'Page', run: () => window.open(href({ view: 'present', id, url: previewURL, branch }), '_blank', 'noopener') }] : []),
          { id: 'page-file', title: `Edit ${source} as a raw file`, group: 'Page', run: () => go({ view: 'files', id, branch, path: source }) },
        ],
  )

  if (loadError) return <div className="pad notice bad" role="alert">{loadError}</div>
  if (!page || !fields) return <p className="pad muted">Loading the page…</p>
  // Front matter the form doesn't show; sections have their own cards.
  const otherKeys = Object.keys(page.frontMatter).filter((k) => !KNOWN.includes(k as keyof Fields) && !(k === 'sections' && secs))
  const set = (k: keyof Fields) => (e: { target: { value: string; checked?: boolean; type?: string } }) =>
    setFields({ ...fields, [k]: e.target.type === 'checkbox' ? !!e.target.checked : e.target.value })

  return (
    <Split
      left={
      <section className="editor-pane">
        <div className="pane-head">
          <a className="back" href={`/sites/${encodeURIComponent(id)}${branch ? `?branch=${encodeURIComponent(branch)}` : ''}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id, branch }) }}>← {site?.name || 'Site'}</a>
          <span className="pane-pills">
            {site && site.accounts.length > 1 && <span className="pill" title="Change it in the site view">as @{site.as.split('/').pop()}</span>}
            <span className={`pill ${onDraft ? 'warn' : ''}`}>{onDraft ? `draft: ${cur}` : `${cur} · published`}</span>
          </span>
        </div>
        <h1>{fields.title || source}</h1>
        <p className="muted mono small">
          {source}{pagePath ? ` → ${pagePath}` : ''}
          {canWrite && moving === undefined && source !== 'content/index.md' && <button type="button" className="ghost small move-btn" onClick={() => setMoving(pagePath || '')}>Move…</button>}
        </p>
        {moving !== undefined && <MoveForm id={id} source={source} branch={cur} baseCommit={page.commit} current={pagePath || ""} value={moving} dirty={dirty} onChange={setMoving} onDone={(r) => { setMoving(undefined); setStatus({ kind: 'good', text: `Moved to ${r.to}${r.redirected ? `; ${r.from} now redirects there` : ''}.${r.created ? ` On a new draft, ${r.branch}.` : ''}` }); onSiteChanged(); go({ view: 'page', id, source: r.source, branch: r.branch }) }} />}

        <div className="actions">
          <button type="button" className="button primary" disabled={!dirty || busy || !canWrite} onClick={save}>{onDraft ? 'Save' : 'Save as draft'}</button>
          {onDraft && !review && <button type="button" className="button" disabled={busy || dirty || !canWrite} onClick={() => setReviewOpen(!reviewOpen)}>Submit for review</button>}
          {onDraft && review && <a className="button" href={review.url} target="_blank" rel="noopener noreferrer">Review #{review.number} ↗</a>}
          {onDraft && review && <button type="button" className="button" disabled={busy || dirty || !canWrite} onClick={publish} title="Merges the review, if the draft passes the checks">Publish</button>}
          {canShare && !sharing && <button type="button" className="ghost" disabled={dirty} onClick={() => setSharing(true)} title="Post a link to this page on Mastodon, Bluesky or LinkedIn">Share…</button>}
          {!canWrite && <span className="muted small">Read only: the platform doesn't let you change this site.</span>}
          {dirty && <span className="muted small">Unsaved changes</span>}
        </div>
        {status && <p className={`notice ${status.kind}`} role="status">{status.text}</p>}
        {problems && problems.length > 0 && (
          <ul className="problems">{problems.filter((p) => p.Level === 'error').map((p, i) => <li key={i} className="error"><span className="pill">{p.Rule}</span> <span className="mono small">{p.File}</span> {p.What}</li>)}</ul>
        )}
        {sharing && canShare && <SharePanel title={fields.title} summary={fields.summary} url={liveURL} onClose={() => setSharing(false)} />}
        {reviewOpen && <ReviewForm id={id} branch={cur} title={fields.title} onDone={(msg) => { setReviewOpen(false); setStatus({ kind: 'good', text: msg }); onSiteChanged() }} onError={(m) => setStatus({ kind: 'bad', text: m })} />}

        {pageProblems.length > 0 && (
          <div className="notice warn">
            <strong>Checks on this page</strong>
            <ul className="problems">{pageProblems.map((p, i) => <li key={i} className={p.Level}><span className="pill">{p.Rule}</span> {p.What}{p.Fix && <span className="fix">{p.Fix}</span>}</li>)}</ul>
          </div>
        )}
        {seo && <SeoPanel id={id} branch={cur} seo={seo.seo} siteURL={seo.siteURL} path={seo.path} />}

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
            {otherKeys.length > 0 && <p className="muted small">Other fields ({otherKeys.join(', ')}) are kept as they are; edit them as YAML.</p>}
          </div>
        )}
        {secs && !yamlMode && (
          <>
            <h2 className="side-label">Sections <span className="muted small">the page, top to bottom</span></h2>
            <SectionsEditor id={id} branch={cur} value={secs} onChange={setSecs} disabled={!canWrite} upload={uploadForSection} />
          </>
        )}
        <div className="body-head">
          <h2 className="side-label">Body</h2>
          <div className="seg small" role="group" aria-label="Edit the body as">
            <button type="button" aria-pressed={bodyMode === 'markdown'} onClick={() => setBodyMode('markdown')}>Markdown</button>
            <button type="button" aria-pressed={bodyMode === 'visual'} onClick={() => setBodyMode('visual')}>Visual</button>
          </div>
          {canWrite && bodyMode === 'markdown' && <button type="button" className="ghost small" onClick={() => setPicture({})}>Insert picture</button>}
        </div>
        {bodyMode === 'visual' && (
          <Suspense fallback={<p className="muted small">Loading the visual editor…</p>}>
            <VisualEditor ref={visualRef} value={body} onChange={setBody} onPicture={(f) => setPicture({ file: f })} disabled={!canWrite} />
          </Suspense>
        )}
        {bodyMode === 'markdown' && <textarea
          ref={bodyRef}
          className="code body-edit"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          onDragOver={(e) => canWrite && e.dataTransfer.types.includes('Files') && e.preventDefault()}
          onDrop={(e) => {
            const f = Array.from(e.dataTransfer.files).find((x) => ACCEPT.split(',').includes(x.type))
            if (f && canWrite) {
              e.preventDefault()
              setPicture({ file: f })
            }
          }}
          onPaste={(e) => {
            const f = Array.from(e.clipboardData.files).find((x) => ACCEPT.split(',').includes(x.type))
            if (f && canWrite) {
              e.preventDefault()
              setPicture({ file: f })
            }
          }}
          spellCheck
          aria-label="Page body, Markdown"
        />}
        {picture && (
          <InsertPicture
            id={id}
            branch={cur}
            baseCommit={page.commit}
            source={source}
            onDraft={onDraft}
            file={picture.file}
            onUploaded={(r) => (uploaded.current = r)}
            onInsert={insertPicture}
            onClose={() => setPicture(undefined)}
          />
        )}
      </section>
      }
      right={
      <section className="preview-pane" aria-label="Preview">
        <div className="preview-bar">
          <DevicePicker device={device} onChange={setDevice} />
          <span className="spacer" />
          {/* A new tab, so the edit in progress stays open here; the preview
              of the unsaved text lives on the server, so it shows there too. */}
          {previewURL && <button type="button" className="ghost small" aria-pressed={pointing} onClick={() => setPointing(!pointing)} title="Click text in the preview to put the cursor there">Point &amp; edit</button>}
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

export function ReviewForm({ id, branch, title, onDone, onError }: { id: string; branch: string; title: string; onDone: (msg: string) => void; onError: (m: string) => void }) {
  const [t, setT] = useState(`Update ${title}`)
  const [d, setD] = useState('')
  const [busy, setBusy] = useState(false)
  const [people, setPeople] = useState<{ login: string; name?: string }[]>()
  const [ask, setAsk] = useState<string[]>([])
  useEffect(() => {
    api.people(id).then((r) => setPeople(r.people), () => setPeople([]))
  }, [id])
  const submit = async () => {
    setBusy(true)
    try {
      const r = await api.review(id, { branch, title: t, description: d, reviewers: ask })
      const who = r.reviewers?.length ? ` ${r.reviewers.map((x) => '@' + x).join(', ')} ${r.reviewers.length === 1 ? 'was' : 'were'} asked to review; the platform lets them know.` : ''
      onDone(`Submitted for review as #${r.number}. The review links to this draft's preview.${who}${r.reviewersError ? ' ' + r.reviewersError : ''}`)
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
      {people && people.length > 0 && (
        <fieldset className="field group">
          <legend>Ask for a review from</legend>
          <div className="reviewer-pick">
            {people.map((p) => (
              <label key={p.login} className="check">
                <input type="checkbox" checked={ask.includes(p.login)} onChange={(e) => setAsk(e.target.checked ? [...ask, p.login] : ask.filter((x) => x !== p.login))} />
                {p.name || p.login} <span className="muted small">@{p.login}</span>
              </label>
            ))}
          </div>
        </fieldset>
      )}
      <div className="actions"><button type="button" className="button primary" disabled={busy || !t.trim()} onClick={submit}>Open the review</button></div>
    </div>
  )
}

// MoveForm moves a page to a new address: its file is renamed, and the old
// address redirects to the new one, in one commit.
function MoveForm({ id, source, branch, baseCommit, current, value, dirty, onChange, onDone }: {
  id: string; source: string; branch: string; baseCommit: string; current: string; value: string; dirty: boolean
  onChange: (v: string | undefined) => void
  onDone: (r: { branch: string; created: boolean; source: string; from: string; to: string; redirected: boolean }) => void
}) {
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string>()
  const clean = '/' + value.trim().replace(/^\/+|\/+$/g, '') + '/'
  const to = clean === '//' ? '' : `content${clean.slice(0, -1)}.md`
  const go2 = async () => {
    setBusy(true)
    setErr(undefined)
    try {
      onDone(await api.move(id, { branch, baseCommit, source, to }))
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <form className="form move-form" onSubmit={(e) => { e.preventDefault(); if (to && !dirty) go2() }}>
      <label>New address<input autoFocus value={value} onChange={(e) => onChange(e.target.value)} placeholder="/about-us/" /></label>
      {to && <p className="muted small">Renames <span className="mono">{source}</span> to <span className="mono">{to}</span>{current && clean !== current ? <>, and <span className="mono">{current}</span> will redirect to <span className="mono">{clean}</span></> : ''}.</p>}
      {dirty && <p className="notice warn small">Save your changes first; the move is its own commit.</p>}
      {err && <p className="notice bad small">{err}</p>}
      <div className="actions-row">
        <button type="submit" className="button" disabled={busy || !to || dirty || clean === current}>Move</button>
        <button type="button" className="ghost" onClick={() => onChange(undefined)}>Cancel</button>
      </div>
    </form>
  )
}
