import type { Block, DocumentData, Span } from '../corpus/types'
import { tokenizeWithPositions } from './tokenize'

/** A piece of document text together with the section it belongs to. */
export interface Segment {
  text: string
  code: boolean
  anchor?: string
  heading?: string
}

export function spansText(spans: Span[] | undefined): string {
  return (spans ?? []).map((s) => s.t).join('')
}

/** Flattens a document into searchable text segments, tagged with their section. */
export function documentSegments(doc: DocumentData): Segment[] {
  const out: Segment[] = []
  let anchor: string | undefined
  let heading: string | undefined
  const walk = (blocks: Block[]) => {
    for (const b of blocks) {
      switch (b.type) {
        case 'heading':
          anchor = b.anchor
          heading = spansText(b.spans)
          out.push({ text: heading, code: false, anchor, heading })
          break
        case 'paragraph':
          out.push({ text: spansText(b.spans), code: false, anchor, heading })
          break
        case 'code':
          out.push({ text: b.text ?? '', code: true, anchor, heading })
          break
        case 'list':
          for (const item of b.items ?? []) walk(item)
          break
        case 'quote':
          walk(b.children ?? [])
          break
        case 'table':
          for (const row of b.rows ?? []) out.push({ text: row.join(' · '), code: false, anchor, heading })
          break
      }
    }
  }
  walk(doc.blocks)
  return out
}

/** Whether a token matches a query term (exact or prefix, like the engine). */
export function tokenMatches(token: string, terms: readonly string[]): string | undefined {
  for (const t of terms) {
    if (token === t || ([...t].length >= 2 && token.startsWith(t))) return t
  }
  return undefined
}

export interface Snippet {
  text: string
  anchor?: string
  heading?: string
}

const BEFORE = 70
const AFTER = 180

/**
 * Picks the segment that matches the most distinct query terms (prose is
 * preferred over code on ties) and cuts a window of text around the first
 * match.
 */
export function bestSnippet(doc: DocumentData, terms: readonly string[]): Snippet | undefined {
  if (terms.length === 0) return undefined
  let best: { seg: Segment; text: string; score: number; first: number } | undefined
  for (const seg of documentSegments(doc)) {
    const text = seg.text.replace(/\s+/g, ' ')
    const found = new Set<string>()
    let first = -1
    for (const tok of tokenizeWithPositions(text)) {
      const t = tokenMatches(tok.token, terms)
      if (t) {
        found.add(t)
        if (first < 0) first = tok.start
      }
    }
    if (found.size === 0) continue
    const isHeadingOnly = seg.heading === seg.text
    const score = found.size * 10 + (seg.code ? 0 : 2) + (isHeadingOnly ? 0 : 1)
    if (!best || score > best.score) best = { seg, text, score, first }
  }
  if (!best) return undefined
  return {
    text: windowAround(best.text, best.first),
    anchor: best.seg.anchor,
    heading: best.seg.heading !== best.seg.text ? best.seg.heading : undefined,
  }
}

function windowAround(text: string, p: number): string {
  let start = Math.max(0, p - BEFORE)
  let end = Math.min(text.length, p + AFTER)
  if (start > 0) {
    const sp = text.indexOf(' ', start)
    if (sp >= 0 && sp < p) start = sp + 1
  }
  if (end < text.length) {
    const sp = text.lastIndexOf(' ', end)
    if (sp > p) end = sp
  }
  return (start > 0 ? '… ' : '') + text.slice(start, end).trim() + (end < text.length ? ' …' : '')
}

/** A piece of text that should or should not be highlighted. */
export interface HighlightPart {
  text: string
  hit: boolean
}

/** Splits text into highlighted and plain parts for the given query terms. */
export function highlight(text: string, terms: readonly string[]): HighlightPart[] {
  if (terms.length === 0 || text === '') return [{ text, hit: false }]
  const parts: HighlightPart[] = []
  let last = 0
  for (const tok of tokenizeWithPositions(text)) {
    if (!tokenMatches(tok.token, terms)) continue
    if (tok.start > last) parts.push({ text: text.slice(last, tok.start), hit: false })
    parts.push({ text: text.slice(tok.start, tok.end), hit: true })
    last = tok.end
  }
  if (last < text.length) parts.push({ text: text.slice(last), hit: false })
  return parts
}
