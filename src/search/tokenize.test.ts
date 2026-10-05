import { describe, expect, it } from 'vitest'
import vectors from '../../internal/index/testdata/tokenizer_vectors.json'
import goldenIndex from '../../internal/index/testdata/golden_index.json'
import { queryTerms, tokenize, tokenizeWithPositions } from './tokenize'

describe('tokenizer (shared vectors with Go)', () => {
  for (const c of vectors.cases) {
    it(`tokenizes ${JSON.stringify(c.input)}`, () => {
      expect(tokenize(c.input)).toEqual(c.tokens)
    })
  }
  const stop = new Set(goldenIndex.tokenizer.stop_words)
  for (const c of vectors.query_cases) {
    it(`query terms for ${JSON.stringify(c.query)}`, () => {
      expect(queryTerms(c.query, stop)).toEqual(c.terms)
    })
  }
})

describe('tokenizeWithPositions', () => {
  it('reports positions in the original string', () => {
    const text = 'Use net/http.Get()'
    const toks = tokenizeWithPositions(text)
    expect(toks.map((t) => text.slice(t.start, t.end))).toEqual(['Use', 'net', 'http', 'Get'])
  })
})
