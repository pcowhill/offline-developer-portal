import { useState, type ReactNode } from 'react'
import type { LinkTarget } from '../corpus/portal'
import type { Block, Span } from '../corpus/types'
import { docHref } from '../router'
import { spansText } from '../search/snippet'
import { Highlighted } from './Highlighted'
import { ExternalIcon } from './Icons'

export interface RenderContext {
  terms: readonly string[]
  resolveLink: (href: string) => LinkTarget | undefined
  query: string
  sources: readonly string[]
}

/** DOM id for a heading anchor; prefixed so it cannot clash with the app. */
export function headingDomId(anchor: string): string {
  return 'sec-' + anchor
}

export function Blocks({ blocks, ctx }: { blocks: Block[]; ctx: RenderContext }) {
  return <>{blocks.map((b, i) => <BlockView key={i} block={b} ctx={ctx} />)}</>
}

function BlockView({ block, ctx }: { block: Block; ctx: RenderContext }) {
  switch (block.type) {
    case 'heading': {
      const level = Math.min(Math.max(block.level ?? 2, 1), 6)
      const Tag = `h${level}` as 'h1'
      return (
        <Tag id={block.anchor ? headingDomId(block.anchor) : undefined} className="doc-heading">
          <Highlighted text={spansText(block.spans)} terms={ctx.terms} />
        </Tag>
      )
    }
    case 'paragraph':
      return (
        <p>
          <Spans spans={block.spans ?? []} ctx={ctx} />
        </p>
      )
    case 'code':
      return <CodeBlock text={block.text ?? ''} lang={block.lang} />
    case 'list': {
      const items = (block.items ?? []).map((item, i) => (
        <li key={i}>
          <Blocks blocks={item} ctx={ctx} />
        </li>
      ))
      return block.ordered ? <ol>{items}</ol> : <ul>{items}</ul>
    }
    case 'quote':
      return (
        <blockquote>
          <Blocks blocks={block.children ?? []} ctx={ctx} />
        </blockquote>
      )
    case 'table':
      return <TableBlock rows={block.rows ?? []} header={!!block.header} ctx={ctx} />
    default:
      return null
  }
}

function Spans({ spans, ctx }: { spans: Span[]; ctx: RenderContext }) {
  return (
    <>
      {spans.map((s, i) => {
        let node: ReactNode = s.c ? <code>{s.t}</code> : <Highlighted text={s.t} terms={ctx.terms} />
        if (s.b) node = <strong>{node}</strong>
        if (s.i) node = <em>{node}</em>
        if (s.h) node = <DocLink href={s.h} ctx={ctx}>{node}</DocLink>
        return <span key={i}>{node}</span>
      })}
    </>
  )
}

/** Links to indexed pages stay inside the portal; others are marked as external. */
function DocLink({ href, ctx, children }: { href: string; ctx: RenderContext; children: ReactNode }) {
  if (!/^https?:\/\//i.test(href)) return <>{children}</>
  const target = ctx.resolveLink(href)
  if (target) {
    return (
      <a href={docHref(target.id, { anchor: target.anchor, q: ctx.query, sources: ctx.sources })} className="local-link">
        {children}
      </a>
    )
  }
  return (
    <a href={href} className="external-link" target="_blank" rel="noopener noreferrer" title={`${href} (not in the offline corpus; needs network access)`}>
      {children}
      <ExternalIcon size={10} />
    </a>
  )
}

function CodeBlock({ text, lang }: { text: string; lang?: string }) {
  const [copied, setCopied] = useState(false)
  const copy = () => {
    navigator.clipboard
      ?.writeText(text)
      .then(() => {
        setCopied(true)
        window.setTimeout(() => setCopied(false), 1500)
      })
      .catch(() => {})
  }
  return (
    <div className="code-block">
      <div className="code-head">
        <span className="code-lang">{lang || 'code'}</span>
        <button type="button" className="copy-button" onClick={copy} aria-label="Copy code to clipboard">
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <pre tabIndex={0}>
        <code>{text}</code>
      </pre>
    </div>
  )
}

function TableBlock({ rows, header, ctx }: { rows: string[][]; header: boolean; ctx: RenderContext }) {
  const head = header ? rows[0] : undefined
  const body = header ? rows.slice(1) : rows
  return (
    <div className="table-wrap" tabIndex={0}>
      <table>
        {head && (
          <thead>
            <tr>
              {head.map((c, i) => (
                <th key={i}>
                  <Highlighted text={c} terms={ctx.terms} />
                </th>
              ))}
            </tr>
          </thead>
        )}
        <tbody>
          {body.map((r, i) => (
            <tr key={i}>
              {r.map((c, j) => (
                <td key={j}>
                  <Highlighted text={c} terms={ctx.terms} />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
