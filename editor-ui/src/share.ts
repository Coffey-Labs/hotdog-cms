// Share links: a published page posted to social networks by opening each
// network's own share page with the text filled in. The person posts it from
// their own account, signed in there as usual; the editor holds no tokens and
// posts nothing itself.

export function shareText(title: string, summary: string, url: string): string {
  return [title.trim(), summary.trim(), url].filter(Boolean).join('\n\n')
}

// mastodonHost takes whatever someone types for their server, "linuxexpert.org",
// "https://linuxexpert.org/", or their handle "@me@linuxexpert.org", and
// returns the host, or '' if it isn't one.
export function mastodonHost(input: string): string {
  let h = input.trim().toLowerCase().replace(/^https?:\/\//, '').replace(/\/.*$/, '')
  if (h.includes('@')) h = h.slice(h.lastIndexOf('@') + 1)
  return /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}(?::\d{1,5})?$/.test(h) ? h : ''
}

export function mastodonURL(host: string, text: string): string {
  return `https://${host}/share?text=${encodeURIComponent(text)}`
}

export function blueskyURL(text: string): string {
  return `https://bsky.app/intent/compose?text=${encodeURIComponent(text)}`
}

// LinkedIn's share page takes only the link; it reads the title, summary and
// picture from the page's own tags.
export function linkedinURL(url: string): string {
  return `https://www.linkedin.com/sharing/share-offsite/?url=${encodeURIComponent(url)}`
}

// Bluesky counts characters as people see them, and allows 300.
export const BLUESKY_MAX = 300

export function graphemes(s: string): number {
  if (typeof Intl !== 'undefined' && 'Segmenter' in Intl) {
    let n = 0
    for (const _ of new Intl.Segmenter(undefined, { granularity: 'grapheme' }).segment(s)) n++
    return n
  }
  return [...s].length
}
