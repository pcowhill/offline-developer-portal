import { SearchEngine } from '../search/engine'
import type { LoadedCorpus } from './load'
import type { IndexedDoc, Manifest, SourceManifest } from './types'

const SEPARATORS = [' - ', ' — ', ' – ', ' | ', ' · ', ' :: ']

/**
 * Finds a site-wide title suffix such as " — Python 3.14 documentation"
 * shared by most documents of each source, so result lists can show the
 * distinctive part of the title. The full title is still shown on the
 * document page.
 */
export function commonTitleSuffixes(docs: readonly IndexedDoc[]): Map<string, string> {
  const bySource = new Map<string, string[]>()
  for (const d of docs) {
    const list = bySource.get(d.source_id) ?? []
    list.push(d.title)
    bySource.set(d.source_id, list)
  }
  const out = new Map<string, string>()
  for (const [source, titles] of bySource) {
    if (titles.length < 2) continue
    const counts = new Map<string, number>()
    for (const t of titles) {
      const seen = new Set<string>()
      for (const sep of SEPARATORS) {
        let i = t.indexOf(sep)
        while (i > 0) {
          const suffix = t.slice(i)
          if (!seen.has(suffix)) {
            seen.add(suffix)
            counts.set(suffix, (counts.get(suffix) ?? 0) + 1)
          }
          i = t.indexOf(sep, i + 1)
        }
      }
    }
    let best = ''
    let bestCount = 0
    for (const [suffix, n] of counts) {
      if (n > bestCount || (n === bestCount && suffix.length > best.length)) {
        best = suffix
        bestCount = n
      }
    }
    if (best && bestCount >= Math.max(2, Math.ceil(titles.length / 2))) out.set(source, best)
  }
  return out
}

export interface LinkTarget {
  id: string
  anchor?: string
}

/** Everything the UI needs about a loaded corpus. */
export class Portal {
  readonly manifest: Manifest
  readonly engine: SearchEngine
  readonly sources: SourceManifest[]
  private readonly sourceById: Map<string, SourceManifest>
  private readonly suffixes: Map<string, string>
  private readonly urlToId: Map<string, string>
  private readonly docById: Map<string, IndexedDoc>

  constructor(corpus: LoadedCorpus) {
    this.manifest = corpus.manifest
    this.engine = new SearchEngine(corpus.index)
    this.sources = corpus.manifest.sources
    this.sourceById = new Map(this.sources.map((s) => [s.id, s]))
    this.suffixes = commonTitleSuffixes(this.engine.docs)
    this.urlToId = new Map()
    this.docById = new Map()
    for (const d of this.engine.docs) {
      if (!this.urlToId.has(d.url)) this.urlToId.set(d.url, d.id)
      this.docById.set(d.id, d)
    }
  }

  sourceName(id: string): string {
    return this.sourceById.get(id)?.name ?? id
  }

  doc(id: string): IndexedDoc | undefined {
    return this.docById.get(id)
  }

  /** Title without the source-wide suffix. */
  shortTitle(doc: { title: string; source_id: string }): string {
    const suffix = this.suffixes.get(doc.source_id)
    if (suffix && doc.title.endsWith(suffix) && doc.title.length > suffix.length) {
      return doc.title.slice(0, -suffix.length).trim()
    }
    return doc.title
  }

  /** Maps an original URL (possibly with #fragment) to an indexed document. */
  resolveLink(href: string): LinkTarget | undefined {
    const hash = href.indexOf('#')
    const base = hash >= 0 ? href.slice(0, hash) : href
    const id = this.urlToId.get(base)
    if (!id) return undefined
    let anchor: string | undefined
    if (hash >= 0) {
      try {
        anchor = decodeURIComponent(href.slice(hash + 1)) || undefined
      } catch {
        anchor = href.slice(hash + 1) || undefined
      }
    }
    return { id, anchor }
  }
}
