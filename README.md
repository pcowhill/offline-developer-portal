# Offline Developer Portal

A static documentation search portal that works **completely offline**.

1. **Index** (once, while the sites are reachable): a small Go tool crawls the
   documentation sites you list in a local `sources.yaml`, extracts the
   readable content, and writes a searchable corpus to disk.
2. **Browse** (any time, no network): the same tool serves the prebuilt static
   portal and that corpus on `http://localhost:8080`. Searching and reading
   use only local files. Neither the original sites nor the Internet are needed.

The prototype targets two environments:

- an Internet-connected **Windows** PC with no developer tools: download a
  release, extract it, edit `sources.yaml`, and double-click two scripts;
- an isolated **Linux** network: the same release (or a local build), with a
  different, private `sources.yaml` whose URLs never leave that network.

> **Status:** v0.1 prototype. It works well on small and medium static
> documentation sites (tens to a few thousand pages). See
> [Known limitations](#known-limitations).

---

## Contents

- [Architecture](#architecture)
- [Index vs. serve lifecycle](#index-vs-serve-lifecycle)
- [Quick start: Windows](#quick-start-windows)
- [Quick start: Linux](#quick-start-linux)
- [Configuration reference](#configuration-reference)
- [How private URLs stay private](#how-private-urls-stay-private)
- [Updating and re-indexing](#updating-and-re-indexing)
- [Command-line reference](#command-line-reference)
- [Building from source](#building-from-source)
- [Development](#development)
- [Release process](#release-process)
- [Known limitations](#known-limitations)
- [Roadmap](#roadmap)

---

## Architecture

```
                         ┌──────────────────────── offline-docs (Go, single binary) ─────────────────────────┐
  sources.yaml ────────► │ index:  config ─► crawler ─► extract ─► index builder ─► corpus writer           │
  (local, gitignored)    │          │          │ GET only, robots.txt, boundaries, max_pages                  │
                         │          ▼          ▼                                                             │
                         │   validation   doc sites (http/https)                                             │
                         │                                                                                  │
                         │ serve:  static file server on 127.0.0.1 (GET/HEAD only, strict CSP)              │
                         └───────────────────────────────┬───────────────────────────────┬──────────────────┘
                                                         │ /                             │ /corpus/
                                                         ▼                               ▼
                                     dist/  (committed static build)            corpus/  (generated, local)
                                     React + TypeScript + Vite                   manifest.json
                                     search engine runs in the browser           search-index.json
                                                                                 docs/xx/<id>.json
                                                         │                               │
                                                         └──────────────► browser ◄──────┘
                                                                   search · filter · read
```

| Part | Technology | Location |
| --- | --- | --- |
| Portal (static UI) | TypeScript, React, Vite — no runtime dependencies, fonts or CDNs | `src/` → built into `dist/` (committed) |
| Crawler / indexer / server | Go 1.25, vendored dependencies (`golang.org/x/net/html`, `gopkg.in/yaml.v3`) | `cmd/offline-docs/`, `internal/` |
| Search | Inverted index (BM25 over title/heading/body fields, prefix matching) built by Go, queried in the browser | `internal/index/`, `src/search/` |
| Corpus | Plain JSON files, documented in [docs/corpus-format.md](docs/corpus-format.md) | `corpus/` (generated, gitignored) |
| CI / releases | GitHub Actions | `.github/workflows/` |

Design choices:

- **No search backend.** The Go indexer precomputes the inverted index; the
  browser loads it once and ranks results locally (typically < 10 ms). The
  same files can be read by the `offline-docs search` CLI, and later by an MCP
  server, without any running service.
- **Hash routing and relative asset paths** so `dist/` works from any static
  file server and any folder.
- **Documents are stored as structured blocks** (headings, paragraphs, lists,
  tables, code) rather than raw HTML, so pages render safely (no injected
  scripts or styles), consistently, and fully offline.
- **The server only reads files** and sends a strict `Content-Security-Policy`
  that forbids loading anything from other origins.

### Repository layout

```
cmd/offline-docs/        CLI entry point: index, serve, validate, search, version
internal/config/         sources.yaml parsing, validation, crawl-boundary rules
internal/urlnorm/        URL normalization (dedupe fragments, queries, ports, ...)
internal/robots/         robots.txt parser
internal/extract/        HTML → title, headings, prose, lists, tables, code
internal/crawler/        polite, bounded crawler for one source
internal/index/          corpus format, tokenizer, search index, reader/writer, search
internal/indexer/        the "index" phase: crawl all sources, write corpus atomically
internal/server/         the "serve" phase: static file server
internal/testsite/       fake documentation site used by tests
src/                     portal source (React + TypeScript)
public/                  static assets copied into dist/
dist/                    production build of the portal (committed on purpose)
vendor/                  vendored Go modules (offline builds)
scripts/                 build, packaging, dist verification, end-to-end test
packaging/               QUICKSTART files shipped in release archives
docs/corpus-format.md    corpus/search-index format reference
sources.example.yaml     safe example configuration (public sites only)
index-windows.cmd / serve-windows.cmd / index-linux.sh / serve-linux.sh
```

---

## Index vs. serve lifecycle

```
 edit sources.yaml ─► index-*.cmd/.sh ─► corpus/ ─► serve-*.cmd/.sh ─► browser
       (local)          needs network      files        no network       local only
                        to the doc sites
                        exits when done
```

**Index phase** (`offline-docs index`)

1. Reads and validates `sources.yaml` (unknown keys are errors, so a typo
   cannot silently remove a boundary). Prints the exact boundaries it will use.
2. For each source: fetches `robots.txt`, then crawls breadth-first from the
   start URLs, following only links that pass the source's boundaries, up to
   `max_pages` requests.
3. Extracts the title, headings, prose, lists, tables and code blocks of each
   HTML page; skips non-HTML responses, duplicates and `noindex` pages.
4. Writes the corpus to `corpus.tmp/`, then swaps it into `corpus/`. If
   indexing fails or is interrupted, the previous corpus is left untouched.
5. Exits. Nothing keeps running.

**Serve phase** (`offline-docs serve`)

1. Serves `dist/` at `/` and `corpus/` at `/corpus/` on `127.0.0.1:8080`
   (next free port if busy) and prints the URL. On Windows it also opens the
   default browser.
2. The portal loads `corpus/manifest.json` and `corpus/search-index.json`,
   and fetches individual documents on demand. No other requests are made.

---

## Quick start: Windows

No installation, no administrator rights, no developer tools.

1. Download `offline-developer-portal-vX.Y.Z-windows-amd64.zip` from the
   [Releases page](https://github.com/pcowhill/offline-developer-portal/releases).
2. Right-click → **Extract All…** into a folder you own (e.g. `Documents`).
3. Double-click **`index-windows.cmd`**. The first time, it offers to create
   `sources.yaml` from `sources.example.yaml` and opens it in Notepad. Edit the
   sources (or keep the examples), save, and close Notepad.
   *Make sure the file is called `sources.yaml`, not `sources.yaml.txt`.*
4. Double-click **`index-windows.cmd`** again. It shows progress for each page
   and a summary, and keeps the window open at the end.
5. Double-click **`serve-windows.cmd`**. Your browser opens
   `http://localhost:8080`. Keep the console window open while browsing; close
   it to stop the portal.

If Windows SmartScreen shows "Windows protected your PC", choose **More info →
Run anyway** (the binaries are not code-signed in this prototype).

## Quick start: Linux

```sh
tar xzf offline-developer-portal-vX.Y.Z-linux-amd64.tar.gz
cd offline-developer-portal-vX.Y.Z-linux-amd64
cp sources.example.yaml sources.yaml     # edit it
./index-linux.sh                         # phase 1: crawl + build corpus
./serve-linux.sh                         # phase 2: http://localhost:8080
```

`./serve-linux.sh --open` opens a browser; `--port 9000` changes the port;
`--host 0.0.0.0` makes the portal reachable from other machines on the
(isolated) network — the default only accepts local connections.

To move a corpus between machines, copy the whole release folder including
`corpus/` (for example, index on one machine of the isolated network and
serve on another).

---

## Configuration reference

`sources.yaml` (copy it from [`sources.example.yaml`](sources.example.yaml)):

```yaml
settings:                       # optional; defaults shown
  concurrency: 4                # parallel requests per source (1-16)
  request_delay_ms: 250         # minimum gap between requests to the same host
  request_timeout_seconds: 20
  max_page_bytes: 5242880       # larger pages are skipped
  respect_robots_txt: true      # false ONLY if you own / are authorized to crawl
  user_agent_contact: ""        # appended to the User-Agent, e.g. an e-mail address

sources:
  - id: example-docs                       # required, stable: [a-z0-9][a-z0-9_-]*
    name: Example Documentation             # display name (defaults to id)
    start_urls:                             # required
      - https://example.com/docs/
    allowed_prefixes:                       # URL prefixes that may be crawled
      - https://example.com/docs/
    allowed_hosts: []                       # whole hosts that may be crawled ("docs.internal", "wiki:8080")
    max_pages: 100                          # max page requests (default 100, hard limit 20000)
    max_depth: 0                            # max link depth from a start URL (0 = unlimited)
    include_patterns: []                    # regexes; if set, a URL must match one
    exclude_patterns: []                    # regexes; matching URLs are skipped
    keep_query_strings: false               # treat ?a=b variants as separate pages
    respect_robots_txt: true                # per-source override
```

### Crawl boundaries (safety rules)

A URL is fetched only if **all** of these hold:

1. it is `http` or `https` and contains no credentials;
2. it starts with one of `allowed_prefixes` (scheme, host and port must match
   exactly — `https://example.com/docs/` never matches
   `https://example.com.evil.net/docs/`), **or** its host is exactly one of
   `allowed_hosts`;
3. it matches at least one `include_patterns` entry (if any are given);
4. it matches no `exclude_patterns` entry;
5. `robots.txt` allows it (unless explicitly overridden);
6. the source has made fewer than `max_pages` page requests.

Redirects are checked against the same rules before they are followed.
Every start URL must itself be inside the boundaries, or the configuration
is rejected. At least one of `allowed_prefixes` / `allowed_hosts` is required —
the crawler never guesses. Patterns use Go's RE2 regular-expression syntax and
are matched against the full normalized URL.

Other behavior: only `GET` requests (no `POST`, no `HEAD`); no JavaScript; no
cookies, logins or forms; a recognizable User-Agent
(`OfflineDeveloperPortal/<version> (+offline documentation indexer; respects robots.txt)`);
the site's `Crawl-delay` is honored (up to 30 s); `<meta name="robots" content="noindex/nofollow">`
is honored; obvious binary links (`.pdf`, `.zip`, images, …) are skipped and
every response's `Content-Type` is checked; URLs are normalized (fragments
removed, query strings dropped by default, default ports and `.`/`..`
segments removed) so each page is fetched once; one failing page never stops
the crawl; a failed request is retried once.

Use `offline-docs validate --config sources.yaml` to check a file without crawling.

---

## How private URLs stay private

**URLs in `sources.yaml` are runtime/local indexing configuration. They are
not required when building the static frontend and are not committed to Git.**

- `sources.yaml` is listed in `.gitignore`; only `sources.example.yaml` (public
  sites) is in the repository.
- The frontend build (`npm run build` → `dist/`) never reads `sources.yaml`
  or the corpus. `dist/` is identical everywhere and CI checks that it matches
  the source code.
- The private URLs appear only in files created on the machine that indexes:
  `sources.yaml` and the generated `corpus/` (both gitignored and never part
  of a release archive — `scripts/check-release.sh` enforces this).
- The portal makes no external requests (enforced by a Content-Security-Policy
  and verified by the end-to-end test), has no analytics, and loads no fonts
  or scripts from CDNs.

So the workflow on the isolated network is: copy the release package in,
create a new `sources.yaml` there with the internal URLs, index, serve.

---

## Updating and re-indexing

- **Refresh everything:** run `index-windows.cmd` / `./index-linux.sh` again.
  The new corpus replaces the old one only after it was written completely.
- **Refresh one source:** `./index-linux.sh --source my-docs` (or
  `offline-docs.exe index --source my-docs` on Windows). Other sources keep
  their previously indexed documents.
- **Add or remove a source:** edit `sources.yaml` and re-index. Sources removed
  from the configuration disappear from the corpus.
- Reload the browser page after re-indexing; the server does not need to be
  restarted. (On Windows, if the swap fails because files are in use, stop
  the server and re-run indexing.)

Each source in the portal shows when it was indexed, how many documents it
has, and whether there were errors; each document shows its original URL and
indexing time.

---

## Command-line reference

```
offline-docs index    [--config sources.yaml] [--corpus corpus] [--source ID]...
offline-docs serve    [--dist dist] [--corpus corpus] [--port 8080] [--host 127.0.0.1] [--open]
offline-docs validate [--config sources.yaml]
offline-docs search   [--corpus corpus] [--source ID] [--limit 10] QUERY...
offline-docs version
```

`search` ranks results with exactly the same algorithm as the portal (both
are tested against shared golden files), which makes it useful on headless
machines and as a starting point for future tooling.

---

## Building from source

### Go binary (Linux with Go 1.25.1, e.g. the isolated network)

Dependencies are vendored in `vendor/`, so no network access is needed:

```sh
scripts/build-go.sh                 # → bin/offline-docs; the wrapper scripts find it there
# or, explicitly:
CGO_ENABLED=0 go build -trimpath -o bin/offline-docs ./cmd/offline-docs
GOOS=windows GOARCH=amd64 go build -o bin/offline-docs.exe ./cmd/offline-docs   # cross-compile
```

A source checkout (or the GitHub "Download ZIP") already contains the built
portal in `dist/`, so `./index-linux.sh` and `./serve-linux.sh` work right
after building the binary — Node is not required.

### Frontend (only needed when changing the UI)

Requires Node 22 and access to an npm registry:

```sh
npm ci
npm run build        # type-checks and writes dist/
```

Commit the updated `dist/` together with the source change; CI fails if
`dist/` is stale.

---

## Development

```sh
npm ci
npm run dev          # Vite dev server; serves ./corpus at /corpus (set CORPUS_DIR to change)
npm run lint
npm run typecheck
npm test             # Vitest: tokenizer/ranking parity with Go, snippets, routing, UI
npm run build && scripts/verify-dist.sh

go vet ./cmd/... ./internal/...
go test -race ./cmd/... ./internal/...

npm run e2e          # full workflow against a local fixture site in Chromium (Playwright)
scripts/package-release.sh v0.0.0-dev   # build and verify both release archives in release/
```

(`./...` would also match a Go package inside `node_modules`, hence the
explicit package patterns.)

Corpus/index format changes must update [docs/corpus-format.md](docs/corpus-format.md)
and the golden files (`go test ./internal/index -run Golden -update`); the
TypeScript tests read the same golden files.

---

## Release process

1. Make sure `main` is green in CI (`CI` workflow, a single job: lint, typecheck, tests,
   stale-`dist/` check, Go tests, cross-builds, end-to-end test, package
   preview). Every CI run also uploads preview packages as a workflow artifact.
2. Update `version` in `package.json` if desired, then tag and push:

   ```sh
   git tag -a v0.1.0 -m "Offline Developer Portal v0.1.0"
   git push origin v0.1.0
   ```

3. The `Release` workflow re-runs the tests, verifies `dist/`, cross-compiles
   Windows amd64 and Linux amd64 binaries, assembles and verifies
   `offline-developer-portal-v0.1.0-windows-amd64.zip`,
   `offline-developer-portal-v0.1.0-linux-amd64.tar.gz` and `SHA256SUMS.txt`,
   and publishes them as a GitHub Release. (It can also be started manually
   from the Actions tab for an existing tag.)

Binaries are never committed to Git; they exist only in release assets and CI
artifacts.

---

## Known limitations

This is intentionally a v0.1 prototype:

- Only server-rendered HTML is indexed. Sites that render their content with
  JavaScript show little or no content (no headless browser).
- No authentication; sites behind a login cannot be indexed.
- PDFs and other non-HTML documents are skipped.
- Pages must be UTF-8 (or ASCII); legacy encodings may show garbled characters.
- The whole search index is loaded into the browser. This is fast for corpora up
  to a few thousand pages; very large corpora (tens of thousands of pages)
  will load slowly.
- Search is keyword-based (BM25 with prefix matching): no stemming beyond
  prefixes, no typo tolerance, no phrase queries, no semantic search.
- Content extraction uses heuristics; unusual page layouts may include some
  navigation text or miss content.
- No scheduled/automatic refresh; re-run indexing manually.
- Corporate HTTP proxies are used only via the `HTTPS_PROXY`/`HTTP_PROXY`
  environment variables.
- The Windows binary is not code-signed.

## Roadmap

Possible future capabilities (deliberately out of scope for v0.1):

- MCP server and richer CLI over the same corpus files (for AI assistants and scripts)
- Sharded/lazy-loaded index for Wikipedia/Stack Overflow-scale corpora
- Stemming, typo tolerance, phrase and field queries
- Semantic/vector search with locally computed embeddings
- Optional headless-browser rendering for JavaScript-heavy sites
- PDF and source-code indexing
- Authenticated sources (with secrets kept out of the corpus)
- Incremental re-crawls (ETag/Last-Modified) and scheduled refresh
- Sitemap support, per-source charset handling
- Code signing for Windows binaries
