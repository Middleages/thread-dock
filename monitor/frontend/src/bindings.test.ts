import { afterEach, describe, expect, it, vi } from 'vitest'
import { getMonitorSettings, getMonitorSnapshot, getMonitorSnapshotAll, getProjectToolbox, openToolboxReference, saveMonitorSettings, saveProjectToolbox } from './bindings'

const response = { schemaVersion: 2, revision: 1, observedAt: '2026-09-10T00:00:00Z', freshness: { state: 'fresh', syncStatus: 'synced' }, state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], projects: [] }
const settings = { githubHost: 'github.samsungds.net', repositories: 'FDYPhotoDX/thread-dock', projects: 'https://github.samsungds.net/orgs/FDYPhotoDX/projects/4', wslDistribution: 'Ubuntu', sessionsFile: '/home/appuser/.threaddock/sessions.json' }
const toolbox = { references: [{ id: 'r1', label: 'Confluence', type: 'web' as const, target: 'https://example.com/wiki' }], commands: [{ id: 'c1', label: 'psql', command: 'psql -d app' }], checklist: [{ id: 'k1', text: 'Smoke test', done: false }] }

describe('monitor browser binding', () => {
  afterEach(() => { vi.restoreAllMocks(); delete (window as Window & { go?: unknown }).go })

  it('rejects clearly without a Wails binding and never falls back to fetch', async () => {
    const fetchMock = vi.spyOn(window, 'fetch')
    await expect(getMonitorSnapshot()).rejects.toThrow('Wails monitor binding is unavailable')
    await expect(getMonitorSnapshotAll()).rejects.toThrow('Wails full monitor binding is unavailable')
    await expect(getMonitorSettings()).rejects.toThrow('Wails settings binding is unavailable')
    await expect(saveMonitorSettings(settings)).rejects.toThrow('Wails settings binding is unavailable')
    await expect(getProjectToolbox('project')).rejects.toThrow('Wails project toolbox binding is unavailable')
    await expect(saveProjectToolbox('project', toolbox)).rejects.toThrow('Wails project toolbox save binding is unavailable')
    await expect(openToolboxReference('file', 'C:\\docs\\guide.pdf')).rejects.toThrow('Wails toolbox path binding is unavailable')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('calls the Wails monitor, settings and toolbox bindings when they are available', async () => {
    const snapshotBinding = vi.fn(async () => response)
    const snapshotAllBinding = vi.fn(async () => ({ ...response, revision: 2 }))
    const getSettingsBinding = vi.fn(async () => settings)
    const saveSettingsBinding = vi.fn(async () => settings)
    const getToolboxBinding = vi.fn(async () => toolbox)
    const saveToolboxBinding = vi.fn(async () => toolbox)
    const openToolboxBinding = vi.fn(async () => undefined)
    ;(window as Window & { go?: unknown }).go = { main: { App: {
      GetMonitorSnapshot: snapshotBinding,
      GetMonitorSnapshotAll: snapshotAllBinding,
      GetMonitorSettings: getSettingsBinding,
      SaveMonitorSettings: saveSettingsBinding,
      GetProjectToolbox: getToolboxBinding,
      SaveProjectToolbox: saveToolboxBinding,
      OpenToolboxReference: openToolboxBinding,
    } } }
    const fetchMock = vi.spyOn(window, 'fetch')

    await expect(getMonitorSnapshot()).resolves.toEqual(response)
    await expect(getMonitorSnapshotAll()).resolves.toMatchObject({ revision: 2 })
    await expect(getMonitorSettings()).resolves.toEqual(settings)
    await expect(saveMonitorSettings(settings)).resolves.toEqual(settings)
    await expect(getProjectToolbox('project')).resolves.toEqual(toolbox)
    await expect(saveProjectToolbox('project', toolbox)).resolves.toEqual(toolbox)
    await expect(openToolboxReference('wsl-file', '/home/appuser/guide.md')).resolves.toBeUndefined()
    expect(snapshotBinding).toHaveBeenCalledTimes(1)
    expect(snapshotAllBinding).toHaveBeenCalledTimes(1)
    expect(getSettingsBinding).toHaveBeenCalledTimes(1)
    expect(saveSettingsBinding).toHaveBeenCalledWith(settings)
    expect(getToolboxBinding).toHaveBeenCalledWith('project')
    expect(saveToolboxBinding).toHaveBeenCalledWith('project', toolbox)
    expect(openToolboxBinding).toHaveBeenCalledWith('wsl-file', '/home/appuser/guide.md')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
