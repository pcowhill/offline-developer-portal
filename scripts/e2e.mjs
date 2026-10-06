#!/usr/bin/env node
/* global document, window -- used inside page.evaluate() callbacks, which run in the browser */
// End-to-end test of the complete workflow, using a local fixture website:
//
//   1. start a small documentation website on 127.0.0.1;
//   2. run `offline-docs index` against it (two sources);
//   3. shut the website down, so the original source is unavailable;
//   4. run `offline-docs serve` with the committed dist/;
//   5. drive the portal in Chromium, blocking every request that does not
//      go to the local portal server, and check search, source filtering,
//      document reading and provenance.
//
// Usage: npm run e2e            (builds bin/offline-docs with Go if needed)
//        OFFLINE_DOCS_BIN=path npm run e2e
// Screenshots are written to test-results/e2e/.

import { execFileSync, spawn } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import http from 'node:http'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const shots = path.join(root, 'test-results', 'e2e')
mkdirSync(shots, { recursive: true })

function check(cond, message) {
  if (!cond) throw new Error('E2E check failed: ' + message)
  console.log('  ✓ ' + message)
}

// ---------- 1. fixture website ----------

const page = (title, body) => `<!doctype html><html><head><title>${title} - Fixture Docs</title>
<meta name="description" content="${title} page of the fixture documentation."></head>
<body><nav class="sidebar"><a href="index.html">Home</a> <a href="install.html">Install</a> <a href="api.html">API</a></nav>
<main>${body}</main><footer>Fixture footer</footer></body></html>`

const pages = {
  '/alpha/index.html': page('Alpha Home', `<h1>Alpha toolkit</h1><p>The alpha toolkit builds <strong>quantum widgets</strong>. Start with the <a href="install.html#requirements">installation guide</a>.</p>`),
  '/alpha/install.html': page('Installing Alpha', `<h1>Installing Alpha</h1>
<h2 id="requirements">Requirements</h2><p>You need a flux capacitor and at least 1.21 gigawatts.</p>
<h2 id="steps">Steps</h2><pre class="language-sh">$ alpha install --with-flux
$ alpha doctor</pre><p>Then verify the zorblax configuration.</p>`),
  '/alpha/api.html': page('Alpha API', `<h1>Alpha API reference</h1><h2 id="new">alpha.New</h2>
<pre><code class="language-go">func New(cfg Config) (*Client, error)</code></pre><p>New returns a client for zorblax services.</p>
<table><tr><th>Option</th><th>Default</th></tr><tr><td>Timeout</td><td>30s</td></tr></table>`),
  '/beta/index.html': page('Beta Home', `<h1>Beta service</h1><p>Beta stores zorblax records durably.</p><p><a href="ops.html">Operations</a></p>`),
  '/beta/ops.html': page('Beta Operations', `<h1>Operating Beta</h1><h2 id="backup">Backups</h2><p>Run nightly zorblax backups with <code>beta backup --all</code>.</p>`),
}

const site = http.createServer((req, res) => {
  const p = new URL(req.url, 'http://x').pathname
  if (p === '/robots.txt') return res.end('User-agent: *\nDisallow: /alpha/private/\n')
  const body = pages[p]
  if (!body) {
    res.statusCode = 404
    return res.end('not found')
  }
  res.setHeader('Content-Type', 'text/html; charset=utf-8')
  res.end(body)
})
await new Promise((r) => site.listen(0, '127.0.0.1', r))
const siteURL = `http://127.0.0.1:${site.address().port}`
console.log('Fixture site at', siteURL)

// ---------- 2. index ----------

let bin = process.env.OFFLINE_DOCS_BIN
if (!bin) {
  bin = path.join(root, 'bin', process.platform === 'win32' ? 'offline-docs.exe' : 'offline-docs')
  if (!existsSync(bin)) {
    console.log('Building offline-docs...')
    execFileSync('go', ['build', '-o', bin, './cmd/offline-docs'], { cwd: root, stdio: 'inherit' })
  }
}

const work = mkdtempSync(path.join(os.tmpdir(), 'odp-e2e-'))
const cfg = path.join(work, 'sources.yaml')
const corpus = path.join(work, 'corpus')
writeFileSync(
  cfg,
  `settings:
  request_delay_ms: 0
sources:
  - id: alpha
    name: Alpha Toolkit
    start_urls: [${siteURL}/alpha/index.html]
    allowed_prefixes: [${siteURL}/alpha/]
    max_pages: 20
  - id: beta
    name: Beta Service
    start_urls: [${siteURL}/beta/index.html]
    allowed_prefixes: [${siteURL}/beta/]
    max_pages: 20
`,
)

console.log('\n[1/3] Indexing')
// Must run asynchronously: the fixture site lives in this process.
await new Promise((resolve, reject) => {
  const child = spawn(bin, ['index', '--config', cfg, '--corpus', corpus], { stdio: 'inherit' })
  child.on('error', reject)
  child.on('exit', (code) => (code === 0 ? resolve() : reject(new Error('indexing failed with exit code ' + code))))
})
check(existsSync(path.join(corpus, 'manifest.json')), 'indexing completed and wrote a manifest')

// ---------- 3. take the original site offline ----------

await new Promise((r) => site.close(r))
let siteDown = false
try {
  await fetch(siteURL + '/alpha/index.html')
} catch {
  siteDown = true
}
check(siteDown, 'original documentation site is no longer reachable')

// ---------- 4. serve ----------

