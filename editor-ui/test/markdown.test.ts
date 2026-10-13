// Round trips for the visual editor's Markdown (node --test test/).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { load, save, schema } from '../src/markdown.ts'

const body = (f: string) => readFileSync(new URL(f, import.meta.url), 'utf8').replace(/^---\n[\s\S]*?\n---\n\n?/, '')

test('untouched pages come back exactly as written', () => {
  for (const f of ['../../starter/site/content/about.md', '../../starter/site/content/articles/hello-world.md', '../../starter/site/content/contact.md']) {
    const src = body(f)
    const { doc, origins } = load(src)
    assert.equal(save(doc, origins), src.replace(/\n*$/, '\n'), f)
  }
})

test('what the editor cannot represent is kept, read-only', () => {
  const src = [
    'Intro with *em*, **strong**, `code` and a [link][ref].',
    '| a | b |\n|---|---|\n| 1 | 2 |',
    'This is ~~gone~~.',
    '<div class="x">raw</div>',
    '- [ ] todo\n- [x] done',
    '## Heading {#custom}',
    'Claim.[^1]',
    '[^1]: Source.',
    '[ref]: /about/',
  ].join('\n\n') + '\n'
  const { doc, origins, raw } = load(src)
  // The footnote's definition reads as a link definition, so it's kept with
  // [ref]: in one block of lines no other block covers.
  assert.deepEqual(raw, ['a table', 'strikethrough', 'raw HTML', 'a task list', 'attributes ({#id})', 'a footnote', 'link definitions or other Markdown kept as it is'])
  assert.equal(doc.child(0).type.name, 'paragraph')
  assert.equal(save(doc, origins), src)
})

test('an edited block is written afresh, the rest untouched', () => {
  const src = 'First paragraph,\nwrapped by hand.\n\n* a star list\n* kept as it is\n\nLast one,\nalso wrapped.\n'
  const { doc, origins } = load(src)
  // Replace the last paragraph and add a new list.
  const p = schema.nodes.paragraph.create(null, schema.text('Rewritten.'))
  const list = schema.nodes.bullet_list.create(null, [
    schema.nodes.list_item.create(null, schema.nodes.paragraph.create(null, schema.text('new'))),
  ])
  const edited = doc.copy(doc.content.replaceChild(2, p).addToEnd(list))
  assert.equal(save(edited, origins), 'First paragraph,\nwrapped by hand.\n\n* a star list\n* kept as it is\n\nRewritten.\n\n- new\n')
})

test('escaping is not a reason to lock a paragraph', () => {
  const { raw } = load('Use site_name and 2*3 in text.\n')
  assert.deepEqual(raw, [])
})

test('documentation syntax is kept whole, read-only', () => {
  const src = [
    'Intro paragraph.',
    '!!! note "Title"\n    Body with **bold**.\n\n    ```sh\n    a command\n    ```',
    '???+ tip\n    Open to start.',
    '=== "Linux"\n    One.\n\n=== "macOS"\n    Two.',
    'Press ++ctrl+k++ to search.',
    '![A screenshot](shot.png){ .shot }',
    'Term\n:   Its definition.',
    'Last paragraph.',
  ].join('\n\n') + '\n'
  const { doc, origins, raw } = load(src)
  assert.deepEqual(raw, ['an admonition', 'a collapsible block', 'content tabs', 'attributes ({#id})', 'a definition list'])
  assert.equal(doc.child(0).type.name, 'paragraph')
  assert.equal(save(doc, origins), src)
})
