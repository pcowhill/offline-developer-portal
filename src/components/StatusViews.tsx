export function LoadingView() {
  return (
    <div className="status-view" role="status">
      <div className="spinner" aria-hidden="true" />
      <p>Loading the local search index…</p>
    </div>
  )
}

export function MissingCorpusView() {
  return (
    <div className="status-view">
      <h1>No documentation has been indexed yet</h1>
      <p>The portal is running, but it could not find a generated corpus. Indexing runs once, while the documentation sites are reachable; afterwards the portal works completely offline.</p>
      <ol className="steps">
        <li>
          Copy <code>sources.example.yaml</code> to <code>sources.yaml</code> and list the documentation sites to index.
        </li>
        <li>
          Run <code>index-windows.cmd</code> (Windows) or <code>./index-linux.sh</code> (Linux) and wait for it to finish.
        </li>
        <li>Reload this page.</li>
      </ol>
    </div>
  )
}

export function ErrorView({ message }: { message: string }) {
  return (
    <div className="status-view">
      <h1>The corpus could not be loaded</h1>
      <p className="error-text">{message}</p>
      <p>Try re-running the indexer, then reload this page.</p>
    </div>
  )
}
