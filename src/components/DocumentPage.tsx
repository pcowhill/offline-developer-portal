import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { CorpusMissingError, loadDocument } from '../corpus/load'
import type { Portal } from '../corpus/portal'
import type { Block, DocumentData, Heading } from '../corpus/types'
import { docHref, replaceHash, searchHref } from '../router'
import { formatDateTime } from '../util/format'
import { Blocks, headingDomId, type RenderContext } from './Blocks'
import { ExternalIcon } from './Icons'
import { SourceBadge } from './SourceBadge'

interface Props {
  portal: Portal
  id: string
  query: string
  sources: string[]
  anchor?: string
}

type State = { id: string; doc?: DocumentData; error?: string }

export function DocumentPage({ portal, id, query, sources, anchor }: Props) {
  const [state, setState] = useState<State>({ id })

  useEffect(() => {
    let cancelled = false
    loadDocument(id)
      .then((doc) => !cancelled && setState({ id, doc }))
      .catch((e: unknown) => {
        if (cancelled) return
        const message =
          e instanceof CorpusMissingError
            ? 'This document is not in the current corpus. It may have been removed when the sources were re-indexed.'
            : e instanceof Error
              ? e.message
              : String(e)
        setState({ id, error: message })
      })
    return () => {
      cancelled = true
    }
  }, [id])

  const doc = state.id === id ? state.doc : undefined
  const error = state.id === id ? state.error : undefined
  const terms = useMemo(() => portal.engine.queryTerms(query), [portal, query])

  // Scroll to the requested section (or the top) once the document is shown.
  useEffect(() => {
    if (!doc) return
    const el = anchor ? document.getElementById(headingDomId(anchor)) : null
    if (el) el.scrollIntoView?.({ block: 'start' })
    else window.scrollTo?.(0, 0)
  }, [doc, anchor])

  useEffect(() => {
    if (doc) document.title = `${portal.shortTitle(doc)} · Offline Developer Portal`
    return () => {
      document.title = 'Offline Developer Portal'
    }
  }, [doc, portal])

  const back = (
    <nav className="doc-back">
      <a href={searchHref(query, sources)}>← {query.trim() ? `Back to results for “${query.trim()}”` : 'Back to search'}</a>
    </nav>
  )

  if (error) {
    return (
      <div className="doc-page">
        {back}
        <div className="status-view">
          <h1>Document unavailable</h1>
          <p className="error-text">{error}</p>
        </div>
      </div>
    )
  }
  if (!doc) {
    return (
      <div className="doc-page">
        {back}
        <div className="status-view" role="status">
          <div className="spinner" aria-hidden="true" />
          <p>Loading document…</p>
        </div>
      </div>
    )
  }
  return <DocumentView doc={doc} portal={portal} query={query} sources={sources} terms={terms} back={back} />
}

