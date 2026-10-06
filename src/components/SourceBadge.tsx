const PALETTE_SIZE = 8

/** Stable color slot for a source id. */
export function sourceColor(id: string): number {
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0
  return h % PALETTE_SIZE
}

export function SourceBadge({ id, name }: { id: string; name: string }) {
  return (
    <span className={`badge badge-c${sourceColor(id)}`} title={`Source: ${name} (${id})`}>
      {name}
    </span>
  )
}
