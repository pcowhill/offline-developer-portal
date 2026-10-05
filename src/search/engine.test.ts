import { describe, expect, it } from 'vitest'
import goldenIndex from '../../internal/index/testdata/golden_index.json'
import goldenSearch from '../../internal/index/testdata/golden_search.json'
import type { SearchIndexData } from '../corpus/types'
import { SearchEngine } from './engine'

const engine = new SearchEngine(goldenIndex as unknown as SearchIndexData)

describe('SearchEngine matches the Go implementation', () => {
  for (const [query, expected] of Object.entries(goldenSearch as Record<string, Array<{ id: string; score: number }>>)) {
    it(`ranks ${JSON.stringify(query)} like the Go indexer`, () => {
      const got = engine.search(query)
      expect(got.map((r) => r.doc.id)).toEqual(expected.map((r) => r.id))
      got.forEach((r, i) => expect(r.score).toBeCloseTo(expected[i].score, 4))
    })
  }
})

describe('SearchEngine', () => {
  it('filters by source', () => {
    const all = engine.search('re')
    expect(new Set(all.map((r) => r.doc.source_id)).size).toBeGreaterThan(1)
    const beta = engine.search('re', { sources: new Set(['beta']) })
    expect(beta.length).toBeGreaterThan(0)
    expect(beta.every((r) => r.doc.source_id === 'beta')).toBe(true)
  })

  it('returns nothing for empty or unknown queries', () => {
    expect(engine.search('')).toEqual([])
    expect(engine.search('   ...  ')).toEqual([])
    expect(engine.search('qqqqzzzz')).toEqual([])
  })

  it('is not confused by Object.prototype property names', () => {
    expect(engine.search('constructor')).toEqual([])
    expect(engine.search('__proto__ toString')).toEqual([])
  })

  it('browses documents by source', () => {
    expect(engine.browse(new Set(['alpha'])).every((d) => d.source_id === 'alpha')).toBe(true)
    expect(engine.browse().length).toBe(goldenIndex.docs.length)
  })
})
