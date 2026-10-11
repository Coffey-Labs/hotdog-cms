import { useEffect, useState } from 'react'
import { api, type Commit } from './api'
import { go } from './route'

// The site's history: every change that reached the published branch, who
// made it and when, from git itself, so it can't be edited by whoever it
// records. Undoing one is a revert on a new draft, reviewed and published
// like anything else.

export function HistoryView({ id }: { id: string }) {
  const [commits, setCommits] = useState<Commit[]>()
  const [status, setStatus] = useState<{ kind: 'info' | 'good' | 'bad'; text: string; branch?: string }>()
  const [canWrite, setCanWrite] = useState(false)
  const [busy, setBusy] = useState<string>()
  useEffect(() => {
    api.site(id).then((d) => setCanWrite(d.canWrite), () => undefined)
    api.log(id, undefined, undefined, 50).then((r) => setCommits(r.commits), (e: Error) => setStatus({ kind: 'bad', text: e.message }))
  }, [id])
  const undo = async (c: Commit) => {
    setBusy(c.sha)
    setStatus({ kind: 'info', text: `Undoing "${c.subject}"…` })
    try {
      const r = await api.revert(id, c.sha)
      setStatus({ kind: 'good', text: `"${c.subject}" is undone on a new draft, ${r.branch}. Look it over, then submit it for review and publish.`, branch: r.branch })
    } catch (e) {
      setStatus({ kind: 'bad', text: (e as Error).message })
    } finally {
      setBusy(undefined)
    }
  }
  return (
    <section className="pad history">
      <a className="back" href={`/sites/${encodeURIComponent(id)}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id }) }}>← Site</a>
      <h1>History</h1>
      <p className="muted">Every change that reached the published site, newest first. Undoing one makes a draft that reverses it; nothing changes until that's published.</p>
      {status && (
        <p className={`notice ${status.kind}`} role="status">
          {status.text}
          {status.branch && <> <a href="#" onClick={(e) => { e.preventDefault(); go({ view: 'site', id, branch: status.branch }) }}>Open the draft</a></>}
        </p>
      )}
      {!commits && !status && <p className="muted">Reading the history…</p>}
      <ol className="commit-list">
        {commits?.map((c) => (
          <li key={c.sha}>
            <div>
              <strong>{c.subject}</strong>
              <span className="muted small"> {c.author} · {new Date(c.when).toLocaleString()} · <span className="mono">{c.sha.slice(0, 8)}</span></span>
            </div>
            {canWrite && <button type="button" className="ghost small" disabled={!!busy} onClick={() => undo(c)}>{busy === c.sha ? 'Undoing…' : 'Undo on a draft'}</button>}
          </li>
        ))}
      </ol>
    </section>
  )
}
