import { afterEach, describe, expect, it, vi } from 'vitest'
import { getMonitorSnapshot } from './bindings'

const response = { schemaVersion: 2, revision: 1, observedAt: '2026-09-10T00:00:00Z', freshness: { state: 'fresh', syncStatus: 'synced' }, state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], projects: [] }

describe('monitor browser binding', () => {
  afterEach(() => { vi.restoreAllMocks(); delete (window as Window & { go?: unknown }).go })

  it('fetches the GitHub monitor endpoint when Wails is unavailable', async () => {
    const fetchMock = vi.spyOn(window, 'fetch').mockResolvedValue(new Response(JSON.stringify(response), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    await expect(getMonitorSnapshot()).resolves.toEqual(response)
    expect(fetchMock).toHaveBeenCalledWith('/api/github-monitor', expect.objectContaining({ method: 'GET', headers: { Accept: 'application/json' } }))
  })

  it('prefers the legacy Wails binding when it is available', async () => {
    const binding = vi.fn(async () => response)
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetMonitorSnapshot: binding } } }
    const fetchMock = vi.spyOn(window, 'fetch')
    await expect(getMonitorSnapshot()).resolves.toEqual(response)
    expect(binding).toHaveBeenCalledTimes(1)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
