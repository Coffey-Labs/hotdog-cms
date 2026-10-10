import { useCallback, useEffect, useMemo, useState, type MouseEvent, type ReactNode } from 'react'
import logoDark from '../../brand/svg/logo-header-dark.svg'
import logoLight from '../../brand/svg/logo-header-light.svg'
import { api, signInURL, SignedOut, type SessionInfo, type SiteDetail, type SiteSummary } from './api'
import { go, href, useRoute, type Route } from './route'
import { MediaLibrary } from './Media'
import { PageEditor } from './PageEditor'
import { Present } from './Present'

function Link({ to, children, className }: { to: Route; children: ReactNode; className?: string }) {
  const onClick = (e: MouseEvent<HTMLAnchorElement>) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return
    e.preventDefault()
    go(to)
  }
  return <a className={className} href={href(to)} onClick={onClick}>{children}</a>
}

function useLoad<T>(load: () => Promise<T>, deps: unknown[]) {
  const [state, setState] = useState<{ data?: T; error?: Error; loading: boolean }>({ loading: true })
  const run = useCallback(() => {
    setState((s) => ({ ...s, loading: true }))
    load().then((data) => setState({ data, loading: false }), (error: Error) => setState({ error, loading: false }))
  }, deps)
  useEffect(run, [run])
  return { ...state, reload: run }
}

export function App() {
  const route = useRoute()
  const session = useLoad(api.session, [])
  if (session.loading && !session.data) return <Shell><p className="muted pad">Loading…</p></Shell>
  if (session.error) return <Shell><Problem error={session.error} /></Shell>
  const s = session.data!
  if (!s.signedIn) return <SignIn session={s} />
  return (
    <Shell session={s} onSignOut={() => api.logout().then(() => session.reload())}>
      {route.view === 'sites' && <Sites />}
      {route.view === 'site' && <SiteView id={route.id} branch={route.branch} />}
      {route.view === 'page' && <PageRoute id={route.id} source={route.source} branch={route.branch} />}
      {route.view === 'present' && <Present id={route.id} url={route.url} branch={route.branch} />}
      {route.view === 'media' && <MediaLibrary id={route.id} branch={route.branch} />}
    </Shell>
  )
}

function Shell({ children, session, onSignOut }: { children: ReactNode; session?: SessionInfo; onSignOut?: () => void }) {
  return (
    <div className="app">
      <header className="top">
        <Link to={{ view: 'sites' }} className="brand">
          <img className="logo only-light" src={logoLight} alt="HotDog CMS" />
          <img className="logo only-dark" src={logoDark} alt="HotDog CMS" />
          <span className="brand-sub">Editor</span>
        </Link>
        {session?.user && (
          <div className="who">
            <span title={`Signed in with ${session.user.forge}`}>{session.user.name} <span className="muted">@{session.user.login}</span></span>
            <button type="button" className="ghost" onClick={onSignOut}>Sign out</button>
          </div>
        )}
      </header>
      <main id="main">{children}</main>
    </div>
  )
}

function Problem({ error }: { error: Error }) {
  if (error instanceof SignedOut) {
    return <div className="pad"><p>{error.message}</p><p><button type="button" className="button" onClick={() => location.reload()}>Sign in again</button></p></div>
  }
  return <div className="pad notice bad" role="alert">{error.message}</div>
}

function SignIn({ session }: { session: SessionInfo }) {
  const ret = location.pathname + location.search
  return (
    <div className="signin">
      <div className="card">
        <img className="signin-logo only-light" src={logoLight} alt="HotDog CMS" />
        <img className="signin-logo only-dark" src={logoDark} alt="HotDog CMS" />
        <h1>Sign in</h1>
        <p className="muted">
          With the git platform your sites live on. The editor keeps no accounts of its own: what you can open and change is
          what the platform already lets you, and every change is made as you.
        </p>
        <div className="stack">
          {session.forges.map((f) => (
            <a key={f.host} className="button primary" href={signInURL(f.host, ret)}>
              Sign in with {label(f.kind)} <span className="muted-on">· {f.host}</span>
            </a>
          ))}
          {session.forges.length === 0 && <p className="notice bad">No git platform is configured for this editor.</p>}
        </div>
      </div>
    </div>
  )
}

