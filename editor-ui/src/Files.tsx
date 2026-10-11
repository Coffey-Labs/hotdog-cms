import { useEffect, useMemo, useRef, useState } from 'react'
import { api, type Commit, type FileInfo, type SiteDetail } from './api'
import { useCommands, useShortcut } from './Commands'
import { DiffView } from './Diff'
import { go } from './route'

// Every file of a site, for people who know what they're changing: any text
// file edited as raw text, the diff shown before it's saved, files added and
// deleted, the history of each. A save is a commit on a draft, checked and
// reviewed like everything else.

function kb(n: number) {
  return n >= 1024 ? `${Math.round(n / 1024)} KB` : `${n} B`
}

export function FilesView({ id, branch, path }: { id: string; branch?: string; path?: string }) {
  const [files, setFiles] = useState<FileInfo[]>()
  const [error, setError] = useState<string>()
  const [filter, setFilter] = useState('')
  const [site, setSite] = useState<SiteDetail>()
  const [orig, setOrig] = useState<{ content: string; exists: boolean; commit: string }>()
  const [text, setText] = useState('')
  const [mode, setMode] = useState<'edit' | 'review' | 'history' | 'delete' | 'compare'>('edit')
  const [diff, setDiff] = useState('')
  const [message, setMessage] = useState('')
  const [history, setHistory] = useState<Commit[]>()
  const [status, setStatus] = useState<{ kind: 'info' | 'good' | 'bad'; text: string }>()
  const [busy, setBusy] = useState(false)
  const [pending, setPending] = useState<string>() // a file to open once unsaved changes are dealt with
  const [newPath, setNewPath] = useState<string>()
  const keepStatus = useRef(false) // a save that moved to a new draft keeps its message

  const loadList = () => api.files(id, branch).then((r) => setFiles(r.files), (e: Error) => setError(e.message))
  useEffect(() => {
    loadList()
    api.site(id, branch).then(setSite, () => undefined)
  }, [id, branch])
  useEffect(() => {
    setMode('edit')
    if (!keepStatus.current) setStatus(undefined)
    keepStatus.current = false
    setOrig(undefined)
    if (!path) return
    api.file(id, path, branch).then(
      (f) => {
        setOrig(f)
        setText(f.content)
      },
      (e: Error) => setStatus({ kind: 'bad', text: e.message }),
    )
  }, [id, branch, path])

  const cur = branch || site?.publishBranch || ''
  const onDraft = site ? cur !== site.publishBranch : false
  const dirty = orig !== undefined && (text !== orig.content || !orig.exists)
  const canWrite = !!site?.canWrite

  const shown = useMemo(() => (files || []).filter((f) => !filter || f.path.toLowerCase().includes(filter.toLowerCase())), [files, filter])

  const open = (p: string) => {
    if (dirty && p !== path) return setPending(p)
    go({ view: 'files', id, branch, path: p })
  }
  const review = async () => {
    if (!path || !dirty) return
    setBusy(true)
    try {
      const r = await api.fileDiff(id, { branch: cur, path, content: text })
      setDiff(r.diff)
      setMessage(orig?.exists ? `Edit ${path}` : `Add ${path}`)
      setMode('review')
    } catch (e) {
      setStatus({ kind: 'bad', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }
  const save = async (del = false) => {
    if (!path || !orig) return
    setBusy(true)
    setStatus({ kind: 'info', text: del ? 'Deleting…' : 'Saving…' })
    try {
      const r = await api.fileSave(id, { branch: cur, baseCommit: orig.commit, path, content: del ? undefined : text, delete: del, message })
      setStatus({ kind: 'good', text: `${del ? 'Deleted' : 'Saved'} on ${r.created ? `a new draft, ${r.branch}` : r.branch}.` })
      if (r.created || r.branch !== branch) {
        keepStatus.current = true
        go({ view: 'files', id, branch: r.branch, path: del ? undefined : path })
      } else if (del) {
        keepStatus.current = true
        go({ view: 'files', id, branch, path: undefined })
      }
      else {
        const f = await api.file(id, path, r.branch)
        setOrig(f)
        setText(f.content)
        setMode('edit')
      }
      loadList()
    } catch (e) {
      setStatus({ kind: 'bad', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }
  const showHistory = async () => {
    if (!path) return
    setMode('history')
    setHistory(undefined)
    api.log(id, cur, path).then((r) => setHistory(r.commits), (e: Error) => setStatus({ kind: 'bad', text: e.message }))
  }
  const compare = async () => {
    setMode('compare')
    setDiff('')
    api.compare(id, cur).then((r) => setDiff(r.stat ? `${r.stat}\n\n${r.diff}` : r.diff), (e: Error) => setStatus({ kind: 'bad', text: e.message }))
  }

  useShortcut('mod+s', () => (mode === 'review' ? save() : review()), !!path)
  useCommands(() => [
    ...(path && canWrite ? [{ id: 'files-review', title: 'Review changes and save', group: 'File', keys: 'Ctrl+S', run: review }] : []),
    ...(path ? [{ id: 'files-history', title: `History of ${path}`, group: 'File', run: showHistory }] : []),
    ...(path && canWrite ? [{ id: 'files-delete', title: `Delete ${path}`, group: 'File', run: () => setMode('delete') }] : []),
    ...(onDraft ? [{ id: 'files-compare', title: `Compare ${cur} with ${site?.publishBranch}`, group: 'Draft', run: compare }] : []),
    ...(canWrite ? [{ id: 'files-new', title: 'New file…', group: 'File', run: () => setNewPath('') }] : []),
    ...(files || []).filter((f) => f.text).map((f) => ({ id: `file:${f.path}`, title: f.path, group: 'Open file', run: () => open(f.path) })),
  ])

  if (error) return <p className="pad notice bad" role="alert">{error}</p>
  return (
    <div className="split files-view">
      <aside className="side" aria-label="Files">
        <div className="side-head">
          <a className="back" href={`/sites/${encodeURIComponent(id)}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id, branch }) }}>← Site</a>
          <h1>Files</h1>
          <p className="muted mono small">{onDraft ? `${cur} · draft` : `${cur} · published branch`}</p>
        </div>
        <input className="file-filter" placeholder="Filter files" value={filter} onChange={(e) => setFilter(e.target.value)} aria-label="Filter files" />
        {canWrite && newPath === undefined && <button type="button" className="ghost small file-new" onClick={() => setNewPath('')}>New file</button>}
        {newPath !== undefined && (
          <form className="file-new-form" onSubmit={(e) => { e.preventDefault(); if (newPath.trim()) { setNewPath(undefined); open(newPath.trim()) } }}>
            <input autoFocus placeholder="data/team.yaml" value={newPath} onChange={(e) => setNewPath(e.target.value)} aria-label="New file's path" />
            <button type="submit" className="ghost small">Create</button>
            <button type="button" className="ghost small" onClick={() => setNewPath(undefined)}>Cancel</button>
          </form>
        )}
        {onDraft && <button type="button" className="ghost small file-new" onClick={compare}>Compare with {site?.publishBranch}</button>}
        {!files && <p className="muted small side-note">Listing files…</p>}
        <ul className="file-list">
          {shown.map((f) => (
            <li key={f.path}>
              <button type="button" className={`side-item ${f.path === path ? 'on' : ''}`} disabled={!f.text} title={f.text ? '' : 'Not text: change it through git'} onClick={() => open(f.path)}>
                <span className="mono">{f.path}</span> <span>{kb(f.size)}</span>
              </button>
            </li>
          ))}
        </ul>
      </aside>
      <section className="content file-pane">
        {pending && (
          <div className="notice warn" role="alert">
            <strong>{path} has unsaved changes.</strong>
            <div className="actions-row">
              <button type="button" className="button" onClick={() => setPending(undefined)}>Keep editing</button>
              <button type="button" className="ghost" onClick={() => { const p = pending; setPending(undefined); setOrig(undefined); go({ view: 'files', id, branch, path: p }) }}>Discard them and open {pending}</button>
            </div>
          </div>
        )}
        {status && <p className={`notice ${status.kind}`} role="status">{status.text}</p>}
        {mode === 'compare' && (
          <>
            <div className="toolbar"><h2>{cur} compared with {site?.publishBranch}</h2><button type="button" className="ghost small" onClick={() => setMode('edit')}>Close</button></div>
            <p className="muted small">What publishing this draft would change.</p>
            <DiffView diff={diff} empty="Nothing differs from the published branch." />
          </>
        )}
        {mode !== 'compare' && !path && <p className="muted">Choose a file to edit, or start a new one. Saving shows the changes first, and goes to a draft.</p>}
        {mode !== 'compare' && path && orig && (
          <>
            <div className="toolbar">
              <h2 className="mono">{path}{!orig.exists && <span className="pill warn">new</span>}</h2>
              <span className="toolbar-actions">
                <button type="button" className="button primary" disabled={!dirty || busy || !canWrite} onClick={review}>Review changes <kbd>Ctrl+S</kbd></button>
                {orig.exists && <button type="button" className="ghost small" onClick={showHistory}>History</button>}
                {orig.exists && canWrite && <button type="button" className="ghost small" onClick={() => setMode('delete')}>Delete</button>}
              </span>
            </div>
            {mode === 'review' && (
              <div className="review-box">
                <DiffView diff={diff} />
                <label className="field">Commit message<input value={message} onChange={(e) => setMessage(e.target.value)} /></label>
                <div className="actions-row">
                  <button type="button" className="button primary" disabled={busy} onClick={() => save()}>{onDraft ? 'Save' : 'Save as draft'} <kbd>Ctrl+S</kbd></button>
                  <button type="button" className="ghost" onClick={() => setMode('edit')}>Back to editing</button>
                  {!onDraft && <span className="muted small">Starts a draft; the published site is unchanged.</span>}
                </div>
              </div>
            )}
            {mode === 'delete' && (
              <div className="notice warn">
                <strong>Delete {path}?</strong> It's removed on {onDraft ? 'this draft' : 'a new draft'}, so it's back in one revert if it was a mistake.
                <label className="field">Commit message<input value={message || `Delete ${path}`} onChange={(e) => setMessage(e.target.value)} /></label>
                <div className="actions-row">
                  <button type="button" className="button" disabled={busy} onClick={() => save(true)}>Delete it</button>
                  <button type="button" className="ghost" onClick={() => setMode('edit')}>Cancel</button>
                </div>
              </div>
            )}
            {mode === 'history' && (
              <div className="history">
                <div className="toolbar"><h3>History</h3><button type="button" className="ghost small" onClick={() => setMode('edit')}>Close</button></div>
                {!history && <p className="muted small">Reading the history…</p>}
                <ol className="commits">
                  {history?.map((c) => (
                    <li key={c.sha}><span className="mono small">{c.sha.slice(0, 8)}</span> {c.subject} <span className="muted small">· {c.author} · {new Date(c.when).toLocaleString()}</span></li>
                  ))}
                </ol>
              </div>
            )}
            <textarea
              className="code file-edit"
              value={text}
              spellCheck={false}
              readOnly={!canWrite || mode === 'review'}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Tab' && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
                  // Tab indents; Esc then Tab leaves the editor.
                  e.preventDefault()
                  const el = e.currentTarget
                  const at = el.selectionStart
                  setText(text.slice(0, at) + '  ' + text.slice(el.selectionEnd))
                  requestAnimationFrame(() => el.setSelectionRange(at + 2, at + 2))
                } else if (e.key === 'Escape') {
                  e.currentTarget.blur()
                }
              }}
              aria-label={`Contents of ${path}`}
            />
          </>
        )}
      </section>
    </div>
  )
}
