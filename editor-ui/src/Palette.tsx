import { useEffect, useMemo, useRef, useState } from 'react'
import { allCommands, score, useShortcut, type Command } from './Commands'

// The command palette: Ctrl+K (⌘K on a Mac) from anywhere, type a few
// letters of what you want, Enter. Arrow keys move, Esc closes.

let openPalette: () => void = () => undefined
export const showPalette = () => openPalette()

export function Palette() {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const [sel, setSel] = useState(0)
  const [cmds, setCmds] = useState<Command[]>([])
  const dialog = useRef<HTMLDialogElement>(null)
  const list = useRef<HTMLUListElement>(null)

  const show = () => {
    setCmds(allCommands())
    setQ('')
    setSel(0)
    setOpen(true)
  }
  openPalette = show
  useShortcut('mod+k', () => (open ? setOpen(false) : show()))
  useEffect(() => {
    if (open) dialog.current?.showModal()
    else if (dialog.current?.open) dialog.current.close()
  }, [open])

  const shown = useMemo(() => {
    const scored = cmds
      .map((c) => ({ c, s: Math.max(score(q, c.title), score(q, `${c.group} ${c.title}`) - 50) }))
      .filter((x) => x.s >= 0)
    if (q.trim()) scored.sort((a, b) => b.s - a.s)
    return scored.slice(0, 60).map((x) => x.c)
  }, [cmds, q])

  useEffect(() => {
    list.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' })
  }, [sel])

  const run = (c: Command) => {
    setOpen(false)
    // After the dialog closes, so focus lands where the command puts it.
    setTimeout(c.run, 0)
  }

  return (
    <dialog ref={dialog} className="palette" aria-label="Command palette" onClose={() => setOpen(false)}>
      {open && (
        <>
          <input
            autoFocus
            className="palette-input"
            placeholder="Type a command or a page…"
            value={q}
            role="combobox"
            aria-expanded="true"
            aria-controls="palette-list"
            aria-activedescendant={shown[sel] ? `cmd-${sel}` : undefined}
            onChange={(e) => {
              setQ(e.target.value)
              setSel(0)
            }}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') {
                e.preventDefault()
                setSel((n) => Math.min(n + 1, shown.length - 1))
              } else if (e.key === 'ArrowUp') {
                e.preventDefault()
                setSel((n) => Math.max(n - 1, 0))
              } else if (e.key === 'Enter' && shown[sel]) {
                e.preventDefault()
                run(shown[sel])
              }
            }}
          />
          <ul id="palette-list" role="listbox" ref={list} className="palette-list">
            {shown.length === 0 && <li className="muted small palette-empty">Nothing matches.</li>}
            {shown.map((c, i) => (
              <li
                key={c.id}
                id={`cmd-${i}`}
                role="option"
                aria-selected={i === sel}
                className={i === sel ? 'on' : ''}
                onMouseMove={() => setSel(i)}
                onClick={() => run(c)}
              >
                <span className="muted small palette-group">{c.group}</span>
                <span className="palette-title">{c.title}</span>
                {c.keys && <kbd>{c.keys}</kbd>}
              </li>
            ))}
          </ul>
          <p className="palette-foot muted small">↑↓ to move · Enter to run · Esc to close · Ctrl+` opens the console</p>
        </>
      )}
    </dialog>
  )
}
