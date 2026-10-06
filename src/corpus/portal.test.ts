import { describe, expect, it } from 'vitest'
import goldenIndex from '../../internal/index/testdata/golden_index.json'
import { commonTitleSuffixes, Portal } from './portal'
import type { IndexedDoc, Manifest, SearchIndexData } from './types'

const doc = (id: string, source: string, title: string): IndexedDoc => ({ id, source_id: source, title, url: `https://e.com/${id}`, length: 1, summary: '' })

describe('commonTitleSuffixes', () => {
  it('finds a site-wide suffix shared by most titles', () => {
    const m = commonTitleSuffixes([
      doc('1', 'py', '4. More Control Flow Tools — Python 3.14 documentation'),
      doc('2', 'py', '5. Data Structures — Python 3.14 documentation'),
      doc('3', 'py', 'Glossary — Python 3.14 documentation'),
      doc('4', 'go', 'Effective Go - The Go Programming Language'),
      doc('5', 'go', 'Tutorial: Get started - The Go Programming Language'),
      doc('6', 'solo', 'Only - One'),
    ])
    expect(m.get('py')).toBe(' — Python 3.14 documentation')
    expect(m.get('go')).toBe(' - The Go Programming Language')
    expect(m.has('solo')).toBe(false)
  })
})

describe('Portal', () => {
  const manifest = {
    format: 'offline-developer-portal-corpus',
    format_version: 1,
    generator: 't',
    generated_at: '2026-01-01T00:00:00Z',
    document_count: 3,
    search_index: 'search-index.json',
    documents_path: 'docs/',
    sources: [{ id: 'alpha', name: 'Alpha Docs' }],
  } as unknown as Manifest
  const portal = new Portal({ manifest, index: goldenIndex as unknown as SearchIndexData })

  it('resolves links to indexed documents', () => {
    const d = portal.engine.docs[0]
    expect(portal.resolveLink(d.url)).toEqual({ id: d.id, anchor: undefined })
    expect(portal.resolveLink(d.url + '#Some%20Section')).toEqual({ id: d.id, anchor: 'Some Section' })
    expect(portal.resolveLink('https://elsewhere.example/')).toBeUndefined()
  })

  it('names sources', () => {
    expect(portal.sourceName('alpha')).toBe('Alpha Docs')
    expect(portal.sourceName('unknown')).toBe('unknown')
  })
})
