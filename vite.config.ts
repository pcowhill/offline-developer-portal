import { createReadStream, statSync } from 'node:fs'
import path from 'node:path'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

/**
 * During `npm run dev`, serve a locally generated corpus at /corpus/ (the
 * Go server does this in production). Set CORPUS_DIR to point elsewhere.
 * The corpus is never bundled into dist/.
 */
function devCorpus(): Plugin {
  const root = path.resolve(process.env.CORPUS_DIR ?? 'corpus')
  return {
    name: 'offline-docs-dev-corpus',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use('/corpus', (req, res, next) => {
        const rel = decodeURIComponent((req.url ?? '/').split('?')[0])
        const file = path.resolve(root, '.' + rel)
        if (!file.startsWith(root + path.sep)) return next()
        try {
          if (!statSync(file).isFile()) return next()
        } catch {
          res.statusCode = 404
          res.end('not found')
          return
        }
        res.setHeader('Content-Type', 'application/json; charset=utf-8')
        createReadStream(file).pipe(res)
      })
    },
  }
}

export default defineConfig({
  // Relative asset URLs so dist/ works from any path.
  base: './',
  plugins: [react(), devCorpus()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
    // Keep output deterministic and self-contained.
    assetsInlineLimit: 0,
  },
})
