import { useEffect, useState } from 'react'
import { api, CheckFailed, type PageDetail, type Problem, type QuickInfo, type QuickResult } from './api'
import { go } from './route'
import { fromFrontMatter, SectionsEditor, toFrontMatter, type Sec } from './Sections'

// Quick posts: what an operator needs up in minutes. A banner or alert
// across the top of the site, a news post, an event, the front page. One
// form, one button: "Post now" makes the commit, opens the review, runs the
// checks and publishes, when the platform lets this person publish. If it
// doesn't, the review waits for someone who can.

type Kind = 'banner' | 'post' | 'event' | 'home'

const KINDS: { kind: Kind; label: string; about: string }[] = [
  { kind: 'banner', label: 'Banner or alert', about: 'A line across the top of the site: hours, a closure, an offer.' },
  { kind: 'post', label: 'News post', about: 'A short announcement, dated today.' },
  { kind: 'event', label: 'Event', about: 'Something happening on a date, added to the calendar.' },
  { kind: 'home', label: 'Front page', about: 'Change the words and buttons on the home page.' },
]

// Dates in the site's time zone, as plain strings: "2026-11-07 18:00".
function addDays(date: string, n: number): string {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + n)
  return d.toISOString().slice(0, 10)
}

export function QuickPost({ id, kind: initial }: { id: string; kind?: string }) {
  const [info, setInfo] = useState<QuickInfo>()
  const [error, setError] = useState<string>()
  const kinds = KINDS.filter((k) => k.kind !== 'event' || (info?.calendars.length ?? 0) > 0)
  const kind = (kinds.find((k) => k.kind === initial)?.kind || 'banner') as Kind
  const [result, setResult] = useState<QuickResult>()
  const [problems, setProblems] = useState<Problem[]>()
  const [busy, setBusy] = useState(false)

  const load = () => api.quickInfo(id).then(setInfo, (e: Error) => setError(e.message))
  useEffect(() => {
    load()
  }, [id])

  const send = async (body: Record<string, unknown>, publish: boolean): Promise<boolean> => {
    setBusy(true)
    setResult(undefined)
    setProblems(undefined)
    try {
      const r = await api.quick(id, { ...body, publish })
      setResult(r)
      setProblems(r.problems || undefined)
      load()
      return true
    } catch (e) {
      if (e instanceof CheckFailed) setProblems(e.problems)
      setError((e as Error).message)
      return false
    } finally {
      setBusy(false)
    }
  }
  const props = { info: info!, busy, send: (b: Record<string, unknown>, p: boolean) => (setError(undefined), send(b, p)) }

  return (
    <section className="pad quick">
      <a className="back" href={`/sites/${encodeURIComponent(id)}`} onClick={(e) => { e.preventDefault(); go({ view: 'site', id }) }}>← Site</a>
      <h1>Quick post</h1>
      <div className="quick-kinds" role="tablist" aria-label="What to post">
        {kinds.map((k) => (
          <button key={k.kind} type="button" role="tab" aria-selected={k.kind === kind} className={`quick-kind ${k.kind === kind ? 'on' : ''}`} onClick={() => { setResult(undefined); setError(undefined); go({ view: 'quick', id, kind: k.kind }) }}>
            <strong>{k.label}</strong>
            <span className="muted small">{k.about}</span>
          </button>
        ))}
      </div>
      {error && <p className="notice bad" role="alert">{error}</p>}
      {result && (
        <div className={`notice ${result.published ? 'good' : problems?.length ? 'bad' : 'info'}`} role="status">
          <p>{result.message}</p>
          <p className="small">
            {result.review && <a href={result.review.url} target="_blank" rel="noopener noreferrer">Review #{result.review.number} ↗</a>}
            {result.preview && <> · <a href={result.preview} target="_blank" rel="noopener noreferrer">Preview ↗</a></>}
          </p>
        </div>
      )}
      {problems && problems.length > 0 && (
        <ul className="problems">{problems.filter((p) => p.Level === 'error').map((p, i) => <li key={i} className="error"><span className="pill">{p.Rule}</span> <span className="mono small">{p.File}</span> {p.What}{p.Fix && <span className="fix">{p.Fix}</span>}</li>)}</ul>
      )}
      {!info && !error && <p className="muted">Reading the site…</p>}
      {info && kind === 'banner' && <BannerForm {...props} />}
      {info && kind === 'post' && <PostForm {...props} />}
      {info && kind === 'event' && <EventForm {...props} />}
      {info && kind === 'home' && <HomeForm id={id} {...props} />}
    </section>
  )
}

type FormProps = { info: QuickInfo; busy: boolean; send: (body: Record<string, unknown>, publish: boolean) => Promise<boolean> }

function Buttons({ busy, ready, onSend }: { busy: boolean; ready: boolean; onSend: (publish: boolean) => void }) {
  return (
    <div className="actions-row">
      <button type="button" className="button primary" disabled={busy || !ready} onClick={() => onSend(true)}>{busy ? 'Posting…' : 'Post now'}</button>
      <button type="button" className="button" disabled={busy || !ready} onClick={() => onSend(false)}>Send for review</button>
      <span className="muted small">Post now runs the checks and publishes, if the platform lets you. Otherwise it waits in review.</span>
    </div>
  )
}

function BannerForm({ info, busy, send }: FormProps) {
  const [text, setText] = useState('')
  const [link, setLink] = useState('')
  const [linkText, setLinkText] = useState('')
  const [tone, setTone] = useState('info')
  const [show, setShow] = useState('all')
  const [when, setWhen] = useState('now')
  const [from, setFrom] = useState('')
  const [until, setUntil] = useState('today')
  const [untilAt, setUntilAt] = useState('')
  const today = info.now.slice(0, 10)
  const untilValue = { none: '', today: `${addDays(today, 1)} 00:00`, tomorrow: `${addDays(today, 2)} 00:00`, week: `${addDays(today, 7)} ${info.now.slice(11)}`, pick: untilAt.replace('T', ' ') }[until] ?? ''
  const fromValue = when === 'later' ? from.replace('T', ' ') : ''
  const post = async (publish: boolean) => {
    if (await send({ kind: 'banner', text, link, linkText, tone, show, from: fromValue, until: untilValue }, publish)) {
      setText('')
      setLink('')
      setLinkText('')
    }
  }
  return (
    <>
      <div className="form quick-form">
        <label>
          What it says
          <input value={text} maxLength={280} onChange={(e) => setText(e.target.value)} placeholder="Closed Monday for the holiday." />
        </label>
        <div className="row2">
          <label>Link (optional)<input value={link} onChange={(e) => setLink(e.target.value)} placeholder="/news/holiday-hours/ or https://…" /></label>
          <label>Link text<input value={linkText} disabled={!link} onChange={(e) => setLinkText(e.target.value)} placeholder="More" /></label>
        </div>
        <div className="row2">
          <label>
            Kind
            <select value={tone} onChange={(e) => setTone(e.target.value)}>
              <option value="info">Announcement</option>
              <option value="alert">Alert (stands out)</option>
            </select>
          </label>
          <label>
            Where
            <select value={show} onChange={(e) => setShow(e.target.value)}>
              <option value="all">Every page</option>
              <option value="home">The home page only</option>
            </select>
          </label>
        </div>
        <div className="row2">
          <label>
            Starts
            <select value={when} onChange={(e) => setWhen(e.target.value)}>
              <option value="now">As soon as it's published</option>
              <option value="later">At a date and time…</option>
            </select>
            {when === 'later' && <input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} aria-label="Starts at" />}
          </label>
          <label>
            Comes down
            <select value={until} onChange={(e) => setUntil(e.target.value)}>
              <option value="today">At the end of today</option>
              <option value="tomorrow">At the end of tomorrow</option>
              <option value="week">In a week</option>
              <option value="pick">At a date and time…</option>
              <option value="none">When I take it down</option>
            </select>
            {until === 'pick' && <input type="datetime-local" value={untilAt} onChange={(e) => setUntilAt(e.target.value)} aria-label="Comes down at" />}
          </label>
        </div>
        <p className="muted small">Times are in the site's time zone, {info.timezone}. A scheduled banner appears and comes down when the site is next rebuilt after that time.</p>
        {text.trim() && (
          <div className={`banner-preview ${tone === 'alert' ? 'alert' : ''}`} aria-label="Preview">
            {tone === 'alert' ? <strong>{text}</strong> : text}
            {link && <> <u>{linkText || 'More'}</u></>}
          </div>
        )}
      </div>
      <Buttons busy={busy} ready={!!text.trim() && (when !== 'later' || !!from) && (until !== 'pick' || !!untilAt)} onSend={post} />
      <h2>On the site</h2>
      {info.banners.length === 0 && <p className="muted">No banners.</p>}
      <ul className="quick-banners">
        {info.banners.map((b, i) => (
          <li key={i}>
            <span className={`pill ${b.showing ? 'good' : b.expired ? '' : 'warn'}`}>{b.showing ? 'showing' : b.expired ? 'over' : 'scheduled'}</span>
            {b.tone === 'alert' && <span className="pill">alert</span>}
            <span>{b.text}</span>
            <span className="muted small">{[b.show === 'home' ? 'home page' : '', b.from ? `from ${b.from}` : '', b.until ? `until ${b.until}` : ''].filter(Boolean).join(' · ')}</span>
            <button type="button" className="ghost small" disabled={busy} onClick={() => send({ kind: 'banner-remove', index: i, text: b.text }, true)}>Take down</button>
          </li>
        ))}
      </ul>
    </>
  )
}

