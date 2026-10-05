import {
  CORPUS_FORMAT,
  FORMAT_VERSION,
  SEARCH_INDEX_FORMAT,
  type DocumentData,
  type Manifest,
  type SearchIndexData,
} from './types'

/** Base URL of the corpus, relative to the page so dist/ works from any path. */
export const CORPUS_BASE = 'corpus/'

export class CorpusMissingError extends Error {
  constructor() {
    super('No corpus has been generated yet.')
    this.name = 'CorpusMissingError'
  }
}

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(CORPUS_BASE + path, { cache: 'no-cache' })
  if (res.status === 404) throw new CorpusMissingError()
  if (!res.ok) throw new Error(`Could not load ${path}: HTTP ${res.status}`)
  return (await res.json()) as T
}

export interface LoadedCorpus {
  manifest: Manifest
  index: SearchIndexData
}

export async function loadCorpus(): Promise<LoadedCorpus> {
  const manifest = await getJSON<Manifest>('manifest.json')
  if (manifest.format !== CORPUS_FORMAT || manifest.format_version !== FORMAT_VERSION) {
    throw new Error(
      `Unsupported corpus format (${manifest.format} v${manifest.format_version}). Re-run indexing with this version of the portal.`,
    )
  }
  const index = await getJSON<SearchIndexData>(manifest.search_index || 'search-index.json')
  if (index.format !== SEARCH_INDEX_FORMAT || index.format_version !== FORMAT_VERSION) {
    throw new Error('Unsupported search index format. Re-run indexing with this version of the portal.')
  }
  return { manifest, index }
}

/** Path of a document file relative to the corpus root (mirrors index.DocumentPath). */
export function documentPath(id: string): string {
  return `docs/${id.slice(0, 2)}/${id}.json`
}

const docCache = new Map<string, Promise<DocumentData>>()

export function loadDocument(id: string): Promise<DocumentData> {
  if (!/^[0-9a-f]{16}$/.test(id)) return Promise.reject(new Error('Invalid document id'))
  let p = docCache.get(id)
  if (!p) {
    p = getJSON<DocumentData>(documentPath(id))
    p.catch(() => docCache.delete(id))
    docCache.set(id, p)
  }
  return p
}
