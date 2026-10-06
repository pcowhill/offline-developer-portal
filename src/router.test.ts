import { describe, expect, it } from 'vitest'
import { docHref, parseHash, searchHref } from './router'

describe('router', () => {
  it('parses search routes', () => {
    expect(parseHash('')).toEqual({ name: 'search', q: '', sources: [] })
    expect(parseHash('#/?q=for+loop&s=a,b')).toEqual({ name: 'search', q: 'for loop', sources: ['a', 'b'] })
  })

  it('parses document routes', () => {
    expect(parseHash('#/doc/0123456789abcdef?q=x&a=sec%201')).toEqual({
      name: 'doc',
      id: '0123456789abcdef',
      q: 'x',
      sources: [],
      anchor: 'sec 1',
    })
  })

  it('round-trips hrefs', () => {
    const s = searchHref('a & b', ['x', 'y'])
    expect(parseHash(s)).toEqual({ name: 'search', q: 'a & b', sources: ['x', 'y'] })
    const d = docHref('0123456789abcdef', { q: 'q', sources: ['x'], anchor: 'intro' })
    expect(parseHash(d)).toEqual({ name: 'doc', id: '0123456789abcdef', q: 'q', sources: ['x'], anchor: 'intro' })
    expect(searchHref('', [])).toBe('#/')
  })
})
