// Tokenizer v1 — must match internal/index/tokenize.go exactly.
// Both implementations are tested against
// internal/index/testdata/tokenizer_vectors.json.

export const MAX_TOKEN_LENGTH = 40

const TOKEN_RE = /[\p{L}\p{N}]+/gu

export interface PositionedToken {
  token: string
  start: number
  end: number
}

/** Splits text into lower-case tokens with their positions in the original string. */
export function tokenizeWithPositions(text: string): PositionedToken[] {
  const out: PositionedToken[] = []
  for (const m of text.matchAll(TOKEN_RE)) {
    const token = m[0].toLowerCase()
    const len = [...token].length
    if (len > 0 && len <= MAX_TOKEN_LENGTH) {
      out.push({ token, start: m.index, end: m.index + m[0].length })
    }
  }
  return out
}

/** Splits text into lower-case tokens (stop words are not removed). */
export function tokenize(text: string): string[] {
  return tokenizeWithPositions(text).map((t) => t.token)
}

/**
 * Tokenizes a query: stop words are removed unless the query consists only
 * of stop words; duplicates are removed. Mirrors index.QueryTerms.
 */
export function queryTerms(query: string, stopWords: ReadonlySet<string>): string[] {
  const toks = tokenize(query)
  const kept = [...new Set(toks.filter((t) => !stopWords.has(t)))]
  return kept.length > 0 ? kept : [...new Set(toks)]
}
