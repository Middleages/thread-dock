import test from 'node:test'
import assert from 'node:assert/strict'
import { createThreadDockMonitor } from './thread-dock-monitor.mjs'

test('composes independent GitHub and Herdr observations without dropping either source', async () => {
  const monitor = createThreadDockMonitor({
    github: { getSnapshot: async () => ({ source: 'github', projects: [{ projectId: 'repo:a/b' }], observedAt: '2026-09-10T00:00:00Z', freshness: { state: 'fresh', syncStatus: 'synced' } }) },
    herdr: { getSnapshot: async () => ({ source: 'herdr', status: 'fresh', observedAt: '2026-09-10T00:00:01Z', sessions: [], connections: [] }) },
  })
  const snapshot = await monitor.getSnapshot()
  assert.equal(snapshot.projects.length, 1)
  assert.equal(snapshot.herdr.status, 'fresh')
  assert.equal(snapshot.observedAt, '2026-09-10T00:00:00Z')
})

test('keeps Herdr visible when GitHub source fails', async () => {
  const snapshot = await createThreadDockMonitor({ github: { getSnapshot: async () => { throw new Error('private github error') } }, herdr: { getSnapshot: async () => ({ source: 'herdr', status: 'fresh', sessions: [{ session: 'feature', agents: [] }], connections: [] }) } }).getSnapshot()
  assert.equal(snapshot.projects.length, 0)
  assert.equal(snapshot.herdr.sessions[0].session, 'feature')
  assert.doesNotMatch(JSON.stringify(snapshot), /private github error/)
})
