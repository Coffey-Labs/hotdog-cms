import { useEffect, useState } from 'react'
import { api, type Analytics } from './api'
import { go } from './route'

// The analytics wizard and privacy settings. Statistics wait for a visitor's
// consent (the banner asks; "Cookie settings" changes the answer) unless the
// site chooses otherwise, and that choice is written into site.yaml where a
// reviewer sees it. The privacy notice lists what the site uses, from these
// settings, each time the site is built.

const PROVIDERS: { id: Analytics['provider'] | ''; label: string; about: string }[] = [
  { id: '', label: 'None', about: 'No visit statistics.' },
  { id: 'plausible', label: 'Plausible', about: 'Counts visits without cookies; hosted or self-hosted.' },
  { id: 'matomo', label: 'Matomo', about: 'Self-hosted statistics.' },
  { id: 'ga4', label: 'Google Analytics', about: 'Google Analytics 4. Visitors’ data goes to Google.' },
]

export function PrivacyView({ id, branch }: { id: string; branch?: string }) {
  const [info, setInfo] = useState<{ commit: string; branch: string; canWrite: boolean; start: string }>()
  const [an, setAn] = useState<Analytics | null>(null)
  const [pv, setPv] = useState<Record<string, string>>({})
  const [status, setStatus] = useState<{ kind: 'info' | 'good' | 'bad'; text: string }>()
  const [busy, setBusy] = useState(false)
  const load = () =>
    api.privacy(id, branch).then(
      (r) => {
        setAn(r.analytics)
        setPv(r.privacy)
        setInfo({ commit: r.commit, branch: r.branch, canWrite: r.canWrite, start: JSON.stringify([r.analytics, r.privacy]) })
      },
      (e: Error) => setStatus({ kind: 'bad', text: e.message }),
    )
  useEffect(() => {
    load()
  }, [id, branch])

  const dirty = !!info && JSON.stringify([an, pv]) !== info.start
  const gated = !an || an.gated !== false
  const field = (k: keyof Analytics, label: string, ph: string, help?: string) => (
    <label className="field">
      {label}
      <input value={(an?.[k] as string) || ''} placeholder={ph} disabled={!info?.canWrite} onChange={(e) => setAn({ ...an!, [k]: e.target.value })} />
      {help && <span className="muted small">{help}</span>}
    </label>
  )
  const save = async () => {
    if (!info) return
    setBusy(true)
    setStatus({ kind: 'info', text: 'Saving…' })
    try {
      const r = await api.privacySave(id, { branch: info.branch, baseCommit: info.commit, analytics: an, privacy: pv })
      setStatus({ kind: 'good', text: `Saved on ${r.created ? `a new draft, ${r.branch}` : r.branch}. Its preview's /privacy/ page shows the new list.` })
      if (r.created || r.branch !== branch) go({ view: 'privacy', id, branch: r.branch })
      else load()
    } catch (e) {
      setStatus({ kind: 'bad', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="pad privacy-view">
      <a className="back" href={`/sites/${encodeURIComponent(id)}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id, branch }) }}>← Site</a>
      <h1>Privacy and statistics</h1>
      <p className="muted">What the site counts, and who answers for visitors' data. The privacy notice at /privacy/ is written from these settings every time the site is built.</p>
      {status && <p className={`notice ${status.kind}`} role="status">{status.text}</p>}
      {info && (
        <div className="form wizard">
          <fieldset className="field group">
            <legend>Visit statistics</legend>
            <div className="provider-pick" role="radiogroup" aria-label="Statistics">
              {PROVIDERS.map((p) => (
                <label key={p.id} className={`provider ${(an?.provider || '') === p.id ? 'on' : ''}`}>
                  <input type="radio" name="provider" checked={(an?.provider || '') === p.id} disabled={!info.canWrite} onChange={() => setAn(p.id ? { provider: p.id, gated: an?.gated } : null)} />
                  <strong>{p.label}</strong>
                  <span className="muted small">{p.about}</span>
                </label>
              ))}
            </div>
            {an?.provider === 'plausible' && <>{field('domain', 'Site domain in Plausible', 'example.org')}{field('src', 'Script address (if self-hosted)', 'https://plausible.io/js/script.js', 'Leave empty for plausible.io.')}</>}
            {an?.provider === 'matomo' && <>{field('url', 'Matomo address', 'https://matomo.example.org/')}{field('siteId', 'Site ID', '1')}</>}
            {an?.provider === 'ga4' && field('id', 'Measurement ID', 'G-XXXXXXX')}
            {an && (
              <label className="check field">
                <input type="checkbox" checked={gated} disabled={!info.canWrite} onChange={(e) => setAn({ ...an, gated: e.target.checked ? undefined : false })} />
                Ask visitors first (recommended)
              </label>
            )}
            {an && gated && <p className="muted small">A banner asks on the first visit, with Decline first and as prominent as Accept. Nothing is counted until someone accepts; declining later removes the counter's cookies.</p>}
            {an && !gated && <p className="notice warn small">Statistics will start on page view, without asking. That's recorded in site.yaml (gated: false), the privacy notice says so, and every check warns about it. In much of the world, counting with cookies needs consent.</p>}
          </fieldset>
          <fieldset className="field group">
            <legend>Who answers for visitors' data</legend>
            <label className="field">Person or organization<input value={pv.controller || ''} disabled={!info.canWrite} placeholder="Your name, or your company" onChange={(e) => setPv({ ...pv, controller: e.target.value })} /></label>
            <label className="field">Email to write to<input value={pv.contact || ''} disabled={!info.canWrite} placeholder="privacy@example.org" onChange={(e) => setPv({ ...pv, contact: e.target.value })} /></label>
            <label className="field">Last changed<input value={pv.updated || ''} disabled={!info.canWrite} placeholder="10 October 2026" onChange={(e) => setPv({ ...pv, updated: e.target.value })} /></label>
          </fieldset>
          {info.canWrite && (
            <div className="actions-row">
              <button type="button" className="button primary" disabled={!dirty || busy} onClick={save}>{info.branch === branch ? 'Save' : 'Save as draft'}</button>
            </div>
          )}
        </div>
      )}
    </section>
  )
}