function DocumentView({
  doc,
  portal,
  query,
  sources,
  terms,
  back,
}: {
  doc: DocumentData
  portal: Portal
  query: string
  sources: string[]
  terms: readonly string[]
  back: ReactNode
}) {
  // Show the page's own <h1> as the title (and not twice); fall back to <title>.
  const { title, blocks } = useMemo(() => splitTitle(doc), [doc])
  const toc = useMemo(() => tocEntries(doc.headings, blocks), [doc.headings, blocks])
  const activeAnchor = useScrollSpy(toc)
  const ctx: RenderContext = useMemo(
    () => ({ terms, resolveLink: (h) => portal.resolveLink(h), query, sources }),
    [terms, portal, query, sources],
  )

  const goTo = (anchor: string) => {
    document.getElementById(headingDomId(anchor))?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
    replaceHash(docHref(doc.id, { q: query, sources, anchor }))
  }

  const tocList = toc.length > 1 && (
    <ul>
      {toc.map((h) => (
        <li key={h.anchor} className={`toc-l${h.depth}`}>
          <a
            href={docHref(doc.id, { q: query, sources, anchor: h.anchor })}
            className={h.anchor === activeAnchor ? 'active' : undefined}
            onClick={(e) => {
              e.preventDefault()
              goTo(h.anchor)
            }}
          >
            {h.text}
          </a>
        </li>
      ))}
    </ul>
  )

  return (
    <div className="doc-page">
      {back}
      <div className={tocList ? 'doc-layout with-toc' : 'doc-layout'}>
        <article className="doc">
          <header className="doc-header">
            <SourceBadge id={doc.source_id} name={doc.source_name || portal.sourceName(doc.source_id)} />
            <h1>{title}</h1>
            <dl className="provenance">
              <div>
                <dt>Source</dt>
                <dd>{doc.source_name || portal.sourceName(doc.source_id)}</dd>
              </div>
              <div>
                <dt>Original URL</dt>
                <dd>
                  <span className="provenance-url">{doc.url}</span>{' '}
                  <a href={doc.url} target="_blank" rel="noopener noreferrer" className="external-link" title="Opens the original page; requires network access">
                    open original <ExternalIcon size={10} />
                  </a>
                </dd>
              </div>
              <div>
                <dt>Indexed</dt>
                <dd>
                  <time dateTime={doc.indexed_at}>{formatDateTime(doc.indexed_at)}</time>
                </dd>
              </div>
            </dl>
            {tocList && (
              <details className="toc-mobile">
                <summary>On this page</summary>
                {tocList}
              </details>
            )}
          </header>
          <div className="doc-body">
            <Blocks blocks={blocks} ctx={ctx} />
          </div>
          <footer className="doc-footer muted small">
            Local copy of <span className="provenance-url">{doc.url}</span>, indexed {formatDateTime(doc.indexed_at)}.
          </footer>
        </article>
        {tocList && (
          <aside className="toc" aria-label="On this page">
            <h2>On this page</h2>
            {tocList}
          </aside>
        )}
      </div>
    </div>
  )
}

/** Uses the first <h1> as the display title and removes it from the body. */
export function splitTitle(doc: DocumentData): { title: string; blocks: Block[] } {
  const idx = doc.blocks.findIndex((b) => b.type === 'heading')
  const first = idx >= 0 ? doc.blocks[idx] : undefined
  if (first && first.level === 1) {
    const text = (first.spans ?? []).map((s) => s.t).join('').trim()
    const onlyH1 = doc.blocks.filter((b) => b.type === 'heading' && b.level === 1).length === 1
    if (text && onlyH1) {
      return { title: text, blocks: doc.blocks.filter((_, i) => i !== idx) }
    }
  }
  return { title: doc.title, blocks: doc.blocks }
}

interface TocEntry {
  anchor: string
  text: string
  depth: number
}

function tocEntries(headings: Heading[], blocks: Block[]): TocEntry[] {
  const present = new Set<string>()
  const collect = (bs: Block[]) => {
    for (const b of bs) {
      if (b.type === 'heading' && b.anchor) present.add(b.anchor)
      if (b.items) b.items.forEach(collect)
      if (b.children) collect(b.children)
    }
  }
  collect(blocks)
  const hs = headings.filter((h) => present.has(h.anchor) && h.level <= 4)
  if (hs.length === 0) return []
  const min = Math.min(...hs.map((h) => h.level))
  return hs.map((h) => ({ anchor: h.anchor, text: h.text, depth: Math.min(h.level - min, 3) }))
}

/** Tracks which heading is currently at the top of the viewport. */
function useScrollSpy(toc: TocEntry[]): string | undefined {
  const [active, setActive] = useState<string>()
  useEffect(() => {
    if (toc.length < 2) return
    const onScroll = () => {
      let current: string | undefined
      for (const h of toc) {
        const el = document.getElementById(headingDomId(h.anchor))
        if (!el) continue
        if (el.getBoundingClientRect().top <= 140) current = h.anchor
        else break
      }
      setActive(current ?? toc[0].anchor)
    }
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [toc])
  return active
}
