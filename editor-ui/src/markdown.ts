// Markdown in, Markdown out, for the visual editor. Three rules keep it from
// rewriting what nobody touched:
//
//  1. Every top-level block remembers the exact source it came from. A block
//     that hasn't been edited is written back as that source, character for
//     character, wrapping and all. Only edited or new blocks are written
//     afresh. (ProseMirror reuses unchanged nodes, so "unchanged" is just
//     "the same object".)
//  2. A block the editor can't represent exactly (a table, raw HTML,
//     footnotes, task lists, {#id} attributes, strikethrough, or anything
//     whose round trip would change it) becomes a read-only "raw" block: shown,
//     kept verbatim, edited in the Markdown view.
//  3. New Markdown is plain and conventional: - bullets, * emphasis, ** strong.

import MarkdownIt from 'markdown-it'
import { Schema, type Node } from 'prosemirror-model'
import { defaultMarkdownParser, defaultMarkdownSerializer, MarkdownParser, MarkdownSerializer, schema as baseSchema } from 'prosemirror-markdown'

export const schema = new Schema({
  nodes: baseSchema.spec.nodes.addToEnd('raw_block', {
    group: 'block',
    atom: true,
    selectable: true,
    draggable: false,
    attrs: { text: { default: '' }, why: { default: '' } },
    toDOM: (n) => ['pre', { class: 'raw-block', 'data-why': n.attrs.why, contenteditable: 'false' }, n.attrs.text],
    parseDOM: [],
  }),
  marks: baseSchema.spec.marks,
})

const commonmark = MarkdownIt('commonmark', { html: false })
const parser = new MarkdownParser(schema, commonmark, defaultMarkdownParser.tokens)

export const serializer = new MarkdownSerializer(
  {
    ...defaultMarkdownSerializer.nodes,
    bullet_list(state, node) {
      state.renderList(node, '  ', () => '- ')
    },
    raw_block(state, node) {
      state.write(node.attrs.text)
      state.closeBlock(node)
    },
  },
  defaultMarkdownSerializer.marks,
)

// The site renders GitHub-flavored Markdown; this reader sees what it sees,
// to find blocks the visual editor must leave alone.
const gfm = MarkdownIt('default', { html: true })

type Block = { start: number; end: number; type: string; inline: string[] }

// topBlocks finds each top-level block and its source lines.
function topBlocks(src: string): Block[] {
  const tokens = gfm.parse(src, {})
  const out: Block[] = []
  let cur: Block | undefined
  for (const t of tokens) {
    if (t.level === 0 && t.map && t.nesting >= 0) {
      cur = { start: t.map[0], end: t.map[1], type: t.type, inline: [] }
      out.push(cur)
    }
    if (cur && t.children) for (const c of t.children) cur.inline.push(c.type)
    if (cur && t.type !== cur.type && t.nesting >= 0) cur.inline.push(t.type)
  }
  return out
}

