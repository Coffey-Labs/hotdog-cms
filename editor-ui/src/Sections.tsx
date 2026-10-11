import { useEffect, useRef, useState } from 'react'
import { ACCEPT } from './Media'
import { api, type Field, type MediaItem, type SectionInfo, type SectionLib } from './api'

// A page made of sections, as cards: add one from the site's library, drag or
// move it, duplicate or remove it, and edit its fields as a form. The form
// comes from the section's own template (its declared fields, or the ones it
// uses), so a developer adds a section type by adding a template.

export type Data = Record<string, unknown>
export type Sec = { key: string; type: string; data: Data; raw?: unknown }

let nextKey = 0
const newKey = () => `s${nextKey++}`

// fromFrontMatter reads `sections:` (a list of one-key maps) into cards.
export function fromFrontMatter(v: unknown): Sec[] | undefined {
  if (!Array.isArray(v)) return undefined
  return v.map((item) => {
    if (item && typeof item === 'object' && !Array.isArray(item) && Object.keys(item).length === 1) {
      const [type, data] = Object.entries(item as Data)[0]
      return { key: newKey(), type, data: data && typeof data === 'object' && !Array.isArray(data) ? (data as Data) : {} }
    }
    return { key: newKey(), type: '', data: {}, raw: item }
  })
}

export function toFrontMatter(secs: Sec[]): unknown[] {
  return secs.map((s) => (s.raw !== undefined ? s.raw : { [s.type]: s.data }))
}

function defaults(fields: Field[]): Data {
  const d: Data = {}
  for (const f of fields) if (f.default !== undefined && f.default !== null && f.default !== '') d[f.name] = f.type === 'number' ? Number(f.default) : f.default
  return d
}

// with sets or removes a key, keeping the others where they were.
function withKey(obj: Data, key: string, val: unknown): Data {
  const empty = val === undefined || val === '' || val === false || (Array.isArray(val) && val.length === 0) || (val && typeof val === 'object' && !Array.isArray(val) && Object.keys(val as Data).length === 0)
  if (empty) {
    const { [key]: _, ...rest } = obj
    return rest
  }
  return key in obj ? Object.fromEntries(Object.entries(obj).map(([k, v]) => [k, k === key ? val : v])) : { ...obj, [key]: val }
}

function move<T>(list: T[], from: number, to: number): T[] {
  if (to < 0 || to >= list.length || from === to) return list
  const out = list.slice()
  const [x] = out.splice(from, 1)
  out.splice(to, 0, x)
  return out
}

function summary(d: Data): string {
  for (const k of ['heading', 'title', 'eyebrow', 'text', 'body']) if (typeof d[k] === 'string' && d[k]) return String(d[k]).split('\n')[0].slice(0, 80)
  return ''
}

// upload, when the panel offers it, saves a picture to the site and returns
// its address; the panel takes care of the draft it lands on.
export type Ctx = { id: string; branch: string; lib: SectionLib; upload?: (file: File) => Promise<string> }

