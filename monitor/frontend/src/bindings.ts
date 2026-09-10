import type { Snapshot, SnapshotSource } from './types'

type WailsWindow = Window & { go?: { main?: { App?: { GetMonitorSnapshot?: () => Promise<Snapshot> } } } }

export const getMonitorSnapshot: SnapshotSource = async () => {
  const binding = (window as WailsWindow).go?.main?.App?.GetMonitorSnapshot
  if (binding) return binding()
  const response = await fetch('/api/github-monitor', { method: 'GET', headers: { Accept: 'application/json' } })
  if (!response.ok) throw new Error('GitHub Monitor endpoint is unavailable')
  return response.json() as Promise<Snapshot>
}
