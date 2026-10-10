import { useEffect, useRef } from 'react'
import { api, signInURL, type SessionInfo } from './api'

// The accounts this browser is signed in to, on one platform or several.
// Each can be signed out on its own; another can be added from any platform
// the editor knows.

export function label(kind: string) {
  return ({ gitea: 'Gitea', forgejo: 'Forgejo', github: 'GitHub', gitlab: 'GitLab', bitbucket: 'Bitbucket' } as Record<string, string>)[kind] || kind
}

export function AccountsMenu({ session, onChanged }: { session: SessionInfo; onChanged: () => void }) {
  const menu = useRef<HTMLDetailsElement>(null)
  const ret = location.pathname + location.search
  const accounts = session.accounts
  const first = accounts[0]

  // Close when focus or a click goes elsewhere, or on Escape.
  useEffect(() => {
    const away = (e: Event) => {
      if (menu.current?.open && !menu.current.contains(e.target as Node)) menu.current.open = false
    }
    const esc = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && menu.current?.open) {
        menu.current.open = false
        menu.current.querySelector('summary')?.focus()
      }
    }
    document.addEventListener('click', away)
    document.addEventListener('focusin', away)
    document.addEventListener('keydown', esc)
    return () => {
      document.removeEventListener('click', away)
      document.removeEventListener('focusin', away)
      document.removeEventListener('keydown', esc)
    }
  }, [])

  const signOut = (id?: string) => api.logout(id).then(onChanged)

  return (
    <details className="accounts" ref={menu}>
      <summary>
        {accounts.length === 1 ? (
          <>{first.name || first.login} <span className="muted">@{first.login}</span></>
        ) : (
          <>{accounts.length} accounts</>
        )}
      </summary>
      <div className="accounts-panel" role="group" aria-label="Accounts">
        <ul>
          {accounts.map((a) => (
            <li key={a.id}>
              <span>
                <strong>{a.name || a.login}</strong>
                <span className="muted small"> @{a.login} · {a.forge}</span>
              </span>
              <button type="button" className="ghost small" onClick={() => signOut(a.id)}>Sign out</button>
            </li>
          ))}
        </ul>
        <h3 className="side-label">Add an account</h3>
        {session.forges.map((f) => {
          const already = accounts.some((a) => a.forge.toLowerCase() === f.host.toLowerCase())
          return (
            <div key={f.host} className="add-account">
              <a className="button small" href={signInURL(f.host, ret, already)}>
                {label(f.kind)} <span className="muted-on">· {f.host}</span>
              </a>
              {already && f.kind !== 'github' && (
                <p className="muted small">
                  {f.host} signs in whoever is signed in there. To add a different account, switch accounts on {f.host} first.
                </p>
              )}
            </div>
          )
        })}
        {accounts.length > 1 && (
          <button type="button" className="ghost small signout-all" onClick={() => signOut()}>Sign out of all</button>
        )}
      </div>
    </details>
  )
}
