import { useEffect, useState } from 'react'

// A small router: the address bar is the state, so every view can be
// bookmarked and the Back button works.
export type Route =
  | { view: 'sites' }
  | { view: 'site'; id: string; branch?: string }
  | { view: 'page'; id: string; source: string; branch?: string }

export function parse(loc: Location): Route {
  const parts = loc.pathname.split('/').filter(Boolean).map(decodeURIComponent)
  const q = new URLSearchParams(loc.search)
  const branch = q.get('branch') || undefined
  if (parts[0] === 'sites' && parts[1]) {
    if (parts[2] === 'page' && q.get('source')) return { view: 'page', id: parts[1], source: q.get('source')!, branch }
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
