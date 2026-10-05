import type { IndexedDoc, SearchIndexData } from '../corpus/types'
import { queryTerms } from './tokenize'

// Ranking mirrors internal/index/search.go. Keep the two in sync.
const PREFIX_FACTOR = 0.5
const MAX_PREFIX_EXPANSIONS = 30
const MIN_PREFIX_LENGTH = 2

export interface SearchResult {
  doc: IndexedDoc
  score: number
}

export interface SearchOptions {
  /** Restrict to these source ids. Empty or undefined means all sources. */
  sources?: ReadonlySet<string>
}

export class SearchEngine {
  readonly docs: IndexedDoc[]
  readonly stopWords: ReadonlySet<string>
  private readonly terms: Map<string, number[]>
  private readonly sortedTerms: string[]
  private readonly k1: number
  private readonly b: number
  private readonly avgDocLength: number

  constructor(data: SearchIndexData) {
    this.docs = data.docs
    this.stopWords = new Set(data.tokenizer.stop_words)
    this.terms = new Map(Object.entries(data.terms))
    this.sortedTerms = [...this.terms.keys()].sort(compareStrings)
    this.k1 = data.bm25.k1
    this.b = data.bm25.b
    this.avgDocLength = data.avg_doc_length > 0 ? data.avg_doc_length : 1
  }

  queryTerms(query: string): string[] {
    return queryTerms(query, this.stopWords)
  }

  search(query: string, opts: SearchOptions = {}): SearchResult[] {
    const terms = this.queryTerms(query)
    if (terms.length === 0 || this.docs.length === 0) return []
    const n = this.docs.length
    const total = new Map<number, number>()
    const matched = new Map<number, number>()

    for (const qt of terms) {
      const cands: Array<[string, number]> = []
      if (this.terms.has(qt)) cands.push([qt, 1])
      if ([...qt].length >= MIN_PREFIX_LENGTH) {
        const exp: string[] = []
        for (let i = lowerBound(this.sortedTerms, qt); i < this.sortedTerms.length; i++) {
          const t = this.sortedTerms[i]
          if (!t.startsWith(qt)) break
          if (t !== qt) exp.push(t)
        }
        // Prefer the most common expansions (stable sort keeps lexical order on ties).
        exp.sort((a, c) => this.terms.get(c)!.length - this.terms.get(a)!.length)
        for (const e of exp.slice(0, MAX_PREFIX_EXPANSIONS)) cands.push([e, PREFIX_FACTOR])
      }
      const best = new Map<number, number>()
      for (const [term, factor] of cands) {
        const postings = this.terms.get(term)!
        const df = Math.floor(postings.length / 2)
        const idf = Math.log(1 + (n - df + 0.5) / (df + 0.5))
        for (let p = 0; p + 1 < postings.length; p += 2) {
          const d = postings[p]
          const tf = postings[p + 1]
          const dl = this.docs[d].length
          const s =
            (factor * idf * tf * (this.k1 + 1)) /
            (tf + this.k1 * (1 - this.b + (this.b * dl) / this.avgDocLength))
          if (s > (best.get(d) ?? 0)) best.set(d, s)
        }
      }
      for (const [d, s] of best) {
        total.set(d, (total.get(d) ?? 0) + s)
        matched.set(d, (matched.get(d) ?? 0) + 1)
      }
    }

    const phrase = query.trim().split(/\s+/).join(' ').toLowerCase()
    const allowed = opts.sources && opts.sources.size > 0 ? opts.sources : null
    const results: SearchResult[] = []
    for (const [d, s] of total) {
      const doc = this.docs[d]
      if (allowed && !allowed.has(doc.source_id)) continue
      const coord = matched.get(d)! / terms.length
      let score = s * coord * coord
      const title = doc.title.toLowerCase()
      if (phrase !== '' && title === phrase) score *= 2
      else if (terms.length > 1 && title.includes(phrase)) score *= 1.5
      results.push({ doc, score })
    }
    results.sort((a, c) => c.score - a.score || compareStrings(a.doc.id, c.doc.id))
    return results
  }

  /** All documents of the given sources (or all), ordered by source then title. */
  browse(sources?: ReadonlySet<string>): IndexedDoc[] {
    return this.docs.filter((d) => !sources || sources.size === 0 || sources.has(d.source_id))
  }
}

/** Byte-order string comparison (matches Go's sort.Strings for ASCII/UTF-16 BMP). */
function compareStrings(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

function lowerBound(arr: string[], key: string): number {
  let lo = 0
  let hi = arr.length
  while (lo < hi) {
    const mid = (lo + hi) >>> 1
    if (arr[mid] < key) lo = mid + 1
    else hi = mid
  }
  return lo
}
