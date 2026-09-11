import type { Snapshot, SnapshotSource } from './types'

export interface MonitorSettings {
  githubHost: string
  repositories: string
  projects: string
  wslDistribution: string
  sessionsFile: string
}

export type ToolboxReferenceType = 'web' | 'file' | 'wsl-file'
export type ToolboxReference = { id: string; label: string; type: ToolboxReferenceType; target: string }
export type ToolboxCommand = { id: string; label: string; command: string; note?: string }
export type ToolboxChecklistItem = { id: string; text: string; done: boolean }
export type ProjectToolbox = {
  references: ToolboxReference[]
  commands: ToolboxCommand[]
  checklist: ToolboxChecklistItem[]
}

type WailsAppBinding = {
  GetMonitorSnapshot?: () => Promise<Snapshot>
  GetMonitorSnapshotAll?: () => Promise<Snapshot>
  GetMonitorSettings?: () => Promise<MonitorSettings>
  SaveMonitorSettings?: (settings: MonitorSettings) => Promise<MonitorSettings>
  GetProjectToolbox?: (projectKey: string) => Promise<ProjectToolbox>
  SaveProjectToolbox?: (projectKey: string, toolbox: ProjectToolbox) => Promise<ProjectToolbox>
  OpenToolboxReference?: (referenceType: ToolboxReferenceType, target: string) => Promise<void>
}

type WailsWindow = Window & { go?: { main?: { App?: WailsAppBinding } } }

const appBinding = () => (window as WailsWindow).go?.main?.App

export const getMonitorSnapshot: SnapshotSource = async () => {
  const binding = appBinding()?.GetMonitorSnapshot
  if (binding) return binding()
  throw new Error('Wails monitor binding is unavailable. Run the Windows Monitor application.')
}

export const getMonitorSnapshotAll: SnapshotSource = async () => {
  const binding = appBinding()?.GetMonitorSnapshotAll
  if (binding) return binding()
  throw new Error('Wails full monitor binding is unavailable. Run the Windows Monitor application.')
}

export const getMonitorSettings = async (): Promise<MonitorSettings> => {
  const binding = appBinding()?.GetMonitorSettings
  if (binding) return binding()
  throw new Error('Wails settings binding is unavailable. Run the Windows Monitor application.')
}

export const saveMonitorSettings = async (settings: MonitorSettings): Promise<MonitorSettings> => {
  const binding = appBinding()?.SaveMonitorSettings
  if (binding) return binding(settings)
  throw new Error('Wails settings binding is unavailable. Run the Windows Monitor application.')
}

export const getProjectToolbox = async (projectKey: string): Promise<ProjectToolbox> => {
  const binding = appBinding()?.GetProjectToolbox
  if (binding) return binding(projectKey)
  throw new Error('Wails project toolbox binding is unavailable. Run the Windows Monitor application.')
}

export const saveProjectToolbox = async (projectKey: string, toolbox: ProjectToolbox): Promise<ProjectToolbox> => {
  const binding = appBinding()?.SaveProjectToolbox
  if (binding) return binding(projectKey, toolbox)
  throw new Error('Wails project toolbox save binding is unavailable. Run the Windows Monitor application.')
}

export const openToolboxReference = async (referenceType: ToolboxReferenceType, target: string): Promise<void> => {
  const binding = appBinding()?.OpenToolboxReference
  if (binding) return binding(referenceType, target)
  throw new Error('Wails toolbox path binding is unavailable. Run the Windows Monitor application.')
}
