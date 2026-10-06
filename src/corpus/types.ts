// Types for the corpus files written by `offline-docs index`.
// See docs/corpus-format.md and internal/index/*.go.

export interface SourceManifest {
  id: string
  name: string
  start_urls: string[]
  document_count: number
  indexed_at: string
  status: 'ok' | 'partial' | 'failed' | string
  pages_fetched: number
  pages_skipped: number
  errors: number
  error_samples?: string[]
  respects_robots_txt: boolean
}

export interface Manifest {
  format: string
  format_version: number
  generator: string
  generated_at: string
  document_count: number
  search_index: string
  documents_path: string
  sources: SourceManifest[]
}

export interface IndexedDoc {
  id: string
  source_id: string
  title: string
  url: string
  length: number
  summary: string
}

export interface SearchIndexData {
  format: string
  format_version: number
  tokenizer: { version: number; max_token_length: number; stop_words: string[] }
  weights: Record<string, number>
  bm25: { k1: number; b: number }
  avg_doc_length: number
  docs: IndexedDoc[]
  terms: Record<string, number[]>
}

export interface Span {
  t: string
  c?: boolean
  b?: boolean
  i?: boolean
  h?: string
}

export interface Block {
  type: 'heading' | 'paragraph' | 'code' | 'list' | 'quote' | 'table' | string
  level?: number
  anchor?: string
  spans?: Span[]
  text?: string
  lang?: string
  ordered?: boolean
  items?: Block[][]
  children?: Block[]
  rows?: string[][]
  header?: boolean
}

export interface Heading {
  level: number
  text: string
  anchor: string
}

export interface DocumentData {
  id: string
  source_id: string
  source_name: string
  url: string
  title: string
  description?: string
  indexed_at: string
  headings: Heading[]
  blocks: Block[]
}

export const CORPUS_FORMAT = 'offline-developer-portal-corpus'
export const SEARCH_INDEX_FORMAT = 'offline-developer-portal-search-index'
export const FORMAT_VERSION = 1
