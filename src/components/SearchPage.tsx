import { useDeferredValue, useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import type { Portal } from '../corpus/portal'
import type { IndexedDoc } from '../corpus/types'
import { docHref, replaceHash, searchHref } from '../router'
import { displayUrl, formatDateTime, plural, relativeTime } from '../util/format'
import { SearchIcon } from './Icons'
import { ResultItem } from './ResultItem'
import { SourceBadge } from './SourceBadge'

const PAGE_SIZE = 20

interface Props {
  portal: Portal
  initialQuery: string
  initialSources: string[]
}

export function SearchPage({ portal, initialQuery, initialSources }: Props) {
  const [query, setQuery] = useState(initialQuery)
  const [selected, setSelected] = useState<string[]>(() =>
    initialSources.filter((s) => portal.sources.some((m) => m.id === s)),
  )
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLOListElement>(null)
  const deferredQuery = useDeferredValue(query)
  const hasQuery = deferredQuery.trim() !== ''

  // Keep the URL in sync so Back from a document returns to these results.
  useEffect(() => {
    replaceHash(searchHref(query, selected))
  }, [query, selected])

  const terms = useMemo(() => portal.engine.queryTerms(deferredQuery), [portal, deferredQuery])

  const search = useMemo(() => {
    if (!hasQuery) return undefined
    const all = portal.engine.search(deferredQuery)
    const perSource = new Map<string, number>()
    for (const r of all) perSource.set(r.doc.source_id, (perSource.get(r.doc.source_id) ?? 0) + 1)
    const filter = new Set(selected)
    const results = filter.size ? all.filter((r) => filter.has(r.doc.source_id)) : all
    return { results, perSource }
  }, [portal, deferredQuery, hasQuery, selected])

  const browseDocs = useMemo<IndexedDoc[]>(() => {
    if (hasQuery || selected.length === 0) return []
    return portal.engine
      .browse(new Set(selected))
      .slice()
      .sort((a, b) => portal.shortTitle(a).localeCompare(portal.shortTitle(b), undefined, { numeric: true }))
  }, [portal, hasQuery, selected])

  // Paging and keyboard selection reset whenever the result set changes.
  const resultKey = deferredQuery + '\u0000' + selected.join(',')
  const [paging, setPaging] = useState({ key: resultKey, limit: PAGE_SIZE, active: 0 })
  const current = paging.key === resultKey ? paging : { key: resultKey, limit: PAGE_SIZE, active: 0 }
  const { limit, active } = current
  const setActive = (f: (a: number) => number) => setPaging({ ...current, active: f(current.active) })
  const showMore = () => setPaging({ ...current, limit: current.limit + PAGE_SIZE })

  const visible = hasQuery ? (search?.results.slice(0, limit).map((r) => r.doc) ?? []) : browseDocs.slice(0, limit)
  const totalCount = hasQuery ? (search?.results.length ?? 0) : browseDocs.length

  // "/" focuses the search box from anywhere on the page.
  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => {
      const target = e.target as HTMLElement | null
      const typing = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)
      if (e.key === '/' && !typing && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault()
        inputRef.current?.focus()
        inputRef.current?.select()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(`[data-index="${active}"]`)
    el?.scrollIntoView?.({ block: 'nearest' })
  }, [active])

  const toggleSource = (id: string) =>
    setSelected((cur) => (cur.includes(id) ? cur.filter((s) => s !== id) : [...cur, id]))

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((a) => Math.min(a + 1, Math.max(visible.length - 1, 0)))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => Math.max(a - 1, 0))
    } else if (e.key === 'Enter') {
      const doc = visible[active]
      if (doc) {
        const link = listRef.current?.querySelector<HTMLAnchorElement>(`[data-index="${active}"] a.result-link`)
        window.location.hash = link?.getAttribute('href') ?? docHref(doc.id, { q: query, sources: selected })
      }
    } else if (e.key === 'Escape') {
      if (query) setQuery('')
      else inputRef.current?.blur()
    }
  }

  const { manifest } = portal

  return (
    <div className="search-page">
      <section className={hasQuery || selected.length ? 'hero compact' : 'hero'}>
        {!(hasQuery || selected.length) && (
          <>
            <h1>Search your offline documentation</h1>
            <p className="hero-sub">
              {plural(manifest.document_count, 'document')} from {plural(manifest.sources.length, 'source')}, indexed{' '}
              <time dateTime={manifest.generated_at} title={formatDateTime(manifest.generated_at)}>
                {relativeTime(manifest.generated_at)}
              </time>
              . Everything below is served from this computer.
            </p>
          </>
        )}
        <div className="search-box">
          <SearchIcon className="search-icon" />
          <input
            ref={inputRef}
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Search titles, headings, prose and code…"
            aria-label="Search documentation"
            aria-controls="results"
            autoFocus
            spellCheck={false}
            autoComplete="off"
          />
          <kbd className="search-kbd" aria-hidden="true">
            /
          </kbd>
        </div>
        <p className="search-hints" aria-hidden="true">
          <kbd>↑</kbd> <kbd>↓</kbd> choose · <kbd>Enter</kbd> open · <kbd>Esc</kbd> clear
        </p>
      </section>

      <div className="search-layout">
        <aside className="source-filter" aria-label="Filter by source">
          <div className="filter-head">
            <h2>Sources</h2>
            {selected.length > 0 && (
              <button type="button" className="link-button" onClick={() => setSelected([])}>
                Clear
              </button>
            )}
          </div>
          <ul>
            {portal.sources.map((s) => {
              const count = hasQuery ? (search?.perSource.get(s.id) ?? 0) : s.document_count
              const checked = selected.includes(s.id)
              return (
                <li key={s.id}>
                  <label className={checked ? 'filter-item checked' : 'filter-item'}>
                    <input type="checkbox" checked={checked} onChange={() => toggleSource(s.id)} />
                    <span className="filter-name">{s.name}</span>
                    <span className="filter-count" title={hasQuery ? 'matching documents' : 'indexed documents'}>
                      {count.toLocaleString()}
                    </span>
                  </label>
                </li>
              )
            })}
          </ul>
        </aside>

        <section className="results" id="results" aria-live="polite">
          {hasQuery && search && (
            <p className="results-summary">
              <strong>{plural(search.results.length, 'result')}</strong> for “{deferredQuery.trim()}”
              {selected.length > 0 && <> in {plural(selected.length, 'source')}</>}
            </p>
          )}
          {!hasQuery && selected.length > 0 && (
            <p className="results-summary">
              Browsing <strong>{plural(browseDocs.length, 'document')}</strong> in{' '}
              {selected.map((id) => portal.sourceName(id)).join(', ')}
            </p>
          )}

          {!hasQuery && selected.length === 0 && <SourcesOverview portal={portal} onBrowse={(id) => setSelected([id])} />}

          {hasQuery && search && search.results.length === 0 && (
            <div className="empty-state">
              <h2>No matches for “{deferredQuery.trim()}”</h2>
              <p>Try fewer or more general words. Searches match word beginnings, so “config” also finds “configuration”.</p>
              {selected.length > 0 && (
                <button type="button" className="button" onClick={() => setSelected([])}>
                  Search all sources instead
                </button>
              )}
            </div>
          )}

          {visible.length > 0 && (
            <ol className="result-list" ref={listRef}>
              {visible.map((doc, i) => (
                <ResultItem
                  key={doc.id}
                  index={i}
                  doc={doc}
                  portal={portal}
                  terms={terms}
                  query={hasQuery ? deferredQuery : ''}
                  sources={selected}
                  active={i === active}
                  onHover={() => setActive(() => i)}
                />
              ))}
            </ol>
          )}
          {visible.length < totalCount && (
            <button type="button" className="button more-button" onClick={showMore}>
              Show more ({(totalCount - visible.length).toLocaleString()} remaining)
            </button>
          )}
        </section>
      </div>
    </div>
  )
}

