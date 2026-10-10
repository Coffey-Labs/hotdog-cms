import { useState } from 'react'

// Light, dark, or whatever the device prefers; remembered per browser.
// public/theme.js applies the stored choice before the first paint.

type Theme = 'system' | 'light' | 'dark'
const KEY = 'hotdog-theme'

function stored(): Theme {
  try {
    const v = localStorage.getItem(KEY)
    if (v === 'light' || v === 'dark') return v
  } catch {
    /* storage blocked: follow the device */
  }
  return 'system'
}

export function ThemePicker() {
  const [theme, setTheme] = useState<Theme>(stored)
  const pick = (t: Theme) => {
    setTheme(t)
    if (t === 'system') delete document.documentElement.dataset.theme
    else document.documentElement.dataset.theme = t
    try {
      if (t === 'system') localStorage.removeItem(KEY)
      else localStorage.setItem(KEY, t)
    } catch {
      /* not remembered, still applied */
    }
  }
  const label: Record<Theme, string> = { system: 'Auto', light: 'Light', dark: 'Dark' }
  return (
    <div className="seg theme-pick" role="group" aria-label="Color theme">
      {(Object.keys(label) as Theme[]).map((t) => (
        <button key={t} type="button" aria-pressed={theme === t} title={t === 'system' ? 'Follow the device setting' : `${label[t]} theme`} onClick={() => pick(t)}>
          {label[t]}
        </button>
      ))}
    </div>
  )
}
