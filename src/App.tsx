import { useEffect, useState } from 'react'
import { CorpusMissingError, loadCorpus } from './corpus/load'
import { Portal } from './corpus/portal'
import { DocumentPage } from './components/DocumentPage'
import { Header } from './components/Header'
import { SearchPage } from './components/SearchPage'
import { ErrorView, LoadingView, MissingCorpusView } from './components/StatusViews'
import { useRoute } from './router'

type LoadState =
  | { status: 'loading' }
  | { status: 'missing' }
  | { status: 'error'; message: string }
  | { status: 'ready'; portal: Portal }

export default function App() {
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [route, routeKey] = useRoute()

  useEffect(() => {
    let cancelled = false
    loadCorpus()
      .then((corpus) => !cancelled && setState({ status: 'ready', portal: new Portal(corpus) }))
      .catch((e: unknown) => {
        if (cancelled) return
        if (e instanceof CorpusMissingError) setState({ status: 'missing' })
        else setState({ status: 'error', message: e instanceof Error ? e.message : String(e) })
      })
    return () => {
      cancelled = true
    }
  }, [])

  let content
  switch (state.status) {
    case 'loading':
      content = <LoadingView />
      break
    case 'missing':
      content = <MissingCorpusView />
      break
    case 'error':
      content = <ErrorView message={state.message} />
      break
    case 'ready':
      content =
        route.name === 'doc' ? (
          <DocumentPage
            key={route.id}
            portal={state.portal}
            id={route.id}
            query={route.q}
            sources={route.sources}
            anchor={route.anchor}
          />
        ) : (
          <SearchPage key={routeKey} portal={state.portal} initialQuery={route.q} initialSources={route.sources} />
        )
  }

  return (
    <div className="app">
      <a className="skip-link" href="#main" onClick={(e) => {
        e.preventDefault()
        document.getElementById('main')?.focus()
      }}>
        Skip to content
      </a>
      <Header manifest={state.status === 'ready' ? state.portal.manifest : undefined} />
      <main id="main" tabIndex={-1}>
        {content}
      </main>
      <footer className="site-footer">
        Served locally · no network access needed after indexing
        {state.status === 'ready' && <> · {state.portal.manifest.generator}</>}
      </footer>
    </div>
  )
}
