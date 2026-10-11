import { useEffect, useRef, useState } from 'react'
import { api, chooseAccount, type SiteDetail } from './api'
import { useCommands, useShortcut } from './Commands'
import { go, useRoute } from './route'

// The console: a command line in the browser that speaks HotDog CMS. It
// runs nothing on any server: every command is a call the rest of the
// editor makes too (and most have a twin in `hotdog-cms` on a laptop), so
// it's a faster way in, never a different one. Ctrl+` opens and closes it.

type Line = { kind: 'in' | 'out' | 'err' | 'hint'; text: string }

let toggleConsole: () => void = () => undefined
export const showConsole = () => toggleConsole()

const HELP = `Commands (the site and branch are the ones on screen):
  sites                     the sites you can open
  open <site>               open a site
  status                    site, branch, account, checks
  pages [collection]        list pages
  edit <file>               open a page or any file (content/about.md, site.yaml)
  files [text]              list files, optionally only those containing text
  cat <file>                print a text file
  new <collection> "Title"  start a draft with a new page (- for top level)
  check                     the branch's check results       laptop: hotdog-cms check
  drafts                    open drafts and reviews
  branch [name]             switch to a draft, or back to the published branch
  diff                      what publishing this draft would change
  log [file]                history of the branch, or of one file
  review "Title" ["Notes"]  open a review for this draft
  publish                   merge this draft's review, if it passes the checks
  pictures | look | files   open those views
  as <login>                work on this site as another of your accounts
  theme light|dark|auto     the editor's colors
  clear                     clear the console
It never runs shell commands: the editor host is not a shell.`

// words splits a command line, keeping "quoted text" together.
function words(s: string): string[] {
  const out: string[] = []
  const re = /"([^"]*)"|'([^']*)'|(\S+)/g
  let m
  while ((m = re.exec(s))) out.push(m[1] ?? m[2] ?? m[3])
  return out
}

