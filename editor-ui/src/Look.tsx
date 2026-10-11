import { useEffect, useMemo, useRef, useState } from 'react'
import { api, type LookFile, type LookInfo, type LookToken, type LookValue, type SiteDetail } from './api'
import { ReviewForm } from './PageEditor'
import { go } from './route'
import { ImageField } from './Sections'
import { Split } from './Split'
import { DevicePicker, PreviewFrame, useDevice } from './Viewport'

// The Look panel: the colors, fonts, corners and logo the theme lets change
// (its look.yaml), previewed on the home page as they're picked, with every
// pair the theme says must stay readable checked as you go. Saving is a
// commit to site.yaml on a draft, reviewed and published like any change.

function luminance(hex: string) {
  let h = hex.replace('#', '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  if (h.length !== 6) return 0
  const c = [0, 2, 4].map((i) => {
    const x = parseInt(h.slice(i, i + 2), 16) / 255
    return x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]
}

function contrast(a: string, b: string) {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p)
  return Math.round(((x + 0.05) / (y + 0.05)) * 100) / 100
}

const light = (v?: LookValue) => (typeof v === 'string' ? v : v?.light) || ''
const dark = (v?: LookValue) => (typeof v === 'object' && v ? v.dark : '') || ''

function effective(lf: LookFile, vals: Record<string, LookValue>, name: string, mode: 'light' | 'dark') {
  const t = lf.tokens.find((x) => x.name === name)!
  const v = vals[name]
  if (mode === 'dark') return dark(v) || t.dark || light(v) || t.default || '#000000'
  return light(v) || t.default || '#000000'
}

export function LookPanel({ id, branch }: { id: string; branch?: string }) {
  const [info, setInfo] = useState<LookInfo>()
  const [error, setError] = useState<string>()
  const [vals, setVals] = useState<Record<string, LookValue>>({})
  const [status, setStatus] = useState<{ kind: 'info' | 'good' | 'bad'; text: string }>()
  const [busy, setBusy] = useState(false)
  const [previewURL, setPreviewURL] = useState<string>()
  const [previewNote, setPreviewNote] = useState<string>()
  const [reload, setReload] = useState(0)
  const [device, setDevice] = useDevice()
  const [site, setSite] = useState<SiteDetail>()
  const [reviewOpen, setReviewOpen] = useState(false)

  const load = () =>
    api.look(id, branch).then(
      (i) => {
        setInfo(i)
        setVals(i.values || {})
      },
      (e: Error) => setError(e.message),
    )
  useEffect(() => {
    load()
    api.site(id, branch).then(setSite, () => undefined)
  }, [id, branch])

  const cur = branch || info?.branch || ''
  const onDraft = site ? cur !== site.publishBranch : false
  const review = site?.reviews?.find((r) => r.branch === cur)
  const dirty = info ? JSON.stringify(vals) !== JSON.stringify(info.values || {}) : false

  // The preview follows the choices, a moment after the last change.
  const timer = useRef<number>(0)
  useEffect(() => {
    if (!info?.look) return
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      api.lookLive(id, { branch: cur, look: vals }).then(
        (r) => {
          if (r.error) return setPreviewNote(r.error)
          setPreviewNote(undefined)
          if (r.url !== previewURL) setPreviewURL(r.url)
          else setReload((n) => n + 1)
        },
        (e: Error) => setPreviewNote(e.message),
      )
    }, 350)
    return () => window.clearTimeout(timer.current)
  }, [vals, info])

  const checks = useMemo(() => {
    if (!info?.look) return []
    return (info.look.contrast || []).map(([fg, bg]) => ({
      fg, bg,
      light: contrast(effective(info.look!, vals, fg, 'light'), effective(info.look!, vals, bg, 'light')),
      dark: contrast(effective(info.look!, vals, fg, 'dark'), effective(info.look!, vals, bg, 'dark')),
    }))
  }, [info, vals])
  const failing = checks.filter((c) => c.light < (info?.minContrast || 4.5) || c.dark < (info?.minContrast || 4.5)).length

  const setVal = (name: string, v: LookValue | undefined) =>
    setVals((old) => {
      const { [name]: _, ...rest } = old
      return v === undefined || v === '' ? rest : { ...rest, [name]: v }
    })

  const save = async () => {
    if (!info) return
    setBusy(true)
    setStatus({ kind: 'info', text: 'Saving…' })
    try {
      const r = await api.lookSave(id, { branch: cur, baseCommit: info.commit, look: vals })
      setStatus({ kind: 'good', text: r.created ? `Saved. The new look is on a draft, ${r.branch}; the published site is unchanged.` : `Saved to ${r.branch}.` })
      if (r.created || r.branch !== branch) go({ view: 'look', id, branch: r.branch })
      else load()
    } catch (e) {
      setStatus({ kind: 'bad', text: `Not saved: ${(e as Error).message}` })
    } finally {
      setBusy(false)
    }
  }
  const publish = async () => {
    setBusy(true)
    setStatus({ kind: 'info', text: 'Checking the draft and publishing…' })
    try {
      await api.publish(id, { branch: cur })
      setStatus({ kind: 'good', text: 'Published: the change is merged. The live site updates when its publishing pipeline runs.' })
      if (site) go({ view: 'look', id, branch: site.publishBranch })
    } catch (e) {
      setStatus({ kind: 'bad', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  if (error) return <p className="pad notice bad" role="alert">{error}</p>
  if (!info) return <p className="pad muted">Loading the look…</p>
  const back = (
    <a className="back" href={`/sites/${encodeURIComponent(id)}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id, branch }) }}>← Site</a>
  )
  if (!info.look) {
    return (
      <section className="pad">
        {back}
        <h1>Look</h1>
        <p className="muted">This site's theme doesn't say what can be changed: it has no <span className="mono">look.yaml</span>. A developer adds one to offer colors, fonts and a logo here.</p>
      </section>
    )
  }
  const canWrite = info.canWrite

  return (
    <Split
      left={
        <section className="editor-pane">
          <div className="pane-head">
            {back}
            <span className={`pill ${onDraft ? 'warn' : ''}`}>{onDraft ? `draft: ${cur}` : `${cur} · published`}</span>
          </div>
          <h1>Look</h1>
          <p className="muted small">What this site's theme lets change. The preview shows the home page as you pick.</p>
          <div className="actions-row">
            <button type="button" className="button primary" disabled={!dirty || busy || !canWrite} onClick={save}>{onDraft ? 'Save' : 'Save as draft'}</button>
            {onDraft && !review && <button type="button" className="button" disabled={busy || dirty || !canWrite} onClick={() => setReviewOpen(!reviewOpen)}>Submit for review</button>}
            {onDraft && review && <a className="button" href={review.url} target="_blank" rel="noopener noreferrer">Review #{review.number} ↗</a>}
            {onDraft && review && <button type="button" className="button" disabled={busy || dirty || !canWrite} onClick={publish}>Publish</button>}
            {dirty && <span className="muted small">Unsaved changes</span>}
            {!canWrite && <span className="muted small">Read only: the platform doesn't let you change this site.</span>}
          </div>
          {status && <p className={`notice ${status.kind}`} role="status">{status.text}</p>}
          {reviewOpen && <ReviewForm id={id} branch={cur} title="the look" onDone={(m) => { setReviewOpen(false); setStatus({ kind: 'good', text: m }); api.site(id, branch).then(setSite) }} onError={(m) => setStatus({ kind: 'bad', text: m })} />}

          <div className={`notice ${failing ? 'bad' : 'good'}`}>
            <strong>{failing ? `${failing} pair${failing === 1 ? '' : 's'} too hard to read: this can't be published until fixed.` : 'Every pair is readable.'}</strong>
            <table className="contrast">
              <thead><tr><th>Text</th><th>On</th><th>Light</th><th>Dark</th></tr></thead>
              <tbody>
                {checks.map((c) => (
                  <tr key={c.fg + c.bg}>
                    <td>{label(info.look!, c.fg)}</td>
                    <td>{label(info.look!, c.bg)}</td>
                    {[c.light, c.dark].map((r, i) => (
                      <td key={i} className={r < info.minContrast ? 'fail' : 'pass'}>{r.toFixed(2)}:1 {r < info.minContrast ? '✗' : '✓'}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
            <span className="muted small">Text needs at least {info.minContrast}:1 against what it's on.</span>
          </div>

          <div className="form look-form">
            {info.look.tokens.map((t) => (
              <TokenInput key={t.name} t={t} value={vals[t.name]} onChange={(v) => setVal(t.name, v)} id={id} branch={cur} disabled={!canWrite} />
            ))}
          </div>
        </section>
      }
      right={
        <section className="preview-pane" aria-label="Preview">
          <div className="preview-bar">
            <DevicePicker device={device} onChange={setDevice} />
          </div>
          {previewNote && <p className="notice warn small preview-note">{previewNote}</p>}
          <PreviewFrame src={previewURL} device={device} title="Preview of the look" reloadKey={reload} />
        </section>
      }
    />
  )
}

function label(lf: LookFile, name: string) {
  return lf.tokens.find((t) => t.name === name)?.label || name
}

function TokenInput({ t, value, onChange, id, branch, disabled }: { t: LookToken; value?: LookValue; onChange: (v: LookValue | undefined) => void; id: string; branch: string; disabled?: boolean }) {
  const reset = value !== undefined && !disabled && <button type="button" className="ghost small" onClick={() => onChange(undefined)}>Use the theme's</button>
  const help = t.help && <span className="muted small">{t.help}</span>
  switch (t.type) {
    case 'color': {
      const l = light(value)
      const d = dark(value)
      const set = (nl: string, nd: string) => onChange(nd ? { light: nl || t.default, dark: nd } : nl || undefined)
      return (
        <fieldset className="field group token">
          <legend>{t.label || t.name} {reset}</legend>
          {help}
          <div className="color-row">
            <label className="color-pick">
              <input type="color" value={l || t.default} disabled={disabled} onChange={(e) => set(e.target.value, d)} aria-label={`${t.label}, light`} />
              <span className="mono small">{l || `${t.default} (theme)`}</span>
            </label>
            <label className="check small">
              <input type="checkbox" checked={!!d} disabled={disabled} onChange={(e) => set(l, e.target.checked ? t.dark || l || t.default || '#000000' : '')} /> Different in dark mode
            </label>
            {d && (
              <label className="color-pick">
                <input type="color" value={d} disabled={disabled} onChange={(e) => set(l, e.target.value)} aria-label={`${t.label}, dark`} />
                <span className="mono small">{d} (dark)</span>
              </label>
            )}
            {!d && t.dark && <span className="muted small">Dark mode: {t.dark} (theme)</span>}
          </div>
        </fieldset>
      )
    }
    case 'font':
    case 'select':
      return (
        <label className="field token">
          <span className="field-label">{t.label || t.name} {reset}</span>
          <select value={light(value)} disabled={disabled} onChange={(e) => onChange(e.target.value || undefined)}>
            <option value="">The theme's ({t.default})</option>
            {(t.options || []).map((o) => <option key={o.name} value={o.name} style={t.type === 'font' ? { fontFamily: o.value } : undefined}>{o.name}</option>)}
          </select>
          {help}
        </label>
      )
    case 'image':
      return (
        <ImageField
          f={{ name: t.name, label: t.label, type: 'image' }}
          label={<span className="field-label">{t.label || t.name} {reset}</span>}
          help={help}
          value={light(value)}
          onChange={(v) => onChange((v as string) || undefined)}
          ctx={{ id, branch, lib: { sections: [], data: [], collections: [] } }}
          disabled={disabled}
        />
      )
  }
}