// Why a block can't be edited visually, or '' if it can.
function rawReason(b: Block, text: string): string {
  if (b.type === 'table_open' || b.inline.includes('table_open')) return 'a table'
  if (b.type === 'html_block' || b.inline.includes('html_inline')) return 'raw HTML'
  if (b.inline.includes('s_open')) return 'strikethrough'
  if (/^\s*\[\^[^\]]+\]:/m.test(text) || /\[\^[^\]]+\]/.test(text)) return 'a footnote'
  if (/^\s*[-*+] \[[ xX]\] /m.test(text)) return 'a task list'
  if (/\{[#.][^}]*\}\s*$/m.test(text) || /[)\]]\{\s*:?\s*[#.][^}]*\}/.test(text)) return 'attributes ({#id})'
  if (/^:\s{1,3}\S/m.test(text)) return 'a definition list'
  return ''
}

// The documentation syntax HotDog reads the way MkDocs writes it (an
// admonition, a collapsible block, content tabs: a first line, then the body
// indented four spaces). markdown-it doesn't know it and would read it as a
// paragraph and some indented code, so each one is kept whole, as one block.
type Span = { start: number; end: number; why: string }
const docsOpener = /^ {0,3}(?:(!!!|\?\?\?\+?)[ \t]+[A-Za-z][\w-]*|===[+!]{0,2}[ \t]+")/

function docsBlocks(lines: string[]): Span[] {
  const out: Span[] = []
  let fence = false
  for (let i = 0; i < lines.length; i++) {
    if (/^\s*(```|~~~)/.test(lines[i])) fence = !fence
    const m = !fence && docsOpener.exec(lines[i])
    if (!m) continue
    let end = i + 1
    for (let j = i + 1; j < lines.length; j++) {
      const l = lines[j]
      if (l.trim() === '') continue
      if (/^( {4}|\t)/.test(l)) { end = j + 1; continue }
      // Tabs that follow one another are one set.
      if (docsOpener.test(l) && l.trimStart().startsWith('===') && m[0].trimStart().startsWith('===')) { end = j + 1; continue }
      break
    }
    const why = m[1] === '!!!' ? 'an admonition' : m[1] ? 'a collapsible block' : 'content tabs'
    out.push({ start: i, end, why })
    i = end - 1
  }
  return out
}

export type Loaded = { doc: Node; origins: WeakMap<Node, string>; raw: string[] }

// load reads Markdown into a document, remembering each block's source.
export function load(markdown: string): Loaded {
  const src = markdown.replace(/\r\n/g, '\n')
  const lines = src.split('\n')
  const origins = new WeakMap<Node, string>()
  const nodes: Node[] = []
  const raw: string[] = []
  const keep = (text: string, why: string) => {
    const node = schema.nodes.raw_block.create({ text, why })
    origins.set(node, text)
    nodes.push(node)
    raw.push(why)
  }
  let prev = 0
  const spans = docsBlocks(lines)
  for (const b of topBlocks(src)) {
    if (b.end <= prev) continue // inside a block kept whole below
    const sp = spans.find((s) => s.start >= prev && s.start < b.end)
    if (sp) {
      const gap = lines.slice(prev, sp.start).join('\n')
      if (gap.trim()) keep(gap.replace(/^\n+|\n+$/g, ''), 'link definitions or other Markdown kept as it is')
      keep(lines.slice(sp.start, sp.end).join('\n').replace(/\n+$/, ''), sp.why)
      prev = sp.end
      continue
    }
    // Lines no block covers (link definitions, say) are kept as they are.
    const gap = lines.slice(prev, b.start).join('\n')
    if (gap.trim()) keep(gap.replace(/^\n+|\n+$/g, ''), 'link definitions or other Markdown kept as it is')
    prev = b.end
    const text = lines.slice(b.start, b.end).join('\n').replace(/\n+$/, '')
    let why = rawReason(b, text)
    let node: Node | undefined
    if (!why) {
      const doc = parser.parse(text)
      if (doc.childCount !== 1) why = 'a block the visual editor would split'
      else {
        node = doc.child(0)
        // Would writing it back change what it means? (Escaping a character
        // changes the text, not the meaning; that's fine.)
        const again = parser.parse(serializer.serialize(doc))
        if (again.childCount !== 1 || !again.child(0).eq(node)) why = 'Markdown the visual editor would change'
      }
    }
    if (why) {
      keep(text, why)
      continue
    }
    origins.set(node!, text)
    nodes.push(node!)
  }
  const tail = lines.slice(prev).join('\n')
  if (tail.trim()) keep(tail.replace(/^\n+|\n+$/g, ''), 'link definitions or other Markdown kept as it is')
  if (nodes.length === 0) nodes.push(schema.nodes.paragraph.create())
  return { doc: schema.nodes.doc.create(null, nodes), origins, raw }
}

// save writes a document back to Markdown: untouched blocks exactly as they
// were, the rest freshly.
export function save(doc: Node, origins: WeakMap<Node, string>): string {
  const parts: string[] = []
  doc.forEach((child) => {
    const orig = origins.get(child)
    if (orig !== undefined) parts.push(orig)
    else if (child.type.name === 'paragraph' && child.childCount === 0) return // an empty line left by the editor
    else parts.push(serializer.serialize(schema.nodes.doc.create(null, [child])).replace(/\n+$/, ''))
  })
  return parts.length ? parts.join('\n\n') + '\n' : ''
}
