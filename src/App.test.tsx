import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import goldenIndex from '../internal/index/testdata/golden_index.json'
import App from './App'
import type { DocumentData, Manifest } from './corpus/types'

const loopsDoc = goldenIndex.docs.find((d) => d.title === 'Loops')!

const manifest: Manifest = {
  format: 'offline-developer-portal-corpus',
  format_version: 1,
  generator: 'offline-docs test',
  generated_at: '2026-01-02T03:04:05Z',
  document_count: goldenIndex.docs.length,
  search_index: 'search-index.json',
  documents_path: 'docs/',
  sources: [
    { id: 'alpha', name: 'Alpha Docs', start_urls: ['https://a.example/'], document_count: 2, indexed_at: '2026-01-02T03:04:05Z', status: 'ok', pages_fetched: 2, pages_skipped: 0, errors: 0, respects_robots_txt: true },
    { id: 'beta', name: 'Beta Docs', start_urls: ['https://b.example/'], document_count: 1, indexed_at: '2026-01-02T03:04:05Z', status: 'ok', pages_fetched: 1, pages_skipped: 0, errors: 0, respects_robots_txt: true },
  ],
}

const loops: DocumentData = {
  id: loopsDoc.id,
  source_id: 'beta',
  source_name: 'Beta Docs',
  url: loopsDoc.url,
  title: 'Loops',
  indexed_at: '2026-01-02T03:04:05Z',
  headings: [
    { level: 1, text: 'Loops', anchor: 'loops' },
    { level: 2, text: 'While', anchor: 'while' },
    { level: 2, text: 'For', anchor: 'for' },
  ],
  blocks: [
    { type: 'heading', level: 1, anchor: 'loops', spans: [{ t: 'Loops' }] },
    { type: 'paragraph', spans: [{ t: 'A for loop repeats a block of code. See ' }, { t: 'installation', h: goldenIndex.docs.find((d) => d.title === 'Installation guide')!.url }, { t: '.' }] },
    { type: 'heading', level: 2, anchor: 'while', spans: [{ t: 'While' }] },
    { type: 'paragraph', spans: [{ t: 'The while loop repeats while a condition holds.' }] },
    { type: 'heading', level: 2, anchor: 'for', spans: [{ t: 'For' }] },
    { type: 'code', text: 'for i in range(3):\n    print(i)', lang: 'python' },
  ],
}

function mockFetch(files: Record<string, unknown>) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input)
    const body = files[url]
    if (body === undefined) return new Response('not found', { status: 404 })
    return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
  })
}

beforeEach(() => {
  window.location.hash = ''
  window.scrollTo = vi.fn()
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('App', () => {
  it('shows setup instructions when no corpus exists', async () => {
    vi.stubGlobal('fetch', mockFetch({}))
    render(<App />)
    expect(await screen.findByText('No documentation has been indexed yet')).toBeTruthy()
    expect(screen.getByText('index-windows.cmd')).toBeTruthy()
  })

  it('searches, filters by source and opens a document offline', async () => {
    const fetch = mockFetch({
      'corpus/manifest.json': manifest,
      'corpus/search-index.json': goldenIndex,
      [`corpus/docs/${loops.id.slice(0, 2)}/${loops.id}.json`]: loops,
    })
    vi.stubGlobal('fetch', fetch)
    render(<App />)

    // Home view: sources overview and corpus status.
    expect(await screen.findByText('Search your offline documentation')).toBeTruthy()
    expect(screen.getByText('Indexed sources')).toBeTruthy()
    expect(screen.getAllByText('Alpha Docs').length).toBeGreaterThan(0)
    expect(screen.getByText(/3 documents · updated/)).toBeTruthy()

    // Search.
    const input = screen.getByRole('searchbox', { name: 'Search documentation' })
    fireEvent.change(input, { target: { value: 'loop' } })
    expect(await screen.findByText(/1 result/)).toBeTruthy()
    const link = await screen.findByRole('link', { name: /Loops/ })
    expect(link.textContent).toContain('b.example/loops')
    // Snippet is computed from the lazily loaded document.
    await waitFor(() => expect(link.textContent).toContain('A for loop repeats'))

    // Source filter: restricting to Alpha hides the Beta result.
    fireEvent.click(screen.getByRole('checkbox', { name: /Alpha Docs/ }))
    expect(await screen.findByText(/No matches for/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Search all sources instead' }))
    expect(await screen.findByText(/1 result/)).toBeTruthy()

    // Open the document (hash navigation).
    await act(async () => {
      window.location.hash = (await screen.findByRole('link', { name: /Loops/ })).getAttribute('href')!
      window.dispatchEvent(new HashChangeEvent('hashchange'))
    })
    expect(await screen.findByRole('heading', { level: 1, name: 'Loops' })).toBeTruthy()
    expect(screen.getAllByText(loops.url).length).toBeGreaterThan(0)
    expect(screen.getByText('python')).toBeTruthy()
    expect(screen.getAllByText('On this page').length).toBeGreaterThan(0)
    // Links to other indexed documents stay inside the portal.
    const internal = screen.getByRole('link', { name: 'installation' })
    expect(internal.getAttribute('href')).toMatch(/^#\/doc\/[0-9a-f]{16}/)
    // Back navigation keeps the query.
    expect(screen.getByRole('link', { name: /Back to results for “loop”/ })).toBeTruthy()

    // Nothing was requested from outside the local corpus.
    for (const call of fetch.mock.calls) expect(String(call[0])).toMatch(/^corpus\//)
  })
})
