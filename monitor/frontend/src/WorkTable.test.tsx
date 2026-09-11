import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { WorkTable } from './WorkTable'
import type { WorkItem } from './types'

const item = (overrides: Partial<WorkItem> = {}): WorkItem => ({
  workId: 'issue:1', title: 'Very long issue title that should stay readable in one compact row', state: 'open',
  syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], updatedAt: '2026-09-11T04:00:00Z',
  decisions: [], handoffs: [], links: [], github: { kind: 'issue', number: 1, state: 'OPEN', checks: [], relatedIssueUrls: [], relatedPullRequestUrls: [], fields: {}, contentAvailable: true },
  ...overrides,
})

describe('work table', () => {
  afterEach(cleanup)

  it('renders number, kind, title, state and selects a row', () => {
    const onSelect = vi.fn()
    const second = item({ workId: 'pr:2', title: 'Review API change', state: 'open', github: { kind: 'pull_request', number: 2, state: 'OPEN', checks: [], relatedIssueUrls: [], relatedPullRequestUrls: [], fields: {}, contentAvailable: true } })
    render(<WorkTable items={[item(), second]} selected="issue:1" onSelect={onSelect} />)

    expect(screen.getByRole('columnheader', { name: '번호' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: '종류' })).toBeInTheDocument()
    expect(screen.getByText('#1')).toBeInTheDocument()
    expect(screen.getByText('Issue')).toBeInTheDocument()
    expect(screen.getByText('PR')).toBeInTheDocument()
    expect(screen.getByText('Very long issue title that should stay readable in one compact row')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /Review API change/ }))
    expect(onSelect).toHaveBeenCalledWith('pr:2')
  })
})
