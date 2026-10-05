import { useEffect, useState } from 'react'

// Hash-based routing so the portal works from any static file server
// without rewrite rules:
//   #/?q=query&s=source1,source2          search / browse
//   #/doc/<id>?q=query&s=...&a=anchor      document view

export interface SearchRoute {
  name: 'search'
  q: string
  sources: string[]
}

export interface DocRoute {
  name: 'doc'
  id: string
  q: string
  sources: string[]
  anchor?: string
}

export type Route = SearchRoute | DocRoute

export function parseHash(hash: string): Route {
  const raw = hash.replace(/^#/, '')
  const qIndex = raw.indexOf('?')
  const path = qIndex >= 0 ? raw.slice(0, qIndex) : raw
  const params = new URLSearchParams(qIndex >= 0 ? raw.slice(qIndex + 1) : '')
  const q = params.get('q') ?? ''
  const sources = (params.get('s') ?? '').split(',').filter(Boolean)
  const m = /^\/doc\/([^/?]+)$/.exec(path)
  if (m) {
    return { name: 'doc', id: decodeURIComponent(m[1]), q, sources, anchor: params.get('a') ?? undefined }
  }
  return { name: 'search', q, sources }
}

function query(params: Record<string, string | undefined>): string {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v) p.set(k, v)
  const s = p.toString()
  return s ? '?' + s : ''
}

export function searchHref(q: string, sources: readonly string[]): string {
  return '#/' + query({ q: q.trim() ? q : undefined, s: sources.join(',') || undefined })
}

export function docHref(id: string, opts: { q?: string; sources?: readonly string[]; anchor?: string } = {}): string {
  return (
    '#/doc/' +
    encodeURIComponent(id) +
    query({ q: opts.q?.trim() ? opts.q : undefined, s: opts.sources?.join(',') || undefined, a: opts.anchor })
  )
}

/**
 * Current route plus a counter that increases on every navigation (hash
 * change). The counter lets pages reset their local state when the user
 * navigates, while URL updates made with replaceHash() do not.
 */
export function useRoute(): [Route, number] {
  const [state, setState] = useState(() => ({ route: parseHash(window.location.hash), seq: 0 }))
  useEffect(() => {
    const onChange = () => setState((s) => ({ route: parseHash(window.location.hash), seq: s.seq + 1 }))
    window.addEventListener('hashchange', onChange)
    return () => window.removeEventListener('hashchange', onChange)
  }, [])
  return [state.route, state.seq]
}

/** Updates the hash without adding a history entry or triggering navigation. */
export function replaceHash(href: string): void {
  if (window.location.hash !== href) {
    window.history.replaceState(window.history.state, '', href)
  }
}
