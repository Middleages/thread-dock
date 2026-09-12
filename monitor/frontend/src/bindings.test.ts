import { afterEach, describe, expect, it, vi } from 'vitest'
import { getGlobalToolbox, getMonitorSettings, getMonitorSnapshot, getMonitorSnapshotAll, getProjectReferences, openToolboxReference, saveGlobalToolbox, saveMonitorSettings, saveProjectReferences } from './bindings'

const response = { schemaVersion: 2, revision: 1, observedAt: '2026-09-10T00:00:00Z', freshness: { state: 'fresh', syncStatus: 'synced' }, state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], projects: [] }
const settings = { githubHost: 'github.samsungds.net', repositories: 'FDYPhotoDX/thread-dock', projects: 'https://github.samsungds.net/orgs/FDYPhotoDX/projects/4', wslDistribution: 'Ubuntu', sessionsFile: '/home/appuser/.threaddock/sessions.json' }
const refs = { references: [{ id: 'r1', label: 'Confluence', type: 'web' as const, target: 'https://example.com/wiki' }] }
const toolbox = { commands: [{ id: 'c1', label: 'psql', command: 'psql -d app' }], todos: [{ id: 't1', text: 'Smoke test', done: false }] }

describe('monitor browser binding', () => {
  afterEach(() => { vi.restoreAllMocks(); delete (window as Window & { go?: unknown }).go })

  it('rejects clearly without a Wails binding and never falls back to fetch', async () => {
    const fetchMock = vi.spyOn(window, 'fetch')
    await expect(getMonitorSnapshot()).rejects.toThrow('Wails monitor binding is unavailable')
    await expect(getMonitorSnapshotAll()).rejects.toThrow('Wails full monitor binding is unavailable')
    await expect(getMonitorSettings()).rejects.toThrow('Wails settings binding is unavailable')
    await expect(saveMonitorSettings(settings)).rejects.toThrow('Wails settings binding is unavailable')
    await expect(getProjectReferences('project')).rejects.toThrow('Wails project references binding is unavailable')
    await expect(saveProjectReferences('project', refs)).rejects.toThrow('Wails project references save binding is unavailable')
    await expect(getGlobalToolbox()).rejects.toThrow('Wails global toolbox binding is unavailable')
    await expect(saveGlobalToolbox(toolbox)).rejects.toThrow('Wails global toolbox save binding is unavailable')
    await expect(openToolboxReference('file', 'C:\\docs\\guide.pdf')).rejects.toThrow('Wails toolbox path binding is unavailable')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('calls the Wails monitor, settings and new toolbox bindings when available', async () => {
    const snapshotBinding = vi.fn(async () => response)
    const snapshotAllBinding = vi.fn(async () => ({ ...response, revision: 2 }))
    const getSettingsBinding = vi.fn(async () => settings)
    const saveSettingsBinding = vi.fn(async () => settings)
    const getReferencesBinding = vi.fn(async () => refs)
    const saveReferencesBinding = vi.fn(async () => refs)
    const getGlobalBinding = vi.fn(async () => toolbox)
    const saveGlobalBinding = vi.fn(async () => toolbox)
    const openToolboxBinding = vi.fn(async () => undefined)
    ;(window as Window & { go?: unknown }).go = { main: { App: {
      GetMonitorSnapshot: snapshotBinding,
      GetMonitorSnapshotAll: snapshotAllBinding,
      GetMonitorSettings: getSettingsBinding,
      SaveMonitorSettings: saveSettingsBinding,
      GetProjectReferences: getReferencesBinding,
      SaveProjectReferences: saveReferencesBinding,
      GetGlobalToolbox: getGlobalBinding,
      SaveGlobalToolbox: saveGlobalBinding,
      OpenToolboxReference: openToolboxBinding,
    } } }
    const fetchMock = vi.spyOn(window, 'fetch')

    await expect(getMonitorSnapshot()).resolves.toEqual(response)
    await expect(getMonitorSnapshotAll()).resolves.toMatchObject({ revision: 2 })
    await expect(getMonitorSettings()).resolves.toEqual(settings)
    await expect(saveMonitorSettings(settings)).resolves.toEqual(settings)
    await expect(getProjectReferences('github.example/repo:acme/app')).resolves.toEqual(refs)
    await expect(saveProjectReferences('github.example/repo:acme/app', refs)).resolves.toEqual(refs)
    await expect(getGlobalToolbox()).resolves.toEqual(toolbox)
    await expect(saveGlobalToolbox(toolbox)).resolves.toEqual(toolbox)
    await expect(openToolboxReference('wsl-file', '/home/appuser/guide.md')).resolves.toBeUndefined()
    expect(snapshotBinding).toHaveBeenCalledTimes(1)
    expect(snapshotAllBinding).toHaveBeenCalledTimes(1)
    expect(getSettingsBinding).toHaveBeenCalledTimes(1)
    expect(saveSettingsBinding).toHaveBeenCalledWith(settings)
    expect(getReferencesBinding).toHaveBeenCalledWith('github.example/repo:acme/app')
    expect(saveReferencesBinding).toHaveBeenCalledWith('github.example/repo:acme/app', refs)
    expect(getGlobalBinding).toHaveBeenCalledTimes(1)
    expect(saveGlobalBinding).toHaveBeenCalledWith(toolbox)
    expect(openToolboxBinding).toHaveBeenCalledWith('wsl-file', '/home/appuser/guide.md')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