function PostForm({ info, busy, send }: FormProps) {
  const [collection, setCollection] = useState(info.collections.find((c) => c.name === 'news')?.name || info.collections[0]?.name || '')
  const [title, setTitle] = useState('')
  const [summary, setSummary] = useState('')
  const [body, setBody] = useState('')
  const post = async (publish: boolean) => {
    if (await send({ kind: 'post', collection, title, summary, body }, publish)) {
      setTitle('')
      setSummary('')
      setBody('')
    }
  }
  if (info.collections.length === 0) return <p className="muted">This site has no collection for posts yet (a folder in content/, like news/).</p>
  return (
    <>
      <div className="form quick-form">
        <div className="row2">
          <label>Title<input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="New winter hours" /></label>
          <label>
            Goes in
            <select value={collection} onChange={(e) => setCollection(e.target.value)}>
              {info.collections.map((c) => <option key={c.name} value={c.name}>{c.title}</option>)}
            </select>
          </label>
        </div>
        <label>Summary (for lists and link previews)<input value={summary} onChange={(e) => setSummary(e.target.value)} /></label>
        <label>Text (Markdown)<textarea rows={8} value={body} onChange={(e) => setBody(e.target.value)} /></label>
      </div>
      <Buttons busy={busy} ready={!!title.trim()} onSend={post} />
    </>
  )
}

