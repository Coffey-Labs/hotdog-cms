import { useEffect, useRef, useState } from 'react'
import { api, type FormDef, type FormField, type Submissions } from './api'
import { go } from './route'
import { Split } from './Split'
import { DevicePicker, PreviewFrame, useDevice } from './Viewport'

// The form builder: a site's forms (forms/*.yaml) as forms, previewed on the
// page that shows them, in the site's own styling, saved on a draft. And, when
// the editor is set up to read them, the submissions people sent, newest
// first, read from the endpoint's private viewer.

const TYPES = ['text', 'email', 'url', 'tel', 'textarea', 'select', 'checkbox']

function move<T>(list: T[], from: number, to: number): T[] {
  if (to < 0 || to >= list.length) return list
  const out = list.slice()
  const [x] = out.splice(from, 1)
  out.splice(to, 0, x)
  return out
}

export function FormsView({ id, branch, form }: { id: string; branch?: string; form?: string }) {
  const [list, setList] = useState<{ name: string; form: FormDef; usedBy: string[] | null }[]>()
  const [meta, setMeta] = useState<{ commit: string; branch: string; canWrite: boolean; submissions: boolean }>()
  const [def, setDef] = useState<FormDef>()
  const [orig, setOrig] = useState('')
  const [tab, setTab] = useState<'build' | 'submissions'>('build')
  const [status, setStatus] = useState<{ kind: 'info' | 'good' | 'bad'; text: string }>()
  const [previewURL, setPreviewURL] = useState<string>()
  const [previewNote, setPreviewNote] = useState<string>()
  const [reload, setReload] = useState(0)
  const [device, setDevice] = useDevice()
  const [busy, setBusy] = useState(false)
  const [creating, setCreating] = useState<string>()
  const [subs, setSubs] = useState<Submissions>()
  const [offset, setOffset] = useState(0)

  const load = () =>
    api.forms(id, branch).then(
      (r) => {
        setList(r.forms)
        setMeta({ commit: r.commit, branch: r.branch, canWrite: r.canWrite, submissions: r.submissions })
      },
      (e: Error) => setStatus({ kind: 'bad', text: e.message }),
    )
  useEffect(() => {
    load()
  }, [id, branch])
  useEffect(() => {
    const f = list?.find((x) => x.name === form)
    if (f) {
      setDef(structuredClone(f.form))
      setOrig(JSON.stringify(f.form))
    } else if (form && list) {
      const fresh: FormDef = { to: '', fields: [{ name: 'email', type: 'email', label: 'Email', required: true }] }
      setDef(fresh)
      setOrig('')
    } else setDef(undefined)
    setTab('build')
    setSubs(undefined)
    setOffset(0)
  }, [form, list])

  // The preview follows the form, a moment after the last change.
  const timer = useRef<number>(0)
  useEffect(() => {
    if (!def || !form || !meta) return
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      api.formLive(id, { branch: meta.branch, name: form, form: def }).then(
        (r) => {
          if (r.error) return setPreviewNote(r.error)
          setPreviewNote(undefined)
          if (r.url !== previewURL) setPreviewURL(r.url)
          else setReload((n) => n + 1)
        },
        (e: Error) => setPreviewNote(e.message),
      )
    }, 400)
    return () => window.clearTimeout(timer.current)
  }, [def, form, meta])

  useEffect(() => {
    if (tab === 'submissions' && form) api.submissions(id, form, offset).then(setSubs, (e: Error) => setStatus({ kind: 'bad', text: e.message }))
  }, [tab, form, offset])

  const dirty = !!def && JSON.stringify(def) !== orig
  const set = (patch: Partial<FormDef>) => setDef((d) => ({ ...d!, ...patch }))
  const setField = (i: number, patch: Partial<FormField>) => setDef((d) => ({ ...d!, fields: d!.fields.map((f, j) => (j === i ? { ...f, ...patch } : f)) }))
  const save = async (del = false) => {
    if (!meta || !form) return
    setBusy(true)
    setStatus({ kind: 'info', text: del ? 'Deleting…' : 'Saving…' })
    try {
      const r = await api.formSave(id, { branch: meta.branch, baseCommit: meta.commit, name: form, form: del ? undefined : def, delete: del })
      setStatus({ kind: 'good', text: `${del ? 'Deleted' : 'Saved'} on ${r.created ? `a new draft, ${r.branch}` : r.branch}.` })
      go({ view: 'forms', id, branch: r.branch, form: del ? undefined : form })
      if (!r.created && r.branch === branch) load()
    } catch (e) {
      setStatus({ kind: 'bad', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  const can = !!meta?.canWrite
  const used = list?.find((x) => x.name === form)?.usedBy || []
  const emailFields = (def?.fields || []).filter((f) => f.type === 'email')

  return (
    <Split
      left={
        <section className="editor-pane forms-view">
          <div className="pane-head">
            <a className="back" href={`/sites/${encodeURIComponent(id)}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id, branch }) }}>← Site</a>
          </div>
          <h1>Forms</h1>
          {status && <p className={`notice ${status.kind}`} role="status">{status.text}</p>}
          <div className="form-list">
            {list?.map((f) => (
              <button key={f.name} type="button" className={`side-item ${f.name === form ? 'on' : ''}`} onClick={() => go({ view: 'forms', id, branch, form: f.name })}>
                <span>{f.form.title || f.name}</span> <span className="muted small mono">{f.name}</span>
              </button>
            ))}
            {list && list.length === 0 && <p className="muted small">No forms yet.</p>}
            {can && creating === undefined && <button type="button" className="ghost small" onClick={() => setCreating('')}>New form</button>}
            {creating !== undefined && (
              <form className="file-new-form" onSubmit={(e) => { e.preventDefault(); const n = creating.trim(); if (/^[a-z][a-z0-9_]*$/.test(n)) { setCreating(undefined); go({ view: 'forms', id, branch, form: n }) } }}>
                <input autoFocus placeholder="newsletter" value={creating} onChange={(e) => setCreating(e.target.value.toLowerCase())} aria-label="New form's name" />
                <button type="submit" className="ghost small">Create</button>
                <button type="button" className="ghost small" onClick={() => setCreating(undefined)}>Cancel</button>
              </form>
            )}
          </div>
          {def && form && (
            <>
              <div className="toolbar">
                <h2 className="mono">{form}</h2>
                {meta?.submissions && (
                  <div className="seg small" role="tablist">
                    <button type="button" aria-pressed={tab === 'build'} onClick={() => setTab('build')}>Form</button>
                    <button type="button" aria-pressed={tab === 'submissions'} onClick={() => setTab('submissions')}>Submissions</button>
                  </div>
                )}
              </div>
              {tab === 'build' && (
                <div className="form builder">
                  {used.length === 0 && <p className="muted small">No page shows this form yet: add <span className="mono">form: {form}</span> to a page's front matter.</p>}
                  {used.length > 0 && <p className="muted small">Shown on {used.join(', ')}.</p>}
                  <div className="row2">
                    <label>Title<input value={def.title || ''} disabled={!can} onChange={(e) => set({ title: e.target.value })} /></label>
                    <label>Send submissions to<input value={def.to || ''} disabled={!can} placeholder="hello@example.org" onChange={(e) => set({ to: e.target.value })} /></label>
                  </div>
                  <div className="row2">
                    <label>Email subject<input value={def.subject || ''} disabled={!can} placeholder={'Contact from {{ .name }}'} onChange={(e) => set({ subject: e.target.value })} /></label>
                    <label>Reply to
                      <select value={def.reply_to || ''} disabled={!can} onChange={(e) => set({ reply_to: e.target.value || undefined })}>
                        <option value="">—</option>
                        {emailFields.map((f) => <option key={f.name} value={f.name}>{f.label || f.name}</option>)}
                      </select>
                    </label>
                  </div>
                  <div className="row2">
                    <label>Button<input value={def.submit || ''} disabled={!can} placeholder="Send" onChange={(e) => set({ submit: e.target.value })} /></label>
                    <label>After sending, go to<input value={def.success || ''} disabled={!can} placeholder="/thanks/" onChange={(e) => set({ success: e.target.value })} /></label>
                  </div>
                  <label>Address shown without JavaScript<input value={def.mailto || ''} disabled={!can} placeholder="hello@example.org" onChange={(e) => set({ mailto: e.target.value })} /></label>
                  <label>Cloudflare Turnstile site key<input value={def.turnstile_sitekey || ''} disabled={!can} placeholder="Optional" onChange={(e) => set({ turnstile_sitekey: e.target.value })} /></label>
                  <label className="check"><input type="checkbox" checked={!!def.store} disabled={!can} onChange={(e) => set({ store: e.target.checked || undefined })} /> Also keep submissions on the server (readable here, if the editor is set up for it)</label>
                  <h3 className="side-label">Fields</h3>
                  <ol className="list-items">
                    {def.fields.map((f, i) => (
                      <li key={i} className="list-item field-card">
                        <div className="row2">
                          <label>Label<input value={f.label || ''} disabled={!can} onChange={(e) => setField(i, { label: e.target.value })} /></label>
                          <label>Name<input value={f.name} disabled={!can} className="mono" onChange={(e) => setField(i, { name: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '_') })} /></label>
                        </div>
                        <div className="row2">
                          <label>Kind
                            <select value={f.type || 'text'} disabled={!can} onChange={(e) => setField(i, { type: e.target.value === 'text' ? undefined : e.target.value })}>
                              {TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
                            </select>
                          </label>
                          <label>Placeholder<input value={f.placeholder || ''} disabled={!can} onChange={(e) => setField(i, { placeholder: e.target.value })} /></label>
                        </div>
                        {f.type === 'select' && <label>Choices, one per line<textarea rows={3} value={(f.options || []).join('\n')} disabled={!can} onChange={(e) => setField(i, { options: e.target.value.split('\n').map((x) => x.trim()).filter(Boolean) })} /></label>}
                        <label>Help<input value={f.help || ''} disabled={!can} onChange={(e) => setField(i, { help: e.target.value })} /></label>
                        <div className="actions-row">
                          <label className="check"><input type="checkbox" checked={!!f.required} disabled={!can} onChange={(e) => setField(i, { required: e.target.checked || undefined })} /> Required</label>
                          <label className="inline">Most characters<input type="number" min={1} value={f.max || ''} disabled={!can} onChange={(e) => setField(i, { max: e.target.value ? Number(e.target.value) : undefined })} /></label>
                          {can && <>
                            <button type="button" className="ghost small" disabled={i === 0} onClick={() => set({ fields: move(def.fields, i, i - 1) })} aria-label="Move up">↑</button>
                            <button type="button" className="ghost small" disabled={i === def.fields.length - 1} onClick={() => set({ fields: move(def.fields, i, i + 1) })} aria-label="Move down">↓</button>
                            <button type="button" className="ghost small" onClick={() => set({ fields: def.fields.filter((_, j) => j !== i) })}>Remove</button>
                          </>}
                        </div>
                      </li>
                    ))}
                  </ol>
                  {can && (
                    <div className="actions-row">
                      <button type="button" className="ghost small" onClick={() => set({ fields: [...def.fields, { name: `field_${def.fields.length + 1}`, label: 'New field' }] })}>Add a field</button>
                      <button type="button" className="button primary" disabled={!dirty || busy} onClick={() => save()}>{meta?.branch === branch ? 'Save' : 'Save as draft'}</button>
                      {orig && <button type="button" className="ghost small" disabled={busy} onClick={() => save(true)}>Delete form</button>}
                    </div>
                  )}
                </div>
              )}
              {tab === 'submissions' && (
                <div className="subs">
                  {!subs && <p className="muted">Reading submissions…</p>}
                  {subs && subs.total === 0 && <p className="muted">No submissions kept for this form.</p>}
                  {subs && subs.submissions.length > 0 && (
                    <>
                      <p className="muted small">{subs.total} kept, newest first. Read from the endpoint's private viewer; the public site can't show them.</p>
                      <div className="table-wrap">
                        <table className="pages">
                          <thead><tr><th>When</th>{(subs.fields || []).map((f) => <th key={f.name}>{f.label || f.name}</th>)}</tr></thead>
                          <tbody>
                            {subs.submissions.map((s, i) => (
                              <tr key={i}><td className="small">{new Date(s.time).toLocaleString()}</td>{(subs.fields || []).map((f) => <td key={f.name} className="small sub-cell">{s.values[f.name]}</td>)}</tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                      <div className="actions-row">
                        <button type="button" className="ghost small" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}>Newer</button>
                        <button type="button" className="ghost small" disabled={offset + 50 >= subs.total} onClick={() => setOffset(offset + 50)}>Older</button>
                      </div>
                    </>
                  )}
                </div>
              )}
            </>
          )}
          {!form && list && list.length > 0 && <p className="muted">Choose a form to change it.</p>}
        </section>
      }
      right={
        <section className="preview-pane" aria-label="Preview">
          <div className="preview-bar"><DevicePicker device={device} onChange={setDevice} /></div>
          {previewNote && <p className="notice warn small preview-note">{previewNote}</p>}
          <PreviewFrame src={previewURL} device={device} title="Preview of the form" reloadKey={reload} />
        </section>
      }
    />
  )
}
