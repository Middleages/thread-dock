import type { Snapshot, SnapshotSource } from './types'

type WailsWindow = Window & { go?: { main?: { App?: { GetMonitorSnapshot?: () => Promise<Snapshot> } } } }

export const getMonitorSnapshot: SnapshotSource = async () => {
  const binding = (window as WailsWindow).go?.main?.App?.GetMonitorSnapshot
  if (!binding) throw new Error('Wails monitor binding is unavailable')
  return binding()
}
