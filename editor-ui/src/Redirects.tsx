import { useEffect, useState } from 'react'
import { api } from './api'
import { go } from './route'

// Old addresses that should keep working, and where they go. Saved as a
// commit to site.yaml's redirects: block on a draft; the rest of the file is
// left exactly as it was. Moving a page adds its redirect by itself.

type Row = { from: string; to: string }

export function RedirectsView({ id, branch }: { id: string; branch?: string }) {
  const [rows, setRows] = useState<Row[]>()
  const [orig, setOrig] = useState<{ rows: Row[]; commit: string; branch: string; canWrite: boolean }>()
  const [status, setStatus] = useState<{ kind: 'info' | 'good' | 'bad'; text: string }>()
  const [busy, setBusy] = useState(false)
  const load = () =>
    api.redirects(id, branch).then(
      (r) => {
        setOrig({ rows: r.redirects, commit: r.commit, branch: r.branch, canWrite: r.canWrite })
        setRows(r.redirects.map((x) => ({ ...x })))
      },
      (e: Error) => setStatus({ kind: 'bad', text: e.message }),
    )
  useEffect(() => {
    load()
  }, [id, branch])

  const dirty = !!orig && JSON.stringify(rows) !== JSON.stringify(orig.rows)
  const set = (i: number, k: keyof Row, v: string) => setRows((rs) => rs!.map((r, j) => (j === i ? { ...r, [k]: v } : r)))
  const save = async () => {
    if (!orig || !rows) return
    setBusy(true)
    setStatus({ kind: 'info', text: 'Saving…' })
    try {
      const r = await api.redirectsSave(id, { branch: orig.branch, baseCommit: orig.commit, redirects: rows.filter((x) => x.from.trim() || x.to.trim()) })
      setStatus({ kind: 'good', text: `Saved on ${r.created ? `a new draft, ${r.branch}` : r.branch}.` })
      if (r.created || r.branch !== branch) go({ view: 'redirects', id, branch: r.branch })
      else load()
    } catch (e) {
      setStatus({ kind: 'bad', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="pad redirects">
      <a className="back" href={`/sites/${encodeURIComponent(id)}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id, branch }) }}>← Site</a>
      <h1>Redirects</h1>
      <p className="muted">Old addresses that should keep working. Moving a page adds its redirect by itself; add others here, for a page that's gone or a link printed somewhere.</p>
      {status && <p className={`notice ${status.kind}`} role="status">{status.text}</p>}
      {!rows && !status && <p className="muted">Reading site.yaml…</p>}
      {rows && (
        <div className="form">
          <table className="pages redirect-table">
            <thead><tr><th>Old address</th><th>Goes to</th><th></th></tr></thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i}>
                  <td><input value={r.from} placeholder="/old-page/" disabled={!orig?.canWrite} onChange={(e) => set(i, 'from', e.target.value)} aria-label={`Old address ${i + 1}`} /></td>
                  <td><input value={r.to} placeholder="/new-page/ or https://…" disabled={!orig?.canWrite} onChange={(e) => set(i, 'to', e.target.value)} aria-label={`Goes to ${i + 1}`} /></td>
                  <td>{orig?.canWrite && <button type="button" className="ghost small" onClick={() => setRows(rows.filter((_, j) => j !== i))}>Remove</button>}</td>
                </tr>
              ))}
              {rows.length === 0 && <tr><td colSpan={3} className="muted small">No redirects.</td></tr>}
            </tbody>
          </table>
          {orig?.canWrite && (
            <div className="actions-row">
              <button type="button" className="ghost small" onClick={() => setRows([...rows, { from: '', to: '' }])}>Add a redirect</button>
              <button type="button" className="button primary" disabled={!dirty || busy} onClick={save}>{orig.branch === branch ? 'Save' : 'Save as draft'}</button>
            </div>
          )}
        </div>
      )}
    </section>
  )
}