function label(kind: string) {
  return ({ gitea: 'Gitea', forgejo: 'Forgejo', github: 'GitHub', gitlab: 'GitLab', bitbucket: 'Bitbucket' } as Record<string, string>)[kind] || kind
}

function Sites() {
  const sites = useLoad(api.sites, [])
  return (
    <section className="pad">
      <h1>Your sites</h1>
      {sites.loading && !sites.data && <p className="muted">Asking the platform which sites you can open…</p>}
      {sites.error && <Problem error={sites.error} />}
      {sites.data && sites.data.length === 0 && <p className="muted">None of the sites this editor knows about are open to your account.</p>}
      <ul className="tiles">
        {sites.data?.map((s: SiteSummary) => (
          <li key={s.id}>
            <Link to={{ view: 'site', id: s.id }} className="tile">
              <strong>{s.name}</strong>
              <span className="muted mono">{s.repo.replace(/^https?:\/\//, '').replace(/\.git$/, '')}</span>
              <span className="pill">{s.canWrite ? 'You can edit' : 'Read only'}</span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}

function SiteView({ id, branch }: { id: string; branch?: string }) {
  const site = useLoad(() => api.site(id, branch), [id, branch])
  const [collection, setCollection] = useState<string>('')
  const d = site.data
  const pages = useMemo(() => (d?.pages || []).filter((p) => (collection === '' ? true : collection === '-' ? !p.collection : p.collection === collection)), [d, collection])
  if (site.loading && !d) return <p className="pad muted">Fetching the site with your account…</p>
  if (site.error) return <Problem error={site.error} />
  if (!d) return null
  const loose = (d.pages || []).filter((p) => !p.collection).length
  return (
    <div className="split">
      <aside className="side" aria-label="Site">
        <div className="side-head">
          <Link to={{ view: 'sites' }} className="back">← Sites</Link>
          <h1>{d.name}</h1>
          <p className="muted mono small">{d.branch === d.publishBranch ? `${d.branch} · published branch` : `${d.branch} · draft`}</p>
        </div>
        <nav aria-label="Pages">
          <h2 className="side-label">Pages</h2>
          <button type="button" className={`side-item ${collection === '' ? 'on' : ''}`} onClick={() => setCollection('')}>All pages <span>{(d.pages || []).length}</span></button>
          {loose > 0 && <button type="button" className={`side-item ${collection === '-' ? 'on' : ''}`} onClick={() => setCollection('-')}>Pages <span>{loose}</span></button>}
          {(d.collections || []).map((c) => (
            <button type="button" key={c.name} className={`side-item ${collection === c.name ? 'on' : ''}`} onClick={() => setCollection(c.name)}>{c.title} <span>{c.count}</span></button>
          ))}
        </nav>
        <nav aria-label="Site files">
          <h2 className="side-label">Files</h2>
          <Link to={{ view: 'media', id, branch }} className="side-item">Pictures</Link>
        </nav>
        <nav aria-label="Drafts">
          <h2 className="side-label">Drafts</h2>
          {d.branch !== d.publishBranch && <Link to={{ view: 'site', id }} className="side-item">← Back to {d.publishBranch}</Link>}
          {(d.drafts || []).length === 0 && <p className="muted small side-note">No drafts.</p>}
          {(d.drafts || []).map((b) => (
            <Link key={b.branch} to={{ view: 'site', id, branch: b.branch }} className={`side-item ${d.branch === b.branch ? 'on' : ''}`}>{b.branch}</Link>
          ))}
        </nav>
        <nav aria-label="Reviews">
          <h2 className="side-label">In review</h2>
          {(d.reviews || []).length === 0 && <p className="muted small side-note">Nothing waiting for review.</p>}
          {(d.reviews || []).map((r) => (
            <a key={r.number} className="side-item" href={r.url} target="_blank" rel="noopener noreferrer">#{r.number} {r.title}</a>
          ))}
        </nav>
        {d.notYet && <p className="muted small side-note">{d.notYet}</p>}
      </aside>
      <section className="content">
        <Checks build={d.build} />
        <div className="toolbar">
          <h2>{collection === '' ? 'All pages' : collection === '-' ? 'Pages' : (d.collections || []).find((c) => c.name === collection)?.title}</h2>
          <span className="toolbar-actions">
            {d.canWrite && <NewPage id={id} collections={d.collections} />}
            {d.preview && <button type="button" className="button" onClick={() => go({ view: 'present', id, url: d.preview!, branch })}>Present</button>}
            {d.preview && <a className="button" href={d.preview} target="_blank" rel="noopener noreferrer">Open preview ↗</a>}
          </span>
        </div>
        <table className="pages">
          <thead><tr><th>Title</th><th>Address</th><th>Date</th></tr></thead>
          <tbody>
            {pages.map((p) => (
              <tr key={p.source}>
                <td>
                  <Link to={{ view: 'page', id, source: p.source!, branch }}>{p.title}</Link>
                  {p.draft && <span className="pill warn">draft</span>}
                  {p.redirect && <span className="pill">redirects</span>}
                </td>
                <td className="mono small">{p.path}</td>
                <td className="small">{p.date || ''}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </div>
  )
}

function Checks({ build }: { build: SiteDetail['build'] }) {
  const [open, setOpen] = useState(false)
  if (build.failed) return <div className="notice bad" role="alert"><strong>This branch doesn't build.</strong> {build.failed}</div>
  const problems = build.problems || []
  return (
    <div className={`notice ${build.errors ? 'bad' : build.warnings ? 'warn' : 'good'}`}>
      <div className="notice-row">
        <span>
          {build.errors ? <strong>{build.errors} check error{build.errors === 1 ? '' : 's'}: this can't be published until they're fixed.</strong> : <strong>Passes the checks.</strong>}
          {build.warnings > 0 && <> {build.warnings} warning{build.warnings === 1 ? '' : 's'}.</>}
          <span className="muted"> {build.pages} pages built from {build.commit.slice(0, 10)}.</span>
        </span>
        {problems.length > 0 && <button type="button" className="ghost" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? 'Hide' : 'Show'} details</button>}
      </div>
      {open && (
        <ul className="problems">
          {problems.map((p, i) => (
            <li key={i} className={p.Level}><span className="pill">{p.Rule}</span> <span className="mono small">{p.File}</span> {p.What}</li>
          ))}
        </ul>
      )}
    </div>
  )
}

function PageRoute({ id, source, branch }: { id: string; source: string; branch?: string }) {
  const site = useLoad(() => api.site(id, branch), [id, branch])
  return <PageEditor id={id} source={source} branch={branch} site={site.data} onSiteChanged={site.reload} />
}

function NewPage({ id, collections }: { id: string; collections: SiteDetail['collections'] }) {
  const [open, setOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [col, setCol] = useState(collections?.[0]?.name || '')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string>()
  if (!open) return <button type="button" className="button" onClick={() => setOpen(true)}>New page</button>
  const create = async () => {
    setBusy(true)
    setErr(undefined)
    try {
      const r = await api.newPage(id, { collection: col, title })
      go({ view: 'page', id, source: r.source, branch: r.branch })
    } catch (e) {
      setErr((e as Error).message)
      setBusy(false)
    }
  }
  return (
    <div className="newpage form">
      <label>In
        <select value={col} onChange={(e) => setCol(e.target.value)}>
          {(collections || []).map((c) => <option key={c.name} value={c.name}>{c.title}</option>)}
          <option value="">Pages (top level)</option>
        </select>
      </label>
      <label>Title<input value={title} autoFocus onChange={(e) => setTitle(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && title.trim() && create()} /></label>
      <div className="actions">
        <button type="button" className="button primary" disabled={busy || !title.trim()} onClick={create}>Start a draft</button>
        <button type="button" className="ghost" onClick={() => setOpen(false)}>Cancel</button>
      </div>
      {err && <p className="notice bad">{err}</p>}
    </div>
  )
}