function SourcesOverview({ portal, onBrowse }: { portal: Portal; onBrowse: (id: string) => void }) {
  return (
    <div className="sources-overview">
      <h2 className="section-title">Indexed sources</h2>
      <ul className="source-cards">
        {portal.sources.map((s) => (
          <li key={s.id} className="source-card">
            <div className="source-card-head">
              <SourceBadge id={s.id} name={s.name} />
              <span className={`status status-${s.status}`}>{s.status}</span>
            </div>
            <p className="source-count">{plural(s.document_count, 'document')}</p>
            <p className="muted small">
              Indexed{' '}
              <time dateTime={s.indexed_at} title={formatDateTime(s.indexed_at)}>
                {relativeTime(s.indexed_at)}
              </time>
              {s.errors > 0 && <> · {plural(s.errors, 'error')} during crawl</>}
            </p>
            {s.start_urls.map((u) => (
              <p key={u} className="source-url" title="Original location (not needed for browsing)">
                {displayUrl(u)}
              </p>
            ))}
            <button type="button" className="button" onClick={() => onBrowse(s.id)} disabled={s.document_count === 0}>
              Browse documents
            </button>
          </li>
        ))}
      </ul>
      <p className="muted small tip">
        Tip: press <kbd>/</kbd> to start searching. Use the source list to narrow results.
      </p>
    </div>
  )
}