console.log('\n[2/3] Serving')
const server = spawn(bin, ['serve', '--dist', path.join(root, 'dist'), '--corpus', corpus, '--port', '0'], {
  stdio: ['ignore', 'pipe', 'inherit'],
})
const portalURL = await new Promise((resolve, reject) => {
  let out = ''
  const timer = setTimeout(() => reject(new Error('server did not start:\n' + out)), 15000)
  server.stdout.on('data', (d) => {
    out += d
    process.stdout.write(d)
    const m = /http:\/\/127\.0\.0\.1:(\d+)\//.exec(out)
    if (m) {
      clearTimeout(timer)
      resolve(m[0])
    }
  })
  server.on('exit', (code) => reject(new Error('server exited with ' + code)))
})

// ---------- 5. browser ----------

console.log('\n[3/3] Browser checks against', portalURL)
const browser = await chromium.launch()
let failed = false
try {
  for (const scheme of ['light', 'dark']) {
    const context = await browser.newContext({ viewport: { width: 1280, height: 860 }, colorScheme: scheme })
    const blocked = []
    await context.route('**/*', (route) => {
      const u = route.request().url()
      if (u.startsWith(portalURL)) return route.continue()
      blocked.push(u)
      return route.abort()
    })
    const p = await context.newPage()
    const errors = []
    p.on('pageerror', (e) => errors.push(e.message))
    p.on('console', (m) => m.type() === 'error' && errors.push(m.text()))

    await p.goto(portalURL)
    await p.getByText('Indexed sources').waitFor()
    check((await p.locator('.source-card').count()) === 2, `[${scheme}] home lists both indexed sources`)
    check(await p.getByText(/5 documents · updated/).isVisible(), `[${scheme}] corpus generation time and size shown`)
    await p.screenshot({ path: path.join(shots, `home-${scheme}.png`), fullPage: true })

    await p.keyboard.press('/')
    await p.keyboard.type('zorblax')
    await p.getByText(/4 results/).waitFor()
    check(true, `[${scheme}] search for "zorblax" finds 4 documents in both sources`)
    const first = p.locator('.result').first()
    check((await first.locator('.result-url').textContent()).includes('127.0.0.1'), `[${scheme}] results show original URL (provenance)`)
    await p.waitForFunction(() => document.querySelectorAll('.result-snippet mark').length >= 4)
    check(true, `[${scheme}] snippets highlight the query`)
    await p.screenshot({ path: path.join(shots, `results-${scheme}.png`) })

    await p.getByRole('checkbox', { name: /Beta Service/ }).check()
    await p.getByText(/2 results/).waitFor()
    const badges = await p.locator('.result .badge').allTextContents()
    check(badges.length === 2 && badges.every((b) => b === 'Beta Service'), `[${scheme}] source filter restricts results to Beta Service`)
    await p.getByRole('checkbox', { name: /Beta Service/ }).uncheck()
    await p.getByText(/4 results/).waitFor()

    await p.locator('input[type=search]').fill('flux capacitor')
    await p.getByText(/1 result\b/).waitFor()
    await p.keyboard.press('Enter')
    await p.locator('.doc-body').waitFor()
    check(await p.getByRole('heading', { level: 1, name: 'Installing Alpha' }).isVisible(), `[${scheme}] document opens locally`)
    check(await p.locator('.provenance').getByText(siteURL + '/alpha/install.html').isVisible(), `[${scheme}] document shows original URL`)
    check(await p.locator('.provenance').getByText('Alpha Toolkit').isVisible(), `[${scheme}] document shows source name`)
    check((await p.locator('.provenance time').count()) === 1, `[${scheme}] document shows indexed timestamp`)
    check((await p.locator('.code-block pre').first().textContent()).includes('alpha install --with-flux'), `[${scheme}] code block rendered`)
    check((await p.locator('.toc a').count()) >= 2, `[${scheme}] table of contents rendered`)
    await p.screenshot({ path: path.join(shots, `document-${scheme}.png`) })

    await p.getByRole('link', { name: /Back to results/ }).click()
    await p.getByText(/1 result\b/).waitFor()
    check((await p.locator('input[type=search]').inputValue()) === 'flux capacitor', `[${scheme}] back navigation restores the search`)

    // Links between indexed pages stay local.
    await p.locator('input[type=search]').fill('quantum')
    await p.locator('.result-link').first().click()
    await p.getByRole('link', { name: 'installation guide' }).click()
    await p.getByRole('heading', { level: 1, name: 'Installing Alpha' }).waitFor()
    check(p.url().startsWith(portalURL), `[${scheme}] internal documentation links open the local copy`)

    check(blocked.length === 0, `[${scheme}] no request left the local portal (${blocked.join(', ') || 'none'})`)
    check(errors.length === 0, `[${scheme}] no browser errors (${errors.join('; ') || 'none'})`)
    await context.close()
  }

  const mobile = await browser.newPage({ viewport: { width: 390, height: 844 } })
  await mobile.goto(portalURL + '#/?q=zorblax')
  await mobile.getByText(/4 results/).waitFor()
  const overflow = await mobile.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)
  check(!overflow, 'mobile layout has no horizontal overflow')
  await mobile.screenshot({ path: path.join(shots, 'mobile.png') })
} catch (e) {
  failed = true
  console.error(e)
} finally {
  await browser.close()
  server.kill()
  rmSync(work, { recursive: true, force: true })
}

if (failed) process.exit(1)
console.log(`\nEnd-to-end test passed. Screenshots: ${path.relative(root, shots)}`)
