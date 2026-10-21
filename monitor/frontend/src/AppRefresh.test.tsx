import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'
import type { Snapshot } from './types'

const healthySnapshot = (): Snapshot => ({
  schemaVersion: 2,
  revision: 1,
  observedAt: '2026-09-14T01:00:00Z',
  freshness: { state: 'fresh', syncStatus: 'synced' },
  state: 'running',
  syncStatus: 'synced',
  nextAction: 'review',
  evidenceRefs: [],
  source: 'github',
  notices: [],
  projects: [],
  herdr: {
    source: 'herdr',
    schemaVersion: 1,
    revision: 1,
    observedAt: '2026-09-14T01:00:00Z',
    status: 'fresh',
    state: 'running',
    syncStatus: 'synced',
    freshness: { state: 'fresh', syncStatus: 'synced' },
    notices: [],
    sessions: [],
    connections: [],
    unconnectedAgents: [],
    links: [],
  },
})

describe('manual refresh state', () => {
  afterEach(() => {
    cleanup()
    delete (window as Window & { go?: unknown }).go
  })

  it('disables the refresh button and labels the in-flight refresh', async () => {
    let resolveRefresh!: (value: Snapshot) => void
    const source = vi.fn()
      .mockResolvedValueOnce(healthySnapshot())
      .mockImplementationOnce(() => new Promise<Snapshot>((resolve) => { resolveRefresh = resolve }))
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetGlobalToolbox: vi.fn(async () => ({ commands: [], todos: [] })) } } }

    render(<App snapshotSource={source} pollIntervalMs={60_000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })

    const refresh = screen.getByRole('button', { name: 'GitHub 새로고침' })
    fireEvent.click(refresh)

    expect(screen.getByRole('button', { name: '새로고침 중...' })).toBeDisabled()
    expect(source).toHaveBeenCalledTimes(2)

    await act(async () => { resolveRefresh(healthySnapshot()); await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('button', { name: 'GitHub 새로고침' })).toBeEnabled()
  })

  it('restores the refresh button when settings supersede an in-flight manual refresh', async () => {
    let resolveManual!: (value: Snapshot) => void
    let resolveSettings!: (value: Snapshot) => void
    const source = vi.fn()
      .mockResolvedValueOnce(healthySnapshot())
      .mockImplementationOnce(() => new Promise<Snapshot>((resolve) => { resolveManual = resolve }))
      .mockImplementationOnce(() => new Promise<Snapshot>((resolve) => { resolveSettings = resolve }))
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetGlobalToolbox: vi.fn(async () => ({ commands: [], todos: [] })) } } }

    const view = render(<App snapshotSource={source} settingsEpoch={0} pollIntervalMs={60_000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: 'GitHub 새로고침' }))
    expect(screen.getByRole('button', { name: '새로고침 중...' })).toBeDisabled()

    view.rerender(<App snapshotSource={source} settingsEpoch={1} pollIntervalMs={60_000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    await act(async () => { resolveSettings(healthySnapshot()); await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('button', { name: 'GitHub 새로고침' })).toBeEnabled()
    await act(async () => { resolveManual(healthySnapshot()); await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('button', { name: 'GitHub 새로고침' })).toBeEnabled()
  })

  it('keeps Herdr healthy when only GitHub is degraded', async () => {
    const snapshot = healthySnapshot()
    snapshot.freshness = { state: 'stale', syncStatus: 'degraded' }
    snapshot.state = 'stale'
    snapshot.syncStatus = 'degraded'
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetGlobalToolbox: vi.fn(async () => ({ commands: [], todos: [] })) } } }

    render(<App snapshotSource={vi.fn(async () => snapshot)} pollIntervalMs={60_000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })

    expect(screen.getByText('GitHub 확인 필요')).toBeInTheDocument()
    expect(screen.getByText('Herdr 정상')).toBeInTheDocument()
    expect(screen.queryByText(/로컬 연결/)).not.toBeInTheDocument()
  })
})
