// Talking to the editor server. Reads are plain GETs; anything that changes
// state sends the X-HotDog header, which the server requires along with a
// matching Origin, so another site can't make the browser act for you.

export type Forge = { host: string; kind: string }
export type User = { login: string; name: string; forge: string }
export type SessionInfo = { signedIn: boolean; user?: User; forges: Forge[] }
export type SiteSummary = { id: string; name: string; repo: string; branch: string; canWrite: boolean }
export type PageInfo = { path: string; title: string; source?: string; collection?: string; kind: string; date?: string; draft?: boolean; redirect?: string }
export type CollectionInfo = { name: string; title: string; count: number }
export type Problem = { Level: 'error' | 'warning'; Rule: string; File: string; What: string }
export type BuildResult = { commit: string; pages: number; errors: number; warnings: number; problems: Problem[] | null; failed?: string }
export type Draft = { branch: string; commit: string; preview?: string }
export type Review = { number: number; title: string; branch: string; author: string; url: string; preview?: string }
export type SiteDetail = {
  id: string; name: string; repo: string; branch: string; publishBranch: string; commit: string; canWrite: boolean
  siteName: string; siteURL: string; collections: CollectionInfo[] | null; pages: PageInfo[] | null
  drafts: Draft[] | null; reviews: Review[] | null; build: BuildResult; preview?: string; notYet?: string
}
export type PageDetail = { source: string; frontMatter: Record<string, unknown>; frontMatterRaw: string; body: string; branch: string; commit: string }
export type Edit = { set?: Record<string, unknown>; frontMatter?: string; body: string }
export type MediaItem = { url: string; width: number; height: number; thumb: string; bytes: number; files: number; usedBy: string[]; alt: string }
export type Uploaded = { branch: string; commit: string; created: boolean; url: string; width: number; height: number; notes: string[] | null; files: string[] }

export class SignedOut extends Error {}
export class Conflict extends Error {}
export class CheckFailed extends Error {
  problems: Problem[]
  constructor(msg: string, problems: Problem[]) {
    super(msg)
    this.problems = problems
  }
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path, { credentials: 'same-origin', headers: { Accept: 'application/json' } })
  if (res.status === 401) throw new SignedOut('Sign in to continue.')
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error((data as { error?: string }).error || `The editor answered ${res.status}.`)
  return data as T
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json', 'X-HotDog': '1' },
    body: JSON.stringify(body),
  })
  if (res.status === 401) throw new SignedOut('Your sign-in has ended. Sign in again.')
  const data = await res.json().catch(() => ({}))
  if (res.status === 409 && (data as { problems?: Problem[] }).problems) throw new CheckFailed((data as { error: string }).error, (data as { problems: Problem[] }).problems)
  if (res.status === 409) throw new Conflict((data as { error?: string }).error || 'This changed since you opened it.')
  if (!res.ok) throw new Error((data as { error?: string }).error || `The editor answered ${res.status}.`)
  return data as T
}

// Pictures go up as a multipart form; the same headers guard it.
async function upload(id: string, file: File, fields: Record<string, string>): Promise<Uploaded> {
  const form = new FormData()
  for (const [k, v] of Object.entries(fields)) form.set(k, v)
  form.set('file', file)
  const res = await fetch(site(id, 'media'), { method: 'POST', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-HotDog': '1' }, body: form })
  if (res.status === 401) throw new SignedOut('Your sign-in has ended. Sign in again.')
  const data = await res.json().catch(() => ({}))
  if (res.status === 409) throw new Conflict((data as { error?: string }).error || 'This changed since you opened it.')
  if (!res.ok) throw new Error((data as { error?: string }).error || `The editor answered ${res.status}.`)
  return data as Uploaded
}

const site = (id: string, action: string) => `/api/sites/${encodeURIComponent(id)}/${action}`

export const api = {
  save: (id: string, b: { source: string; branch: string; baseCommit: string; edit: Edit; message?: string }) =>
    post<{ branch: string; commit: string; created: boolean; preview?: string }>(site(id, 'save'), b),
  newPage: (id: string, b: { collection: string; title: string }) => post<{ branch: string; commit: string; source: string }>(site(id, 'new'), b),
  live: (id: string, b: { source: string; branch: string; edit: Edit | null }) => post<{ url?: string; path?: string; error?: string }>(site(id, 'live'), b),
  review: (id: string, b: { branch: string; title: string; description: string }) => post<Review>(site(id, 'review'), b),
  publish: (id: string, b: { branch: string }) => post<{ published: boolean; number: number }>(site(id, 'publish'), b),
  upload: (id: string, file: File, b: { branch: string; baseCommit: string; source: string }) => upload(id, file, b),
  media: (id: string, branch?: string) => get<MediaItem[]>(`${site(id, 'media')}${branch ? `?branch=${encodeURIComponent(branch)}` : ''}`),
  mediaFile: (id: string, url: string, branch?: string) => `${site(id, 'media-file')}?url=${encodeURIComponent(url)}${branch ? `&branch=${encodeURIComponent(branch)}` : ''}`,
  session: () => get<SessionInfo>('/api/session'),
  sites: () => get<SiteSummary[]>('/api/sites'),
  site: (id: string, branch?: string) => get<SiteDetail>(`/api/sites/${encodeURIComponent(id)}${branch ? `?branch=${encodeURIComponent(branch)}` : ''}`),
  page: (id: string, source: string, branch?: string) =>
    get<PageDetail>(`/api/sites/${encodeURIComponent(id)}/page?source=${encodeURIComponent(source)}${branch ? `&branch=${encodeURIComponent(branch)}` : ''}`),
  logout: () => fetch('/auth/logout', { method: 'POST', credentials: 'same-origin', headers: { 'X-HotDog': '1' } }),
}

export function signInURL(forge: string, returnTo: string) {
  return `/auth/login?forge=${encodeURIComponent(forge)}&return=${encodeURIComponent(returnTo)}`
}
