import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'
import type { Snapshot } from './types'

const snapshot = (revision: number, title: string): Snapshot => ({
  schemaVersion: 2,
  revision,
  observedAt: '2026-09-11T04:00:00Z',
  freshness: { state: 'fresh', syncStatus: 'synced' },
  state: 'running',
  syncStatus: 'synced',
  nextAction: 'review',
  evidenceRefs: [],
  source: 'github',
  notices: [],
  projects: [{
    projectId: 'repo:acme/app', name: 'acme/app', state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [],
    workItems: [{ workId: `issue:${revision}`, title, state: revision === 1 ? 'open' : 'closed', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], decisions: [], handoffs: [], links: [], github: { kind: 'issue', number: revision, state: revision === 1 ? 'OPEN' : 'CLOSED', checks: [], relatedIssueUrls: [], relatedPullRequestUrls: [], fields: {}, contentAvailable: true } }],
    notices: [], links: [], source: 'github',
  }],
})

describe('work history scope', () => {
  afterEach(() => { cleanup(); vi.useRealTimers(); delete (window as Window & { go?: unknown }).go })

  it('loads open work first and calls the full source only after 전체 보기', async () => {
    const openSource = vi.fn(async () => snapshot(1, 'Open work'))
    const allSource = vi.fn(async () => snapshot(2, 'Closed history'))
    render(<App snapshotSource={openSource} allSnapshotSource={allSource} pollIntervalMs={60_000} />)

    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(openSource).toHaveBeenCalledTimes(1)
    expect(allSource).not.toHaveBeenCalled()
    expect(screen.getByText('Open work')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '작업 현황' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '전체 보기' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(allSource).toHaveBeenCalledTimes(1)
    expect(screen.getByText('Closed history')).toBeInTheDocument()
  })

  it('describes repository health as 조회 상태 instead of a synthetic workflow stage', async () => {
    render(<App snapshotSource={vi.fn(async () => snapshot(1, 'Open work'))} />)

    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('columnheader', { name: '조회 상태' })).toBeInTheDocument()
    expect(screen.queryByRole('columnheader', { name: '현재 단계' })).not.toBeInTheDocument()
    expect(screen.queryByRole('columnheader', { name: '다음 행동' })).not.toBeInTheDocument()
  })

  it('opens a human-only project Toolbox without passing it into agent state', async () => {
    const getToolbox = vi.fn(async () => ({ references: [], commands: [], checklist: [] }))
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetProjectToolbox: getToolbox } } }
    render(<App snapshotSource={vi.fn(async () => snapshot(1, 'Open work'))} />)

    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('tab', { name: 'Toolbox' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })

    expect(screen.getByRole('heading', { name: 'Project Toolbox' })).toBeInTheDocument()
    expect(screen.getByText('Agent에 자동 전달하지 않음')).toBeInTheDocument()
    expect(getToolbox).toHaveBeenCalledWith('repo:acme/app')
  })

  it('does not keep polling closed history after the user opens 전체 보기', async () => {
    vi.useFakeTimers()
    const openSource = vi.fn(async () => snapshot(1, 'Open work'))
    const allSource = vi.fn(async () => snapshot(2, 'Closed history'))
    render(<App snapshotSource={openSource} allSnapshotSource={allSource} pollIntervalMs={4000} />)

    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '전체 보기' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(allSource).toHaveBeenCalledTimes(1)

    await act(async () => { await vi.advanceTimersByTimeAsync(12000) })
    expect(allSource).toHaveBeenCalledTimes(1)
  })
})
