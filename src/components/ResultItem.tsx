import { useEffect, useState } from 'react'
import { loadDocument } from '../corpus/load'
import type { Portal } from '../corpus/portal'
import type { IndexedDoc } from '../corpus/types'
import { docHref } from '../router'
import { bestSnippet, type Snippet } from '../search/snippet'
import { displayUrl } from '../util/format'
import { Highlighted } from './Highlighted'
import { SourceBadge } from './SourceBadge'

interface Props {
  index: number
  doc: IndexedDoc
  portal: Portal
  terms: readonly string[]
  query: string
  sources: readonly string[]
  active: boolean
  onHover: () => void
}

export function ResultItem({ index, doc, portal, terms, query, sources, active, onHover }: Props) {
  const [snippet, setSnippet] = useState<{ key: string; value: Snippet | undefined }>()
  const key = doc.id + '\u0000' + terms.join(' ')

  // Load the document lazily to show the most relevant passage.
  useEffect(() => {
    if (terms.length === 0) return
    let cancelled = false
    loadDocument(doc.id)
      .then((full) => {
        if (!cancelled) setSnippet({ key, value: bestSnippet(full, terms) })
      })
      .catch(() => {
        /* fall back to the summary */
      })
    return () => {
      cancelled = true
    }
  }, [doc.id, terms, key])

  const current = snippet?.key === key ? snippet.value : undefined
  const href = docHref(doc.id, { q: query, sources, anchor: current?.anchor })
  return (
    <li className={active ? 'result active' : 'result'} data-index={index} onMouseEnter={onHover}>
      <a className="result-link" href={href}>
        <div className="result-meta">
          <SourceBadge id={doc.source_id} name={portal.sourceName(doc.source_id)} />
          <span className="result-url" title={doc.url}>
            {displayUrl(doc.url)}
          </span>
        </div>
        <h3 className="result-title">
          <Highlighted text={portal.shortTitle(doc)} terms={terms} />
        </h3>
        <p className="result-snippet">
          {current?.heading && <span className="result-section">{current.heading} › </span>}
          <Highlighted text={current?.text ?? doc.summary} terms={terms} />
        </p>
      </a>
    </li>
  )
}
