import { useCallback, useEffect, useMemo, useState, type MouseEvent, type ReactNode } from 'react'
import logoDark from '../../brand/svg/logo-header-dark.svg'
import logoLight from '../../brand/svg/logo-header-light.svg'
import { AccountsMenu, label } from './Accounts'
import { api, chooseAccount, signInURL, SignedOut, type SessionInfo, type SiteDetail, type SiteSummary } from './api'
import { go, href, useRoute, type Route } from './route'
import { useCommands } from './Commands'
import { Console, showConsole } from './Console'
import { FilesView } from './Files'
import { FormsView } from './Forms'
import { HistoryView } from './History'
import { LookPanel } from './Look'
import { Palette, showPalette } from './Palette'
import { PrivacyView } from './Privacy'
import { RedirectsView } from './Redirects'
import { SearchView } from './Search'
import { MediaLibrary } from './Media'
import { PageEditor } from './PageEditor'
import { Present } from './Present'
import { ThemePicker } from './Theme'

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
    <Shell session={s} onAccountsChanged={session.reload}>
      {route.view === 'sites' && <Sites />}
      {route.view === 'site' && <SiteView id={route.id} branch={route.branch} />}
      {route.view === 'page' && <PageRoute id={route.id} source={route.source} branch={route.branch} />}
      {route.view === 'present' && <Present id={route.id} url={route.url} branch={route.branch} />}
      {route.view === 'media' && <MediaLibrary id={route.id} branch={route.branch} />}
      {route.view === 'look' && <LookPanel id={route.id} branch={route.branch} />}
      {route.view === 'files' && <FilesView id={route.id} branch={route.branch} path={route.path} />}
      {route.view === 'redirects' && <RedirectsView id={route.id} branch={route.branch} />}
      {route.view === 'privacy' && <PrivacyView id={route.id} branch={route.branch} />}
      {route.view === 'search' && <SearchView id={route.id} branch={route.branch} />}
      {route.view === 'forms' && <FormsView id={route.id} branch={route.branch} form={route.form} />}
      {route.view === 'history' && <HistoryView id={route.id} />}
    </Shell>
  )
}

