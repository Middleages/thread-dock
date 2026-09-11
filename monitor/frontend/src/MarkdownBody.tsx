import type { ReactNode } from 'react'
import { isSafeExternalURL } from './safe-url'
import './markdown-body.css'

type MarkdownBodyProps = { value: string }

const inlinePattern = /(`[^`]+`|\[[^\]]+\]\([^)]+\)|\*\*[^*]+\*\*|~~[^~]+~~|\*[^*]+\*)/g

function renderInline(value: string, keyPrefix: string): ReactNode[] {
  const result: ReactNode[] = []
  let cursor = 0
  let index = 0

  for (const match of value.matchAll(inlinePattern)) {
    const start = match.index ?? 0
    if (start > cursor) result.push(value.slice(cursor, start))
    const token = match[0]
    const key = `${keyPrefix}-${index++}`

    if (token.startsWith('`')) {
      result.push(<code key={key}>{token.slice(1, -1)}</code>)
    } else if (token.startsWith('[')) {
      const link = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(token)
      const label = link?.[1] ?? token
      const url = link?.[2] ?? ''
      result.push(isSafeExternalURL(url)
        ? <a key={key} href={url} target="_blank" rel="noreferrer">{label}</a>
        : <span key={key}>{label}</span>)
    } else if (token.startsWith('**')) {
      result.push(<strong key={key}>{token.slice(2, -2)}</strong>)
    } else if (token.startsWith('~~')) {
      result.push(<del key={key}>{token.slice(2, -2)}</del>)
    } else if (token.startsWith('*')) {
      result.push(<em key={key}>{token.slice(1, -1)}</em>)
    } else {
      result.push(token)
    }
    cursor = start + token.length
  }

  if (cursor < value.length) result.push(value.slice(cursor))
  return result
}

function splitTableRow(line: string) {
  return line.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((cell) => cell.trim())
}

function isTableSeparator(line: string) {
  const cells = splitTableRow(line)
  return cells.length > 0 && cells.every((cell) => /^:?-{3,}:?$/.test(cell))
}

function isListLine(line: string) {
  return /^\s*([-+*]|\d+\.)\s+/.test(line)
}

function isBlockStart(lines: string[], index: number) {
  const line = lines[index] ?? ''
  if (!line.trim()) return true
  if (/^```/.test(line.trim())) return true
  if (/^#{1,6}\s+/.test(line)) return true
  if (/^>\s?/.test(line)) return true
  if (isListLine(line)) return true
  if (/^\s*(-{3,}|\*{3,}|_{3,})\s*$/.test(line)) return true
  if (line.includes('|') && index + 1 < lines.length && isTableSeparator(lines[index + 1])) return true
  return false
}

export function MarkdownBody({ value }: MarkdownBodyProps) {
  const lines = value.replace(/\r\n?/g, '\n').split('\n')
  const blocks: ReactNode[] = []
  let index = 0

  while (index < lines.length) {
    const line = lines[index]
    if (!line.trim()) { index++; continue }

    const fence = /^```\s*([^\s]*)/.exec(line.trim())
    if (fence) {
      const language = fence[1]
      const code: string[] = []
      index++
      while (index < lines.length && !/^```\s*$/.test(lines[index].trim())) code.push(lines[index++])
      if (index < lines.length) index++
      blocks.push(<pre key={`code-${index}`}><code className={language ? `language-${language}` : undefined}>{code.join('\n')}</code></pre>)
      continue
    }

    if (line.includes('|') && index + 1 < lines.length && isTableSeparator(lines[index + 1])) {
      const headers = splitTableRow(line)
      index += 2
      const rows: string[][] = []
      while (index < lines.length && lines[index].includes('|') && lines[index].trim()) rows.push(splitTableRow(lines[index++]))
      blocks.push(<div className="markdown-table-wrap" key={`table-${index}`}><table><thead><tr>{headers.map((cell, cellIndex) => <th key={cellIndex}>{renderInline(cell, `th-${index}-${cellIndex}`)}</th>)}</tr></thead><tbody>{rows.map((row, rowIndex) => <tr key={rowIndex}>{headers.map((_, cellIndex) => <td key={cellIndex}>{renderInline(row[cellIndex] ?? '', `td-${index}-${rowIndex}-${cellIndex}`)}</td>)}</tr>)}</tbody></table></div>)
      continue
    }

    const heading = /^(#{1,6})\s+(.+)$/.exec(line)
    if (heading) {
      const level = heading[1].length
      const children = renderInline(heading[2], `heading-${index}`)
      if (level === 1) blocks.push(<h1 key={index}>{children}</h1>)
      else if (level === 2) blocks.push(<h2 key={index}>{children}</h2>)
      else if (level === 3) blocks.push(<h3 key={index}>{children}</h3>)
      else if (level === 4) blocks.push(<h4 key={index}>{children}</h4>)
      else if (level === 5) blocks.push(<h5 key={index}>{children}</h5>)
      else blocks.push(<h6 key={index}>{children}</h6>)
      index++
      continue
    }

    if (/^>\s?/.test(line)) {
      const quoted: string[] = []
      while (index < lines.length && /^>\s?/.test(lines[index])) quoted.push(lines[index++].replace(/^>\s?/, ''))
      blocks.push(<blockquote key={`quote-${index}`}>{quoted.map((text, quoteIndex) => <p key={quoteIndex}>{renderInline(text, `quote-${index}-${quoteIndex}`)}</p>)}</blockquote>)
      continue
    }

    if (isListLine(line)) {
      const ordered = /^\s*\d+\.\s+/.test(line)
      const items: Array<{ text: string; checked?: boolean }> = []
      while (index < lines.length && isListLine(lines[index]) && /^\s*\d+\.\s+/.test(lines[index]) === ordered) {
        const text = lines[index].replace(/^\s*(?:[-+*]|\d+\.)\s+/, '')
        const task = /^\[([ xX])\]\s+(.*)$/.exec(text)
        items.push(task ? { text: task[2], checked: task[1].toLowerCase() === 'x' } : { text })
        index++
      }
      const children = items.map((item, itemIndex) => <li key={itemIndex} className={item.checked === undefined ? undefined : 'task-list-item'}>{item.checked !== undefined && <input type="checkbox" checked={item.checked} readOnly tabIndex={-1} aria-hidden="true" />}{renderInline(item.text, `list-${index}-${itemIndex}`)}</li>)
      blocks.push(ordered ? <ol key={`list-${index}`}>{children}</ol> : <ul key={`list-${index}`}>{children}</ul>)
      continue
    }

    if (/^\s*(-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
      blocks.push(<hr key={`hr-${index}`} />)
      index++
      continue
    }

    const paragraph: string[] = [line.trim()]
    index++
    while (index < lines.length && !isBlockStart(lines, index)) {
      paragraph.push(lines[index].trim())
      index++
    }
    blocks.push(<p key={`p-${index}`}>{renderInline(paragraph.join(' '), `p-${index}`)}</p>)
  }

  return <div className="markdown-body">{blocks}</div>
}
