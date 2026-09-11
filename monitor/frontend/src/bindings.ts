import type { Snapshot, SnapshotSource } from './types'

export interface MonitorSettings {
  githubHost: string
  repositories: string
  projects: string
  wslDistribution: string
  sessionsFile: string
}

type WailsAppBinding = {
  GetMonitorSnapshot?: () => Promise<Snapshot>
  GetMonitorSettings?: () => Promise<MonitorSettings>
  SaveMonitorSettings?: (settings: MonitorSettings) => Promise<MonitorSettings>
}

type WailsWindow = Window & { go?: { main?: { App?: WailsAppBinding } } }

const appBinding = () => (window as WailsWindow).go?.main?.App

export const getMonitorSnapshot: SnapshotSource = async () => {
  const binding = appBinding()?.GetMonitorSnapshot
  if (binding) return binding()
  throw new Error('Wails monitor binding is unavailable. Run the Windows Monitor application.')
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
