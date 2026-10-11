import { useEffect, useState } from 'react'
import { api, type About } from './api'

// The bar along the bottom of every screen: who makes HotDog CMS, the
// version, and where its source is. The editor is under the AGPL, so anyone
// using it over a network can get the source of the version they're using;
// the link is here, on every screen, signed in or not. The operator's own
// links (privacy, terms, help) come from editor.yaml.

let cached: Promise<About> | undefined

export function Footer() {
  const [about, setAbout] = useState<About>()
  useEffect(() => {
    cached ??= api.about()
    cached.then(setAbout, () => (cached = undefined))
  }, [])
  const host = (u: string) => u.replace(/^https?:\/\//, '').replace(/\/$/, '')
  const version = about ? `${about.version}${about.revision ? ` · ${about.revision.slice(0, 7)}` : ''}${about.modified ? ' (modified)' : ''}` : ''
  return (
    <footer className="bottom">
      <span>
        {about?.copyright || '© 2026 Coffey Labs LLC'} ·{' '}
        <a href={about?.companyURL || 'https://coffeylabs.org'} target="_blank" rel="noopener noreferrer">{host(about?.companyURL || 'https://coffeylabs.org')}</a>
      </span>
      {about && (
        <span className="bottom-links">
          <span title={about.revision ? `Built from ${about.revision}` : undefined}>{about.name} {version}</span>
          <a href={about.source} target="_blank" rel="noopener noreferrer">Source code</a>
          <a href={about.licenseURL} target="_blank" rel="noopener noreferrer" title="GNU Affero General Public License, version 3 or later">{about.license}</a>
          {about.links?.map((l) => <a key={l.url} href={l.url} target="_blank" rel="noopener noreferrer">{l.name}</a>)}
        </span>
      )}
    </footer>
  )
}
