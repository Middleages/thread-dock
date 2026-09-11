import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MarkdownBody } from './MarkdownBody'

describe('MarkdownBody', () => {
  it('renders common GitHub issue markdown as readable structure', () => {
    render(<MarkdownBody value={'## Acceptance\n\n- [x] first item\n- [ ] second item\n\n**bold** and `inline`\n\n> note\n\n```ts\nconst value = 1\n```'} />)

    expect(screen.getByRole('heading', { name: 'Acceptance', level: 2 })).toBeInTheDocument()
    const list = screen.getByRole('list')
    expect(within(list).getByText(/first item/)).toBeInTheDocument()
    expect(within(list).getByText(/second item/)).toBeInTheDocument()
    expect(screen.getByText('bold').tagName).toBe('STRONG')
    expect(screen.getByText('inline').tagName).toBe('CODE')
    expect(screen.getByText('note').closest('blockquote')).not.toBeNull()
    expect(screen.getByText('const value = 1')).toBeInTheDocument()
  })

  it('renders safe links and keeps unsafe links inert', () => {
    render(<MarkdownBody value={'[GitHub](https://github.com/acme/app) [unsafe](javascript:alert(1))'} />)

    expect(screen.getByRole('link', { name: 'GitHub' })).toHaveAttribute('href', 'https://github.com/acme/app')
    expect(screen.queryByRole('link', { name: 'unsafe' })).not.toBeInTheDocument()
    expect(screen.getByText('unsafe')).toBeInTheDocument()
  })

  it('renders a simple markdown table', () => {
    render(<MarkdownBody value={'| Field | Value |\n| --- | --- |\n| Status | In Progress |'} />)

    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Field' })).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: 'In Progress' })).toBeInTheDocument()
  })
})
