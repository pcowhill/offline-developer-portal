# Corpus format (version 1)

`offline-docs index` writes a self-contained directory of JSON files. The
portal, the `offline-docs search` command and any future tool (for example
an MCP server) read the same files; no service has to be running.

```
corpus/
├── manifest.json        written last; its presence marks a complete corpus
├── search-index.json    inverted index + per-document metadata
└── docs/
    └── <id[0:2]>/<id>.json   one file per document
```

All files are UTF-8 JSON. Timestamps are RFC 3339 in UTC. Format changes that
are not backwards compatible must increase `format_version` (Go:
`index.FormatVersion`, TypeScript: `FORMAT_VERSION`).

## manifest.json

```json
{
  "format": "offline-developer-portal-corpus",
  "format_version": 1,
  "generator": "offline-docs v0.1.0",
  "generated_at": "2026-10-05T19:23:58Z",
  "document_count": 118,
  "search_index": "search-index.json",
  "documents_path": "docs/",
  "sources": [
    {
      "id": "python-tutorial",
      "name": "Python Tutorial",
      "start_urls": ["https://docs.python.org/3/tutorial/index.html"],
      "document_count": 18,
      "indexed_at": "2026-10-05T19:23:52Z",
      "status": "ok",
      "pages_fetched": 18,
      "pages_skipped": 0,
      "errors": 0,
      "error_samples": [],
      "respects_robots_txt": true
    }
  ]
}
```

`status` is `ok`, `partial` (some pages failed), or `failed` (no documents).

## Documents: docs/xx/&lt;id&gt;.json

`id` is the first 16 hex characters of `sha256(source_id + "\0" + url)`, so it
is stable across re-indexing.

```json
{
  "id": "05f865cc4d233081",
  "source_id": "python-tutorial",
  "source_name": "Python Tutorial",
  "url": "https://docs.python.org/3/tutorial/controlflow.html",
  "title": "4. More Control Flow Tools — Python 3.14 documentation",
  "description": "optional <meta name=description>",
  "indexed_at": "2026-10-05T19:23:52Z",
  "headings": [{ "level": 2, "text": "4.2. for Statements", "anchor": "for-statements" }],
  "blocks": [ ... ]
}
```

Blocks (`type` decides which fields are present):

| type | fields |
| --- | --- |
| `heading` | `level` (1-6), `anchor`, `spans` |
| `paragraph` | `spans` |
| `code` | `text` (whitespace preserved), `lang` (optional) |
| `list` | `ordered`, `items`: array of block arrays |
| `quote` | `children`: block array (also used for `<dd>`) |
| `table` | `rows`: array of string arrays, `header`: first row is a header |

Spans: `{"t": "text", "c": true /* code */, "b": true /* bold */, "i": true /* italic */, "h": "https://absolute/link#fragment"}`.

## search-index.json

```json
{
  "format": "offline-developer-portal-search-index",
  "format_version": 1,
  "tokenizer": { "version": 1, "max_token_length": 40, "stop_words": ["a", "an", "the", "..."] },
  "weights": { "title": 6, "heading": 3, "body": 1 },
  "bm25": { "k1": 1.2, "b": 0.75 },
  "avg_doc_length": 1854.7,
  "docs": [
    { "id": "05f865cc4d233081", "source_id": "python-tutorial", "title": "...", "url": "...", "length": 2310, "summary": "first ~240 characters" }
  ],
  "terms": { "loop": [3, 7, 12, 1] }
}
```

- `docs` are ordered by `source_id`, then `title`, then `url`.
- `terms[t]` is a flat posting list `[docIndex, weightedTF, docIndex, weightedTF, ...]`
  sorted by `docIndex` (an index into `docs`). `weightedTF` sums
  `weights.title` per occurrence in the title, `weights.heading` per
  occurrence in a heading and `weights.body` per occurrence in prose, lists,
  tables and code. `length` is the sum of a document's weighted TFs.

### Tokenizer (version 1)

A token is a maximal run of Unicode letters or numbers (`[\p{L}\p{N}]+`),
lower-cased; tokens longer than 40 code points are dropped; stop words are
removed when indexing. Test vectors: `internal/index/testdata/tokenizer_vectors.json`.

### Ranking

For a query, terms are tokenized as above (stop words dropped unless the
query has nothing else; duplicates removed). For each query term the exact
term (factor 1.0) and up to 30 most frequent terms starting with it (factor
0.5, only for terms of 2+ characters) are scored with BM25:

```
idf   = ln(1 + (N - df + 0.5) / (df + 0.5))
score = factor * idf * tf * (k1 + 1) / (tf + k1 * (1 - b + b * length / avg_doc_length))
```

Each query term contributes its best-scoring candidate. The sum is multiplied
by `(matchedTerms / queryTerms)²`, then by 2 if the title equals the query or
by 1.5 if a multi-word query appears in the title. Ties are broken by
document id. Golden results: `internal/index/testdata/golden_search.json`.
