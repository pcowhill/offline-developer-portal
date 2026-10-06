import { useState } from 'react'
import type { Manifest } from '../corpus/types'
import { applyTheme, storedTheme, type ThemePreference } from '../theme'
import { formatDateTime, plural, relativeTime } from '../util/format'
import { LogoIcon, MoonIcon, SunIcon, SystemIcon } from './Icons'

const NEXT: Record<ThemePreference, ThemePreference> = { system: 'light', light: 'dark', dark: 'system' }
const LABEL: Record<ThemePreference, string> = { system: 'System theme', light: 'Light theme', dark: 'Dark theme' }

export function ThemeToggle() {
  const [pref, setPref] = useState<ThemePreference>(storedTheme)
  const next = NEXT[pref]
  return (
    <button
      type="button"
      className="icon-button"
      onClick={() => {
        applyTheme(next)
        setPref(next)
      }}
      title={`${LABEL[pref]} (click for ${LABEL[next].toLowerCase()})`}
      aria-label={`${LABEL[pref]}. Switch to ${LABEL[next].toLowerCase()}`}
    >
      {pref === 'light' ? <SunIcon /> : pref === 'dark' ? <MoonIcon /> : <SystemIcon />}
    </button>
  )
}

export function Header({ manifest }: { manifest?: Manifest }) {
  return (
    <header className="site-header">
      <div className="site-header-inner">
        <a className="brand" href="#/">
          <LogoIcon />
          <span className="brand-name">Offline Developer Portal</span>
        </a>
        <div className="header-right">
          {manifest && (
            <span className="corpus-status" title={`Corpus generated ${formatDateTime(manifest.generated_at)}`}>
              <span className="status-dot" aria-hidden="true" />
              {plural(manifest.document_count, 'document')} · updated {relativeTime(manifest.generated_at)}
            </span>
          )}
          <ThemeToggle />
        </div>
      </div>
    </header>
  )
}
