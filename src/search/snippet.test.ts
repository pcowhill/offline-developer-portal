import { describe, expect, it } from 'vitest'
import type { DocumentData } from '../corpus/types'
import { bestSnippet, documentSegments, highlight } from './snippet'

const doc: DocumentData = {
  id: '0123456789abcdef',
  source_id: 's',
  source_name: 'S',
  url: 'https://e.com/',
  title: 'T',
  indexed_at: '2026-01-01T00:00:00Z',
  headings: [],
  blocks: [
    { type: 'heading', level: 1, anchor: 'intro', spans: [{ t: 'Introduction' }] },
    { type: 'paragraph', spans: [{ t: 'This page explains ' }, { t: 'widgets', c: true }, { t: ' in general.' }] },
    { type: 'heading', level: 2, anchor: 'config', spans: [{ t: 'Configuration' }] },
    {
      type: 'list',
      items: [[{ type: 'paragraph', spans: [{ t: 'Set the timeout option to control how long widgets wait for a response.' }] }]],
    },
    { type: 'code', text: 'widget --timeout 5' },
    { type: 'table', rows: [['option', 'meaning'], ['retries', 'number of retries']] },
  ],
}

describe('documentSegments', () => {
  it('tags text with the enclosing section', () => {
    const segs = documentSegments(doc)
    const timeout = segs.find((s) => s.text.startsWith('Set the timeout'))
    expect(timeout?.anchor).toBe('config')
    expect(timeout?.heading).toBe('Configuration')
    expect(segs.some((s) => s.code && s.text.includes('--timeout'))).toBe(true)
    expect(segs.some((s) => s.text.includes('retries'))).toBe(true)
  })
})

describe('bestSnippet', () => {
  it('prefers the passage matching the most terms, with its section', () => {
    const s = bestSnippet(doc, ['timeout', 'widgets'])
    expect(s?.anchor).toBe('config')
    expect(s?.heading).toBe('Configuration')
    expect(s?.text).toContain('timeout option')
  })

  it('matches word prefixes', () => {
    expect(bestSnippet(doc, ['config'])?.anchor).toBe('config')
  })

  it('returns undefined when nothing matches', () => {
    expect(bestSnippet(doc, ['nothing'])).toBeUndefined()
    expect(bestSnippet(doc, [])).toBeUndefined()
  })

  it('cuts long text with ellipses', () => {
    const long: DocumentData = { ...doc, blocks: [{ type: 'paragraph', spans: [{ t: 'word '.repeat(100) + 'needle ' + 'word '.repeat(100) }] }] }
    const s = bestSnippet(long, ['needle'])!
    expect(s.text.startsWith('… ')).toBe(true)
    expect(s.text.endsWith(' …')).toBe(true)
    expect(s.text).toContain('needle')
    expect(s.text.length).toBeLessThan(300)
  })
})

describe('highlight', () => {
  it('marks whole tokens that start with a query term', () => {
    const parts = highlight('Installing the installer, not reinstall.', ['install'])
    expect(parts.filter((p) => p.hit).map((p) => p.text)).toEqual(['Installing', 'installer'])
    expect(parts.map((p) => p.text).join('')).toBe('Installing the installer, not reinstall.')
  })

  it('returns the text unchanged without terms', () => {
    expect(highlight('abc', [])).toEqual([{ text: 'abc', hit: false }])
  })
})