export function Console() {
  const route = useRoute()
  const [open, setOpen] = useState(false)
  const [lines, setLines] = useState<Line[]>([{ kind: 'hint', text: 'Type help for the commands. Esc returns to the page.' }])
  const [input, setInput] = useState('')
  const [hist, setHist] = useState<string[]>([])
  const [hi, setHi] = useState(-1)
  const [busy, setBusy] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const field = useRef<HTMLInputElement>(null)

  toggleConsole = () => setOpen((o) => !o)
  useShortcut('mod+`', () => setOpen((o) => !o))
  useCommands(() => [{ id: 'console', title: open ? 'Close the console' : 'Open the console', group: 'Editor', keys: 'Ctrl+`', run: () => setOpen(!open) }])
  useEffect(() => {
    if (open) field.current?.focus()
  }, [open])
  useEffect(() => {
    box.current?.scrollTo(0, box.current.scrollHeight)
  }, [lines])

  const id = 'id' in route ? route.id : undefined
  const branch = 'branch' in route ? route.branch : undefined
  const say = (text: string, kind: Line['kind'] = 'out') => setLines((l) => [...l, { kind, text }])
  const needSite = () => {
    if (!id) throw new Error('Open a site first: sites, then open <site>.')
    return id
  }
  const siteNow = async (): Promise<SiteDetail> => api.site(needSite(), branch)

  const run = async (raw: string) => {
    const [cmd, ...args] = words(raw)
    if (!cmd) return
    switch (cmd) {
      case 'help':
        return say(HELP)
      case 'clear':
        return setLines([])
      case 'sites': {
        const s = await api.sites()
        return say(s.length ? s.map((x) => `${x.id.padEnd(28)} ${x.name}${x.canWrite ? '' : ' (read only)'}`).join('\n') : 'No sites open to your accounts.')
      }
      case 'open':
        if (!args[0]) throw new Error('open <site>')
        return go({ view: 'site', id: args[0] })
      case 'status': {
        const d = await siteNow()
        return say(`${d.name} (${d.id})\nbranch   ${d.branch}${d.branch === d.publishBranch ? ' (published)' : ' (draft)'} at ${d.commit.slice(0, 10)}\nas       ${d.as}${d.canWrite ? '' : ' (read only)'}\nchecks   ${d.build.failed ? `doesn't build: ${d.build.failed}` : `${d.build.errors} error(s), ${d.build.warnings} warning(s), ${d.build.pages} pages`}`)
      }
      case 'pages': {
        const d = await siteNow()
        const ps = (d.pages || []).filter((p) => p.source && (!args[0] || p.collection === args[0] || (args[0] === '-' && !p.collection)))
        return say(ps.map((p) => `${(p.source || '').padEnd(40)} ${p.title}`).join('\n') || 'No pages.')
      }
      case 'edit': {
        const f = args[0]
        if (!f) throw new Error('edit <file>')
        const site = needSite()
        if (/^content\/.+\.md$/.test(f)) return go({ view: 'page', id: site, source: f, branch })
        return go({ view: 'files', id: site, branch, path: f })
      }
      case 'files': {
        const r = await api.files(needSite(), branch)
        const fs = r.files.filter((x) => !args[0] || x.path.includes(args[0]))
        return say(fs.map((x) => `${x.path}${x.text ? '' : '  (not text)'}`).join('\n') || 'No files match.')
      }
      case 'cat': {
        if (!args[0]) throw new Error('cat <file>')
        const f = await api.file(needSite(), args[0], branch)
        return say(f.exists ? f.content : `No ${args[0]} on this branch.`)
      }
      case 'new': {
        if (args.length < 2) throw new Error('new <collection> "Title"   (- for a top-level page)')
        const r = await api.newPage(needSite(), { collection: args[0] === '-' ? '' : args[0], title: args.slice(1).join(' ') })
        say(`Started ${r.source} on ${r.branch}.`)
        say('laptop: create the file under content/, then commit it to a branch', 'hint')
        return go({ view: 'page', id: needSite(), source: r.source, branch: r.branch })
      }
      case 'check': {
        const d = await siteNow()
        if (d.build.failed) return say(`This branch doesn't build: ${d.build.failed}`, 'err')
        const ps = d.build.problems || []
        say(ps.length ? ps.map((p) => `${p.Level.padEnd(8)} ${p.Rule.padEnd(24)} ${p.File}  ${p.What}`).join('\n') : 'Passes every check.')
        return say(`${d.build.errors} error(s), ${d.build.warnings} warning(s).${d.build.errors ? ' Errors block publishing.' : ''}`, d.build.errors ? 'err' : 'hint')
      }
      case 'drafts': {
        const d = await siteNow()
        const ds = (d.drafts || []).map((b) => `${b.branch}${(d.reviews || []).some((r) => r.branch === b.branch) ? '  (in review)' : ''}`)
        return say(ds.join('\n') || 'No drafts.')
      }
      case 'branch': {
        const site = needSite()
        if (!args[0]) {
          const d = await siteNow()
          return say(`${d.branch}${d.branch === d.publishBranch ? ' (published)' : ' (draft)'}`)
        }
        const keep = route.view === 'page' || route.view === 'files' || route.view === 'look' || route.view === 'media' ? route.view : 'site'
        const b = args[0]
        if (keep === 'page' && route.view === 'page') return go({ view: 'page', id: site, source: route.source, branch: b })
        if (keep === 'files' && route.view === 'files') return go({ view: 'files', id: site, branch: b, path: route.path })
        if (keep === 'look') return go({ view: 'look', id: site, branch: b })
        if (keep === 'media') return go({ view: 'media', id: site, branch: b })
        return go({ view: 'site', id: site, branch: b })
      }
      case 'diff': {
        if (!branch) throw new Error('This is the published branch; switch to a draft first (branch <name>).')
        const r = await api.compare(needSite(), branch)
        return say(r.diff ? `${r.stat}\n\n${r.diff}` : 'Nothing differs from the published branch.')
      }
      case 'log': {
        const r = await api.log(needSite(), branch, args[0], 20)
        const when = (s: string) => new Date(s).toLocaleString(undefined, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', timeZoneName: 'short' })
        return say(r.commits.map((c) => `${c.sha.slice(0, 8)}  ${when(c.when).padEnd(24)} ${c.author.padEnd(18)} ${c.subject}`).join('\n') || 'No history.')
      }
      case 'review': {
        if (!branch) throw new Error('Reviews are for drafts; switch to one first.')
        if (!args[0]) throw new Error('review "Title" ["What changed"]')
        const r = await api.review(needSite(), { branch, title: args[0], description: args[1] || '' })
        return say(`Opened review #${r.number}: ${r.url}`)
      }
      case 'publish': {
        if (!branch) throw new Error('Publishing merges a draft; switch to one first.')
        await api.publish(needSite(), { branch })
        say('Published: merged. The live site updates when its publishing pipeline runs.')
        return go({ view: 'site', id: needSite() })
      }
      case 'pictures':
        return go({ view: 'media', id: needSite(), branch })
      case 'look':
        return go({ view: 'look', id: needSite(), branch })
      case 'as': {
        const d = await siteNow()
        const a = d.accounts.find((x) => x.login === args[0] || x.id === args[0])
        if (!a) throw new Error(`Not signed in as ${args[0] || '?'} with access to this site. Accounts here: ${d.accounts.map((x) => x.login).join(', ')}`)
        chooseAccount(d.id, a.id)
        say(`Working on ${d.name} as @${a.login}.`)
        return go({ view: 'site', id: d.id, branch })
      }
      case 'theme': {
        const t = args[0]
        if (t !== 'light' && t !== 'dark' && t !== 'auto') throw new Error('theme light|dark|auto')
        try {
          if (t === 'auto') localStorage.removeItem('hotdog-theme')
          else localStorage.setItem('hotdog-theme', t)
        } catch {
          /* still applied */
        }
        if (t === 'auto') delete document.documentElement.dataset.theme
        else document.documentElement.dataset.theme = t
        return say(`Theme: ${t}.`)
      }
      default:
        throw new Error(`No command ${cmd}. Type help.`)
    }
  }

  const submit = async () => {
    const raw = input.trim()
    if (!raw || busy) return
    setInput('')
    setHi(-1)
    setHist((h) => [raw, ...h.filter((x) => x !== raw)].slice(0, 50))
    say(`${id ? `${id}${branch ? `@${branch}` : ''}` : '~'} $ ${raw}`, 'in')
    setBusy(true)
    try {
      await run(raw)
    } catch (e) {
      say((e as Error).message, 'err')
    } finally {
      setBusy(false)
    }
  }

  if (!open) return null
  return (
    <section className="console" aria-label="Console">
      <div className="console-head">
        <strong>Console</strong>
        <span className="muted small">{id ? `${id}${branch ? ` @ ${branch}` : ''}` : 'no site open'} · help for commands</span>
        <button type="button" className="ghost small" onClick={() => setOpen(false)} aria-label="Close the console">Close</button>
      </div>
      <div className="console-out" ref={box} role="log" aria-live="polite">
        {lines.map((l, i) => <pre key={i} className={`console-line ${l.kind}`}>{l.text}</pre>)}
        {busy && <pre className="console-line hint">…</pre>}
      </div>
      <form className="console-in" onSubmit={(e) => { e.preventDefault(); submit() }}>
        <span aria-hidden="true">$</span>
        <input
          ref={field}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'ArrowUp') {
              e.preventDefault()
              const n = Math.min(hi + 1, hist.length - 1)
              if (n >= 0) { setHi(n); setInput(hist[n]) }
            } else if (e.key === 'ArrowDown') {
              e.preventDefault()
              const n = hi - 1
              setHi(Math.max(n, -1))
              setInput(n >= 0 ? hist[n] : '')
            } else if (e.key === 'Escape') {
              setOpen(false)
            }
          }}
          spellCheck={false}
          autoComplete="off"
          aria-label="Command"
        />
      </form>
    </section>
  )
}
