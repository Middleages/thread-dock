import { afterEach, describe, expect, it, vi } from 'vitest'
import { getMonitorSettings, getMonitorSnapshot, saveMonitorSettings } from './bindings'

const response = { schemaVersion: 2, revision: 1, observedAt: '2026-09-10T00:00:00Z', freshness: { state: 'fresh', syncStatus: 'synced' }, state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], projects: [] }
const settings = { repositories: 'Middleages/thread-dock', projects: 'https://github.com/users/Middleages/projects/1', wslDistribution: 'Ubuntu', sessionsFile: '/home/appuser/.threaddock/sessions.json' }

describe('monitor browser binding', () => {
  afterEach(() => { vi.restoreAllMocks(); delete (window as Window & { go?: unknown }).go })

  it('rejects clearly without a Wails binding and never falls back to fetch', async () => {
    const fetchMock = vi.spyOn(window, 'fetch')
    await expect(getMonitorSnapshot()).rejects.toThrow('Wails monitor binding is unavailable')
    await expect(getMonitorSettings()).rejects.toThrow('Wails settings binding is unavailable')
    await expect(saveMonitorSettings(settings)).rejects.toThrow('Wails settings binding is unavailable')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('calls the Wails monitor and settings bindings when they are available', async () => {
    const snapshotBinding = vi.fn(async () => response)
    const getSettingsBinding = vi.fn(async () => settings)
    const saveSettingsBinding = vi.fn(async () => settings)
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetMonitorSnapshot: snapshotBinding, GetMonitorSettings: getSettingsBinding, SaveMonitorSettings: saveSettingsBinding } } }
    const fetchMock = vi.spyOn(window, 'fetch')

    await expect(getMonitorSnapshot()).resolves.toEqual(response)
    await expect(getMonitorSettings()).resolves.toEqual(settings)
    await expect(saveMonitorSettings(settings)).resolves.toEqual(settings)
    expect(snapshotBinding).toHaveBeenCalledTimes(1)
    expect(getSettingsBinding).toHaveBeenCalledTimes(1)
    expect(saveSettingsBinding).toHaveBeenCalledWith(settings)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
