import { highlight } from '../search/snippet'

/** Renders text with query-term matches wrapped in <mark>. */
export function Highlighted({ text, terms }: { text: string; terms: readonly string[] }) {
  if (terms.length === 0) return <>{text}</>
  return (
    <>
      {highlight(text, terms).map((p, i) => (p.hit ? <mark key={i}>{p.text}</mark> : <span key={i}>{p.text}</span>))}
    </>
  )
}
