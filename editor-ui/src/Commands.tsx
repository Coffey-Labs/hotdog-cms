import { useEffect, useRef } from 'react'

// Every action the editor offers, as commands: the palette (Ctrl+K) lists
// them, and views add their own while they're on screen, so the list is
// always what can be done right here. Keyboard first, mouse optional.

export type Command = {
  id: string
  title: string
  group: string // Go to, Page, Site, Editor …
  keys?: string // shown beside it, e.g. "Ctrl+S"
  run: () => void
}

const providers = new Map<symbol, { current: () => Command[] }>()

// useCommands adds commands while the calling component is mounted. The
// function is called when the palette opens, so it always sees fresh state.
export function useCommands(fn: () => Command[]) {
  const ref = useRef(fn)
  ref.current = fn
  useEffect(() => {
    const key = Symbol()
    providers.set(key, ref)
    return () => {
      providers.delete(key)
    }
  }, [])
}

export function allCommands(): Command[] {
  const seen = new Set<string>()
  const out: Command[] = []
  for (const p of providers.values()) {
    for (const c of p.current()) {
      if (!seen.has(c.id)) {
        seen.add(c.id)
        out.push(c)
      }
    }
  }
  return out
}

// score is a simple fuzzy match: every typed letter in order, closer and
// earlier is better. -1 means no match.
export function score(query: string, text: string): number {
  const q = query.toLowerCase().trim()
  if (!q) return 0
  const t = text.toLowerCase()
  const at = t.indexOf(q)
  if (at >= 0) return 1000 - at
  let i = 0
  let gaps = 0
  let last = -1
  for (const ch of q) {
    if (ch === ' ') continue
    const j = t.indexOf(ch, i)
    if (j < 0) return -1
    if (last >= 0) gaps += j - last - 1
    last = j
    i = j + 1
  }
  return 500 - gaps
}

// useShortcut runs fn on a key combination: "mod+s" is Ctrl+S, or ⌘S on a Mac.
export function useShortcut(combo: string, fn: (e: KeyboardEvent) => void, enabled = true) {
  const ref = useRef(fn)
  ref.current = fn
  useEffect(() => {
    if (!enabled) return
    const [mod, key] = combo.includes('+') ? combo.split('+') : ['', combo]
    const on = (e: KeyboardEvent) => {
      const modOK = mod === 'mod' ? e.ctrlKey || e.metaKey : !e.ctrlKey && !e.metaKey
      if (modOK && e.key.toLowerCase() === key.toLowerCase()) {
        e.preventDefault()
        ref.current(e)
      }
    }
    window.addEventListener('keydown', on)
    return () => window.removeEventListener('keydown', on)
  }, [combo, enabled])
}
