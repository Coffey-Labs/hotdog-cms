// Share links (node --test test/).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { blueskyURL, graphemes, linkedinURL, mastodonHost, mastodonURL, shareText } from '../src/share.ts'

test('the post text is title, summary and link', () => {
  assert.equal(shareText(' Spring opening ', '', 'https://example.org/news/spring/'), 'Spring opening\n\nhttps://example.org/news/spring/')
  assert.equal(shareText('A', 'B', 'https://x.org/'), 'A\n\nB\n\nhttps://x.org/')
})

test('a Mastodon server can be typed any usual way', () => {
  for (const s of ['linuxexpert.org', 'https://linuxexpert.org/', '@john@linuxexpert.org', 'LinuxExpert.org/@john']) {
    assert.equal(mastodonHost(s), 'linuxexpert.org', s)
  }
  for (const s of ['', 'not a host', 'javascript:alert(1)', 'evil.org"><script>', 'localhost']) {
    assert.equal(mastodonHost(s), '', s)
  }
})

test('share links carry the text escaped', () => {
  const text = 'Tea & cake?\n\nhttps://x.org/a?b=1'
  assert.equal(mastodonURL('m.org', text), 'https://m.org/share?text=Tea%20%26%20cake%3F%0A%0Ahttps%3A%2F%2Fx.org%2Fa%3Fb%3D1')
  assert.ok(blueskyURL(text).startsWith('https://bsky.app/intent/compose?text=Tea%20%26'))
  assert.equal(linkedinURL('https://x.org/a?b=1'), 'https://www.linkedin.com/sharing/share-offsite/?url=https%3A%2F%2Fx.org%2Fa%3Fb%3D1')
})

test('Bluesky length counts what people see', () => {
  assert.equal(graphemes('café 👩‍👩‍👧'), 6)
})