export function SectionsEditor({ id, branch, value, onChange, disabled, upload }: { id: string; branch: string; value: Sec[]; onChange: (v: Sec[]) => void; disabled?: boolean; upload?: (file: File) => Promise<string> }) {
  const [lib, setLib] = useState<SectionLib>()
  const [error, setError] = useState<string>()
  const [open, setOpen] = useState<string>()
  const [adding, setAdding] = useState(false)
  const [drag, setDrag] = useState<number>()
  useEffect(() => {
    api.sections(id, branch).then(setLib, (e: Error) => setError(e.message))
  }, [id, branch])
  const info = (type: string) => lib?.sections.find((s) => s.name === type)
  const set = (i: number, s: Sec) => onChange(value.map((x, j) => (j === i ? s : x)))

  return (
    <div className="sections">
      {error && <p className="notice bad small">{error}</p>}
      <ol className="section-list">
        {value.map((s, i) => {
          const si = info(s.type)
          const isOpen = open === s.key
          const extra = si ? Object.keys(s.data).filter((k) => !si.fields.some((f) => f.name === k)) : []
          return (
            <li
              key={s.key}
              className={`section-card ${drag === i ? 'dragging' : ''}`}
              onDragOver={(e) => drag !== undefined && e.preventDefault()}
              onDrop={(e) => {
                e.preventDefault()
                if (drag !== undefined) onChange(move(value, drag, i))
                setDrag(undefined)
              }}
            >
              <div className="section-head" draggable={!disabled} onDragStart={() => setDrag(i)} onDragEnd={() => setDrag(undefined)}>
                <span className="grip" aria-hidden="true" title="Drag to move">⋮⋮</span>
                <button type="button" className="section-title" aria-expanded={isOpen} onClick={() => setOpen(isOpen ? undefined : s.key)}>
                  <strong>{s.raw !== undefined ? 'Not a section' : si?.label || s.type}</strong>
                  <span className="muted small"> {summary(s.data)}</span>
                </button>
                <span className="section-tools">
                  <button type="button" className="ghost small" disabled={disabled || i === 0} onClick={() => onChange(move(value, i, i - 1))} aria-label={`Move ${si?.label || s.type} up`}>↑</button>
                  <button type="button" className="ghost small" disabled={disabled || i === value.length - 1} onClick={() => onChange(move(value, i, i + 1))} aria-label={`Move ${si?.label || s.type} down`}>↓</button>
                  <button type="button" className="ghost small" disabled={disabled} onClick={() => onChange([...value.slice(0, i + 1), { ...s, key: newKey(), data: structuredClone(s.data) }, ...value.slice(i + 1)])} aria-label={`Duplicate ${si?.label || s.type}`}>Duplicate</button>
                  <button type="button" className="ghost small" disabled={disabled} onClick={() => onChange(value.filter((_, j) => j !== i))} aria-label={`Remove ${si?.label || s.type}`}>Remove</button>
                </span>
              </div>
              {isOpen && (
                <div className="section-body">
                  {s.raw !== undefined && <p className="muted small">This entry isn't a section the form can edit; change it in the YAML view.</p>}
                  {s.raw === undefined && lib && !si && <p className="notice bad small">This site has no <span className="mono">sections/{s.type}.html</span>, so the page won't build until it's removed or added.</p>}
                  {si?.problem && <p className="notice warn small">The section's field list couldn't be read ({si.problem}); its fields were worked out from the template instead.</p>}
                  {si && lib && <Fields fields={si.fields} value={s.data} onChange={(d) => set(i, { ...s, data: d })} ctx={{ id, branch, lib, upload }} disabled={disabled} />}
                  {extra.length > 0 && <p className="muted small">Also has {extra.join(', ')}; edit {extra.length === 1 ? 'it' : 'them'} in the YAML view.</p>}
                </div>
              )}
            </li>
          )
        })}
      </ol>
      {!disabled && !adding && <button type="button" className="ghost small" onClick={() => setAdding(true)}>Add a section</button>}
      {adding && lib && (
        <div className="section-library" role="group" aria-label="Section library">
          <div className="library-head">
            <strong>Add a section</strong>
            <button type="button" className="ghost small" onClick={() => setAdding(false)}>Cancel</button>
          </div>
          <ul>
            {lib.sections.map((si: SectionInfo) => (
              <li key={si.name}>
                <button
                  type="button"
                  className="library-card"
                  onClick={() => {
                    const s = { key: newKey(), type: si.name, data: defaults(si.fields) }
                    onChange([...value, s])
                    setOpen(s.key)
                    setAdding(false)
                  }}
                >
                  <strong>{si.label}</strong>
                  {si.about && <span className="muted small">{si.about}</span>}
                  {si.pack && <span className="muted small">From the {si.pack} pack</span>}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

function Fields({ fields, value, onChange, ctx, disabled }: { fields: Field[]; value: Data; onChange: (d: Data) => void; ctx: Ctx; disabled?: boolean }) {
  return (
    <div className="fields">
      {fields.map((f) => (
        <FieldInput key={f.name} field={f} value={value[f.name]} onChange={(v) => onChange(withKey(value, f.name, v))} ctx={ctx} disabled={disabled} />
      ))}
    </div>
  )
}

function FieldInput({ field: f, value, onChange, ctx, disabled }: { field: Field; value: unknown; onChange: (v: unknown) => void; ctx: Ctx; disabled?: boolean }) {
  const label = (
    <span className="field-label">
      {f.label || f.name}
      {f.required && <span className="req" aria-hidden="true"> *</span>}
    </span>
  )
  const help = f.help && <span className="muted small">{f.help}</span>
  const missing = f.required && (value === undefined || value === '') && <span className="field-warn small">Needed</span>
  const str = typeof value === 'string' || typeof value === 'number' ? String(value) : ''
  switch (f.type) {
    case 'bool':
      return (
        <label className="check field">
          <input type="checkbox" checked={value === true} disabled={disabled} onChange={(e) => onChange(e.target.checked)} /> {f.label || f.name}
          {help}
        </label>
      )
    case 'textarea':
    case 'markdown':
      return (
        <label className="field">
          {label}
          <textarea className={f.type === 'markdown' ? 'code' : ''} rows={f.type === 'markdown' ? 4 : 3} value={str} disabled={disabled} required={f.required} onChange={(e) => onChange(e.target.value)} />
          {help}
          {missing}
        </label>
      )
    case 'number':
      return (
        <label className="field">
          {label}
          <input type="number" value={str} disabled={disabled} onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))} />
          {help}
        </label>
      )
    case 'select':
    case 'data':
    case 'collection': {
      const opts = f.type === 'select' ? f.options || [] : f.type === 'data' ? ctx.lib.data : ctx.lib.collections
      return (
        <label className="field">
          {label}
          <select value={str} disabled={disabled} onChange={(e) => onChange(e.target.value || undefined)}>
            <option value="">{f.default !== undefined ? `Default (${String(f.default)})` : '—'}</option>
            {opts.map((o) => <option key={o} value={o}>{o}</option>)}
            {str && !opts.includes(str) && <option value={str}>{str} (not found)</option>}
          </select>
          {help}
          {missing}
        </label>
      )
    }
    case 'image':
      return <ImageField f={f} label={label} help={help} value={str} onChange={onChange} ctx={ctx} disabled={disabled} />
    case 'group': {
      const obj = value && typeof value === 'object' && !Array.isArray(value) ? (value as Data) : {}
      return (
        <fieldset className="field group">
          <legend>{f.label || f.name}</legend>
          {help}
          <Fields fields={f.fields || []} value={obj} onChange={onChange} ctx={ctx} disabled={disabled} />
        </fieldset>
      )
    }
    case 'list': {
      const items = Array.isArray(value) ? value : []
      const plain = !f.fields || f.fields.length === 0
      return (
        <fieldset className="field group">
          <legend>{f.label || f.name}</legend>
          {help}
          <ol className="list-items">
            {items.map((it, i) => (
              <li key={i} className="list-item">
                {plain ? (
                  <input value={typeof it === 'string' ? it : JSON.stringify(it)} disabled={disabled} onChange={(e) => onChange(items.map((x, j) => (j === i ? e.target.value : x)))} aria-label={`${f.label || f.name} ${i + 1}`} />
                ) : (
                  <Fields fields={f.fields!} value={it && typeof it === 'object' ? (it as Data) : {}} onChange={(d) => onChange(items.map((x, j) => (j === i ? d : x)))} ctx={ctx} disabled={disabled} />
                )}
                <span className="list-tools">
                  <button type="button" className="ghost small" disabled={disabled || i === 0} onClick={() => onChange(move(items, i, i - 1))} aria-label="Move up">↑</button>
                  <button type="button" className="ghost small" disabled={disabled || i === items.length - 1} onClick={() => onChange(move(items, i, i + 1))} aria-label="Move down">↓</button>
                  <button type="button" className="ghost small" disabled={disabled} onClick={() => onChange(items.filter((_, j) => j !== i))}>Remove</button>
                </span>
              </li>
            ))}
          </ol>
          {!disabled && <button type="button" className="ghost small" onClick={() => onChange([...items, plain ? '' : defaults(f.fields!)])}>Add {plain ? 'a line' : 'an item'}</button>}
        </fieldset>
      )
    }
    case 'link':
      return (
        <label className="field">
          {label}
          <input value={str} disabled={disabled} placeholder="/about/ or https://…" onChange={(e) => onChange(e.target.value)} />
          {help}
          {missing}
        </label>
      )
    default:
      return (
        <label className="field">
          {label}
          <input value={str} disabled={disabled} required={f.required} onChange={(e) => onChange(e.target.value)} />
          {help}
          {missing}
        </label>
      )
  }
}

export function ImageField({ f, label, help, value, onChange, ctx, disabled }: { f: Field; label: React.ReactNode; help: React.ReactNode; value: string; onChange: (v: unknown) => void; ctx: Ctx; disabled?: boolean }) {
  const [picking, setPicking] = useState(false)
  const [items, setItems] = useState<MediaItem[]>()
  const [uploading, setUploading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)
  const [upErr, setUpErr] = useState<string>()
  const send = async (file?: File) => {
    if (!file || !ctx.upload) return
    setUploading(true)
    setUpErr(undefined)
    try {
      onChange(await ctx.upload(file))
      setPicking(false)
    } catch (e) {
      setUpErr((e as Error).message)
    } finally {
      setUploading(false)
    }
  }
  useEffect(() => {
    if (picking && !items) api.media(ctx.id, ctx.branch).then(setItems, () => setItems([]))
  }, [picking])
  return (
    <div className="field">
      <label className="field">
        {label}
        <span className="image-row">
          <input value={value} disabled={disabled} placeholder="/media/…" onChange={(e) => onChange(e.target.value)} />
          {!disabled && <button type="button" className="ghost small" onClick={() => setPicking(!picking)}>{picking ? 'Close' : 'Choose'}</button>}
          {!disabled && ctx.upload && (
            <button type="button" className="ghost small" disabled={uploading} onClick={() => fileRef.current?.click()}>{uploading ? 'Uploading…' : 'Upload'}</button>
          )}
        </span>
      </label>
      {ctx.upload && <input ref={fileRef} type="file" accept={ACCEPT} className="visually-hidden" tabIndex={-1} aria-hidden="true" onChange={(e) => { send(e.target.files?.[0]); e.target.value = '' }} />}
      {upErr && <span className="field-warn small">{upErr}</span>}
      {help}
      {picking && (
        <ul className="media-grid small">
          {items?.length === 0 && <li className="muted small">No pictures yet{ctx.upload ? '; upload one.' : '; add one from the body with Insert picture.'}</li>}
          {items?.map((m) => (
            <li key={m.url}>
              <button type="button" className={`media-pick ${value === m.url ? 'on' : ''}`} onClick={() => { onChange(m.url); setPicking(false) }}>
                <img src={api.mediaFile(ctx.id, m.thumb, ctx.branch)} alt={m.alt || m.url} />
              </button>
            </li>
          ))}
        </ul>
      )}
      {f.required && !value && <span className="field-warn small">Needed</span>}
    </div>
  )
}
