import { useEffect, useState } from 'react'

// A small router: the address bar is the state, so every view can be
// bookmarked and the Back button works.
export type Route =
  | { view: 'sites' }
  | { view: 'site'; id: string; branch?: string }
  | { view: 'page'; id: string; source: string; branch?: string }
  | { view: 'present'; id: string; url: string; branch?: string }
  | { view: 'media'; id: string; branch?: string }
  | { view: 'look'; id: string; branch?: string }
  | { view: 'redirects'; id: string; branch?: string }
  | { view: 'privacy'; id: string; branch?: string }
  | { view: 'search'; id: string; branch?: string }
  | { view: 'files'; id: string; branch?: string; path?: string }

export function parse(loc: Location): Route {
  const parts = loc.pathname.split('/').filter(Boolean).map(decodeURIComponent)
  const q = new URLSearchParams(loc.search)
  const branch = q.get('branch') || undefined
  if (parts[0] === 'sites' && parts[1]) {
    if (parts[2] === 'page' && q.get('source')) return { view: 'page', id: parts[1], source: q.get('source')!, branch }
    if (parts[2] === 'media') return { view: 'media', id: parts[1], branch }
    if (parts[2] === 'look') return { view: 'look', id: parts[1], branch }
    if (parts[2] === 'redirects') return { view: 'redirects', id: parts[1], branch }
    if (parts[2] === 'privacy') return { view: 'privacy', id: parts[1], branch }
    if (parts[2] === 'search') return { view: 'search', id: parts[1], branch }
    if (parts[2] === 'files') return { view: 'files', id: parts[1], branch, path: q.get('path') || undefined }
    if (parts[2] === 'present' && q.get('url')) return { view: 'present', id: parts[1], url: q.get('url')!, branch }
    return { view: 'site', id: parts[1], branch }
  }
  return { view: 'sites' }
}

export function href(r: Route): string {
  const q = new URLSearchParams()
  if ('branch' in r && r.branch) q.set('branch', r.branch)
  switch (r.view) {
    case 'sites':
      return '/'
    case 'site':
      return `/sites/${encodeURIComponent(r.id)}${q.size ? `?${q}` : ''}`
    case 'page':
      q.set('source', r.source)
      return `/sites/${encodeURIComponent(r.id)}/page?${q}`
    case 'files':
      if (r.path) q.set('path', r.path)
      return `/sites/${encodeURIComponent(r.id)}/files${q.size ? `?${q}` : ''}`
    case 'search':
      return `/sites/${encodeURIComponent(r.id)}/search${q.size ? `?${q}` : ''}`
    case 'privacy':
      return `/sites/${encodeURIComponent(r.id)}/privacy${q.size ? `?${q}` : ''}`
    case 'redirects':
      return `/sites/${encodeURIComponent(r.id)}/redirects${q.size ? `?${q}` : ''}`
    case 'look':
      return `/sites/${encodeURIComponent(r.id)}/look${q.size ? `?${q}` : ''}`
    case 'media':
      return `/sites/${encodeURIComponent(r.id)}/media${q.size ? `?${q}` : ''}`
    case 'present':
      q.set('url', r.url)
      return `/sites/${encodeURIComponent(r.id)}/present?${q}`
  }
}

export function go(r: Route) {
  history.pushState(null, '', href(r))
  dispatchEvent(new PopStateEvent('popstate'))
}

export function useRoute(): Route {
  const [route, setRoute] = useState(() => parse(location))
  useEffect(() => {
    const on = () => setRoute(parse(location))
    addEventListener('popstate', on)
    return () => removeEventListener('popstate', on)
  }, [])
  return route
}