function EventForm({ info, busy, send }: FormProps) {
  const [collection, setCollection] = useState(info.calendars[0]?.name || '')
  const [title, setTitle] = useState('')
  const [date, setDate] = useState(info.now.slice(0, 10))
  const [allDay, setAllDay] = useState(false)
  const [start, setStart] = useState('18:00')
  const [end, setEnd] = useState('')
  const [endDate, setEndDate] = useState('')
  const [location, setLocation] = useState('')
  const [link, setLink] = useState('')
  const [body, setBody] = useState('')
  const post = async (publish: boolean) => {
    const b = { kind: 'event', collection, title, date, start: allDay ? '' : start, end: allDay ? '' : end, endDate: allDay ? endDate : '', location, eventLink: link, body }
    if (await send(b, publish)) {
      setTitle('')
      setBody('')
    }
  }
  return (
    <>
      <div className="form quick-form">
        <div className="row2">
          <label>What<input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Spring open house" /></label>
          {info.calendars.length > 1 ? (
            <label>Calendar<select value={collection} onChange={(e) => setCollection(e.target.value)}>{info.calendars.map((c) => <option key={c.name} value={c.name}>{c.title}</option>)}</select></label>
          ) : <span />}
        </div>
        <div className="row2">
          <label>Date<input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></label>
          <label className="check"><input type="checkbox" checked={allDay} onChange={(e) => setAllDay(e.target.checked)} /> All day</label>
        </div>
        {allDay ? (
          <label>Last day, if it runs over several (optional)<input type="date" value={endDate} min={date} onChange={(e) => setEndDate(e.target.value)} /></label>
        ) : (
          <div className="row2">
            <label>Starts<input type="time" value={start} onChange={(e) => setStart(e.target.value)} /></label>
            <label>Ends (optional)<input type="time" value={end} onChange={(e) => setEnd(e.target.value)} /></label>
          </div>
        )}
        <div className="row2">
          <label>Where (optional)<input value={location} onChange={(e) => setLocation(e.target.value)} placeholder="The Grange Hall, 12 Main St" /></label>
          <label>Tickets or sign-up link (optional)<input value={link} onChange={(e) => setLink(e.target.value)} placeholder="https://…" /></label>
        </div>
        <label>About it (Markdown, optional)<textarea rows={6} value={body} onChange={(e) => setBody(e.target.value)} /></label>
        <p className="muted small">Times are in the site's time zone, {info.timezone}. The event appears in the calendar feed, so subscribers see it in their calendar app.</p>
      </div>
      <Buttons busy={busy} ready={!!title.trim() && !!date && (allDay || !!start)} onSend={post} />
    </>
  )
}

function HomeForm({ id, busy, send }: FormProps & { id: string }) {
  const [page, setPage] = useState<PageDetail>()
  const [secs, setSecs] = useState<Sec[]>()
  const [body, setBody] = useState('')
  const [err, setErr] = useState<string>()
  const load = () =>
    api.page(id, 'content/index.md').then(
      (p) => {
        setPage(p)
        setSecs(fromFrontMatter(p.frontMatter.sections))
        setBody(p.body)
      },
      (e: Error) => setErr(e.message),
    )
  useEffect(() => {
    load()
  }, [id])
  if (err) return <p className="notice bad">{err}</p>
  if (!page) return <p className="muted">Reading the home page…</p>
  const dirty = (secs && JSON.stringify(toFrontMatter(secs)) !== JSON.stringify(page.frontMatter.sections)) || body !== page.body
  const post = async (publish: boolean) => {
    const edit = secs ? { set: { sections: toFrontMatter(secs) }, body } : { body }
    if (await send({ kind: 'home', edit, baseCommit: page.commit }, publish)) load()
  }
  return (
    <>
      <div className="form quick-form">
        {secs ? <SectionsEditor id={id} branch={page.branch} value={secs} onChange={setSecs} /> : <label>The home page (Markdown)<textarea rows={14} value={body} onChange={(e) => setBody(e.target.value)} /></label>}
      </div>
      <Buttons busy={busy} ready={!!dirty} onSend={post} />
    </>
  )
}