function Shell({ children, session, onAccountsChanged }: { children: ReactNode; session?: SessionInfo; onAccountsChanged?: () => void }) {
  return (
    <div className="app">
      <header className="top">
        <Link to={{ view: 'sites' }} className="brand">
          <img className="logo only-light" src={logoLight} alt="HotDog CMS" />
          <img className="logo only-dark" src={logoDark} alt="HotDog CMS" />
          <span className="brand-sub">Editor</span>
        </Link>
        <div className="who">
          {session?.signedIn && <button type="button" className="ghost small kbd-hint" onClick={showPalette} title="Every action, by name">Commands <kbd>Ctrl K</kbd></button>}
          <ThemePicker />
        {session?.signedIn && onAccountsChanged && <AccountsMenu session={session} onChanged={onAccountsChanged} />}
        </div>
      </header>
      <main id="main">{children}</main>
      {session?.signedIn && <GlobalCommands session={session} />}
      {session?.signedIn && <Palette />}
      {session?.signedIn && <Console />}
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
              {s.discovered && <span className="muted small" title="Not listed in editor.yaml: found because it has a site.yaml">Found in your repositories</span>}
              <span className="muted mono">{s.repo.replace(/^https?:\/\//, '').replace(/\.git$/, '')}</span>
              <span className="tile-accounts">
                {s.accounts.map((a) => (
                  <span key={a.id} className="pill" title={`${a.login} on ${a.forge}`}>@{a.login} · {a.canWrite ? 'can edit' : 'read only'}</span>
                ))}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}

function SiteView({ id, branch }: { id: string; branch?: string }) {
  const site = useLoad(() => api.site(id, branch), [id, branch])
  useCommands(() => {
    const d = site.data
    if (!d) return []
    return [
      ...(d.pages || []).filter((p) => p.source).map((p) => ({ id: `page:${p.source}`, title: `${p.title}  ${p.path}`, group: 'Open page', run: () => go({ view: 'page', id, source: p.source!, branch }) })),
      ...(d.drafts || []).map((b) => ({ id: `draft:${b.branch}`, title: b.branch, group: 'Switch to draft', run: () => go({ view: 'site', id, branch: b.branch }) })),
      ...(branch ? [{ id: 'draft:back', title: `Back to ${d.publishBranch}`, group: 'Switch to draft', run: () => go({ view: 'site', id }) }] : []),
    ]
  })
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
          {d.role && <p className="muted small role-line" title="From the platform's permissions">{{ reader: 'You can read this site.', writer: 'You can save drafts and open reviews; the platform decides who may publish.', maintainer: 'You maintain this site: drafts, reviews and publishing.' }[d.role]}</p>}
          {d.accounts.length > 1 && (
            <label className="working-as">
              Working as
              <select
                value={d.as}
                onChange={(e) => {
                  chooseAccount(id, e.target.value)
                  site.reload()
                }}
              >
                {d.accounts.map((a) => (
                  <option key={a.id} value={a.id}>@{a.login} · {a.forge}{a.canWrite ? '' : ' (read only)'}</option>
                ))}
              </select>
            </label>
          )}
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
          <Link to={{ view: 'forms', id, branch }} className="side-item">Forms</Link>
          <Link to={{ view: 'look', id, branch }} className="side-item">Look</Link>
          <Link to={{ view: 'files', id, branch }} className="side-item">Files</Link>
          <Link to={{ view: 'redirects', id, branch }} className="side-item">Redirects</Link>
          <Link to={{ view: 'history', id }} className="side-item">History</Link>
          <Link to={{ view: 'privacy', id, branch }} className="side-item">Privacy &amp; statistics</Link>
          <Link to={{ view: 'search', id, branch }} className="side-item">Search engines</Link>
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
            <li key={i} className={p.Level}><span className="pill">{p.Rule}</span> <span className="mono small">{p.File}</span> {p.What}{p.Fix && <span className="fix">{p.Fix}</span>}</li>
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

// Commands that are always there: going places, the editor itself.
function GlobalCommands({ session }: { session: SessionInfo }) {
  const route = useRoute()
  const id = 'id' in route ? route.id : undefined
  const branch = 'branch' in route ? route.branch : undefined
  const theme = (t: 'light' | 'dark' | 'auto') => () => {
    try {
      if (t === 'auto') localStorage.removeItem('hotdog-theme')
      else localStorage.setItem('hotdog-theme', t)
    } catch {
      /* still applied */
    }
    if (t === 'auto') delete document.documentElement.dataset.theme
    else document.documentElement.dataset.theme = t
  }
  useCommands(() => [
    { id: 'go-sites', title: 'All sites', group: 'Go to', run: () => go({ view: 'sites' }) },
    ...(id
      ? [
          { id: 'go-site', title: 'This site\'s pages', group: 'Go to', run: () => go({ view: 'site', id, branch }) },
          { id: 'go-files', title: 'Files', group: 'Go to', run: () => go({ view: 'files', id, branch }) },
          { id: 'go-pictures', title: 'Pictures', group: 'Go to', run: () => go({ view: 'media', id, branch }) },
          { id: 'go-look', title: 'Look', group: 'Go to', run: () => go({ view: 'look', id, branch }) },
          { id: 'go-history', title: 'History (undo a published change)', group: 'Go to', run: () => go({ view: 'history', id }) },
          { id: 'go-forms', title: 'Forms and submissions', group: 'Go to', run: () => go({ view: 'forms', id, branch }) },
          { id: 'go-redirects', title: 'Redirects', group: 'Go to', run: () => go({ view: 'redirects', id, branch }) },
          { id: 'go-search', title: 'Search engines (verification, IndexNow)', group: 'Go to', run: () => go({ view: 'search', id, branch }) },
          { id: 'go-privacy', title: 'Privacy and statistics (analytics)', group: 'Go to', run: () => go({ view: 'privacy', id, branch }) },
        ]
      : []),
    { id: 'console-open', title: 'Open the console', group: 'Editor', keys: 'Ctrl+`', run: showConsole },
    { id: 'theme-light', title: 'Light theme', group: 'Editor', run: theme('light') },
    { id: 'theme-dark', title: 'Dark theme', group: 'Editor', run: theme('dark') },
    { id: 'theme-auto', title: 'Automatic theme', group: 'Editor', run: theme('auto') },
    ...session.forges.map((f) => ({ id: `add-${f.host}`, title: `Add an account: ${label(f.kind)} · ${f.host}`, group: 'Accounts', run: () => { location.href = signInURL(f.host, location.pathname + location.search, session.accounts.some((a) => a.forge === f.host)) } })),
  ])
  return null
}
