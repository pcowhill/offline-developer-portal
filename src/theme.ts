export type ThemePreference = 'system' | 'light' | 'dark'

const KEY = 'offline-developer-portal:theme'

export function storedTheme(): ThemePreference {
  try {
    const v = window.localStorage.getItem(KEY)
    if (v === 'light' || v === 'dark') return v
  } catch {
    // Storage may be unavailable; fall back to the system preference.
  }
  return 'system'
}

export function applyTheme(pref: ThemePreference): void {
  const root = document.documentElement
  if (pref === 'system') root.removeAttribute('data-theme')
  else root.setAttribute('data-theme', pref)
  try {
    if (pref === 'system') window.localStorage.removeItem(KEY)
    else window.localStorage.setItem(KEY, pref)
  } catch {
    // Ignore storage failures.
  }
}
