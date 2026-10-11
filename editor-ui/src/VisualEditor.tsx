import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react'
import { baseKeymap, setBlockType, toggleMark, wrapIn, lift } from 'prosemirror-commands'
import { history, redo, undo } from 'prosemirror-history'
import { inputRules, textblockTypeInputRule, wrappingInputRule } from 'prosemirror-inputrules'
import { keymap } from 'prosemirror-keymap'
import { liftListItem, sinkListItem, splitListItem, wrapInList } from 'prosemirror-schema-list'
import { EditorState, TextSelection, type Command } from 'prosemirror-state'
import { EditorView } from 'prosemirror-view'
import 'prosemirror-view/style/prosemirror.css'
import { load, save, schema, type Loaded } from './markdown'

// The visual editor: the page body as it reads, edited in place, written
// back as the Markdown a person would have written (markdown.ts). It offers
// only what Markdown can say, so nothing it produces is HTML.

export type VisualHandle = {
  insertImage: (src: string, alt: string) => string // returns the new Markdown
  jumpTo: (text: string) => boolean
  focus: () => void
}

const n = schema.nodes
const m = schema.marks

function plugins() {
  return [
    inputRules({
      rules: [
        textblockTypeInputRule(/^(#{1,4})\s$/, n.heading, (match) => ({ level: match[1].length })),
        wrappingInputRule(/^\s*>\s$/, n.blockquote),
        wrappingInputRule(/^\s*([-+*])\s$/, n.bullet_list),
        wrappingInputRule(/^(\d+)\.\s$/, n.ordered_list, (match) => ({ order: +match[1] }), (match, node) => node.childCount + node.attrs.order === +match[1]),
        textblockTypeInputRule(/^```$/, n.code_block),
      ],
    }),
    keymap({
      'Mod-z': undo,
      'Shift-Mod-z': redo,
      'Mod-y': redo,
      'Mod-b': toggleMark(m.strong),
      'Mod-i': toggleMark(m.em),
      'Mod-`': toggleMark(m.code),
      Enter: splitListItem(n.list_item),
      Tab: sinkListItem(n.list_item),
      'Shift-Tab': liftListItem(n.list_item),
      'Shift-Enter': (state, dispatch) => {
        dispatch?.(state.tr.replaceSelectionWith(n.hard_break.create()).scrollIntoView())
        return true
      },
    }),
    keymap(baseKeymap),
    history(),
  ]
}

function markActive(state: EditorState, type: typeof m.strong) {
  const { from, $from, to, empty } = state.selection
  return empty ? !!type.isInSet(state.storedMarks || $from.marks()) : state.doc.rangeHasMark(from, to, type)
}

function blockKind(state: EditorState): string {
  const p = state.selection.$from.parent
  if (p.type === n.heading) return `h${p.attrs.level}`
  if (p.type === n.code_block) return 'code'
  return 'p'
}

type Props = {
  value: string
  onChange: (md: string) => void
  onPicture: (file?: File) => void
  disabled?: boolean
}

export const VisualEditor = forwardRef<VisualHandle, Props>(function VisualEditor({ value, onChange, onPicture, disabled }, ref) {
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView>(undefined)
  const loaded = useRef<Loaded>(undefined)
  const emitted = useRef<string>(value)
  const [, setTick] = useState(0) // the toolbar follows the selection
  const [link, setLink] = useState<string>()
  const cb = useRef({ onChange, onPicture })
  cb.current = { onChange, onPicture }

  const fresh = (md: string) => {
    loaded.current = load(md)
    return EditorState.create({ doc: loaded.current.doc, plugins: plugins() })
  }

  useEffect(() => {
    const images = (files: FileList | null | undefined) => Array.from(files || []).find((f) => /^image\/(jpeg|png|gif|webp)$/.test(f.type))
    const v = new EditorView(host.current!, {
      state: fresh(value),
      editable: () => !disabled,
      attributes: { 'aria-label': 'Page body', 'aria-multiline': 'true', role: 'textbox', class: 'visual-body' },
      dispatchTransaction(tr) {
        const next = v.state.apply(tr)
        v.updateState(next)
        setTick((t) => t + 1)
        if (tr.docChanged && loaded.current) {
          const md = save(next.doc, loaded.current.origins)
          emitted.current = md
          cb.current.onChange(md)
        }
      },
      // A picture dropped or pasted goes through Insert picture, so it gets
      // re-encoded and alt text like any other.
      handleDrop(_view, e) {
        const f = images((e as DragEvent).dataTransfer?.files)
        if (!f) return false
        e.preventDefault()
        cb.current.onPicture(f)
        return true
      },
      handlePaste(_view, e) {
        const f = images(e.clipboardData?.files)
        if (!f) return false
        cb.current.onPicture(f)
        return true
      },
    })
    view.current = v
    return () => v.destroy()
  }, [])

  // A change from outside (saved, reloaded, edited in the Markdown view)
  // starts the visual editor afresh from that text.
  useEffect(() => {
    if (value !== emitted.current && view.current) {
      emitted.current = value
      view.current.updateState(fresh(value))
      setTick((t) => t + 1)
    }
  }, [value])

  useEffect(() => {
    view.current?.setProps({ editable: () => !disabled })
  }, [disabled])

  useImperativeHandle(ref, () => ({
    insertImage(src, alt) {
      const v = view.current!
      const img = n.image.create({ src, alt: alt || null })
      const para = n.paragraph.create(null, img)
      const { $from } = v.state.selection
      const at = $from.depth > 0 ? $from.after(1) : v.state.doc.content.size
      v.dispatch(v.state.tr.insert(at, para).scrollIntoView())
      return emitted.current
    },
    jumpTo(text) {
      const v = view.current
      if (!v) return false
      const want = text.replace(/\s+/g, ' ').trim().toLowerCase()
      let found = -1
      v.state.doc.descendants((node, pos) => {
        if (found >= 0) return false
        if (node.isTextblock) {
          const t = node.textContent.replace(/\s+/g, ' ').trim().toLowerCase()
          if (t && (t === want || want.startsWith(t) || t.startsWith(want.slice(0, 40)))) found = pos + 1
        }
        return true
      })
      if (found < 0) return false
      v.dispatch(v.state.tr.setSelection(TextSelection.create(v.state.doc, found)).scrollIntoView())
      v.focus()
      return true
    },
    focus() {
      view.current?.focus()
    },
  }))

  const run = (c: Command) => () => {
    const v = view.current
    if (!v) return
    c(v.state, v.dispatch, v)
    v.focus()
  }
  const st = view.current?.state
  const kind = st ? blockKind(st) : 'p'
  const can = (c: Command) => !!st && !disabled && c(st)
  const active = (t: typeof m.strong) => !!st && markActive(st, t)

  const applyLink = (href: string | undefined) => {
    const v = view.current
    if (!v) return
    const { from, to, empty } = v.state.selection
    let tr = v.state.tr
    if (empty) {
      // Inside a link: the whole link.
      const $p = v.state.selection.$from
      const linkMark = m.link.isInSet($p.marks())
      if (!linkMark) return setLink(undefined)
      let start = $p.pos, end = $p.pos
      while (start > $p.start() && m.link.isInSet(v.state.doc.resolve(start - 1).marks())) start--
      while (end < $p.end() && m.link.isInSet(v.state.doc.resolve(end + 1).marks())) end++
      tr = tr.removeMark(start, end, m.link)
      if (href) tr = tr.addMark(start, end, m.link.create({ href }))
    } else {
      tr = tr.removeMark(from, to, m.link)
      if (href) tr = tr.addMark(from, to, m.link.create({ href }))
    }
    v.dispatch(tr)
    setLink(undefined)
    v.focus()
  }

  return (
    <div className={`visual ${disabled ? 'readonly' : ''}`}>
      <div className="visual-bar" role="toolbar" aria-label="Formatting">
        <select
          aria-label="Block type"
          value={kind}
          disabled={disabled}
          onChange={(e) => {
            const k = e.target.value
            run(k === 'p' ? setBlockType(n.paragraph) : k === 'code' ? setBlockType(n.code_block) : setBlockType(n.heading, { level: +k.slice(1) }))()
          }}
        >
          <option value="p">Paragraph</option>
          <option value="h2">Heading</option>
          <option value="h3">Subheading</option>
          <option value="h4">Small heading</option>
          <option value="code">Code block</option>
          {kind === 'h1' && <option value="h1">Title heading</option>}
        </select>
        <button type="button" aria-pressed={active(m.strong)} disabled={!can(toggleMark(m.strong))} onClick={run(toggleMark(m.strong))} title="Bold (Ctrl+B)"><b>B</b></button>
        <button type="button" aria-pressed={active(m.em)} disabled={!can(toggleMark(m.em))} onClick={run(toggleMark(m.em))} title="Italic (Ctrl+I)"><i>I</i></button>
        <button type="button" aria-pressed={active(m.code)} disabled={!can(toggleMark(m.code))} onClick={run(toggleMark(m.code))} title="Code (Ctrl+`)"><code>{'<>'}</code></button>
        <button type="button" aria-pressed={active(m.link)} disabled={disabled} onClick={() => setLink(link === undefined ? '' : undefined)} title="Link">Link</button>
        <span className="bar-gap" />
        <button type="button" disabled={!can(wrapInList(n.bullet_list))} onClick={run(wrapInList(n.bullet_list))} title="Bulleted list">• List</button>
        <button type="button" disabled={!can(wrapInList(n.ordered_list))} onClick={run(wrapInList(n.ordered_list))} title="Numbered list">1. List</button>
        <button type="button" disabled={!can(wrapIn(n.blockquote))} onClick={run(wrapIn(n.blockquote))} title="Quote">Quote</button>
        <button type="button" disabled={!can(lift)} onClick={run(lift)} title="Out of the list or quote">Outdent</button>
        <button type="button" disabled={disabled} onClick={run((s, d) => { d?.(s.tr.replaceSelectionWith(n.horizontal_rule.create()).scrollIntoView()); return true })} title="A line across">Rule</button>
        <button type="button" disabled={disabled} onClick={() => cb.current.onPicture()} title="Insert picture">Picture</button>
        <span className="bar-gap" />
        <button type="button" disabled={!can(undo)} onClick={run(undo)} title="Undo (Ctrl+Z)" aria-label="Undo">↶</button>
        <button type="button" disabled={!can(redo)} onClick={run(redo)} title="Redo (Ctrl+Shift+Z)" aria-label="Redo">↷</button>
      </div>
      {link !== undefined && (
        <form className="visual-link" onSubmit={(e) => { e.preventDefault(); applyLink(link.trim() || undefined) }}>
          <input autoFocus value={link} placeholder="/about/ or https://…" onChange={(e) => setLink(e.target.value)} aria-label="Link address" />
          <button type="submit" className="ghost small">Apply</button>
          <button type="button" className="ghost small" onClick={() => applyLink(undefined)}>Remove link</button>
          <button type="button" className="ghost small" onClick={() => setLink(undefined)}>Cancel</button>
        </form>
      )}
      {loaded.current && loaded.current.raw.length > 0 && (
        <p className="muted small visual-note">Parts of this page ({[...new Set(loaded.current.raw)].join(', ')}) are kept exactly as written; change them in the Markdown view.</p>
      )}
      <div ref={host} className="visual-host" />
    </div>
  )
})
