import type { Snapshot, SnapshotSource } from './types'

type WailsWindow = Window & { go?: { main?: { App?: { GetMonitorSnapshot?: () => Promise<Snapshot> } } } }

export const getMonitorSnapshot: SnapshotSource = async () => {
  const binding = (window as WailsWindow).go?.main?.App?.GetMonitorSnapshot
  if (!binding && new URLSearchParams(window.location.search).has('fixture')) return fixtureSnapshot
  if (!binding) throw new Error('Wails monitor binding is unavailable')
  return binding()
}

const fixtureSnapshot: Snapshot = {
  schemaVersion: 2, revision: 3, observedAt: '2026-09-07T01:00:00Z',
  freshness: { state: 'fresh', syncStatus: 'synced' }, state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [],
  projects: [{ projectId: 'payments', name: '결제 안정성 개선', state: 'needs_operator', syncStatus: 'synced', nextAction: 'review', updatedAt: '2026-09-07T01:00:00Z', evidenceRefs: ['state://payments'], workItems: [{ workId: 'retry', title: '결제 실패 재시도 개선', request: '중복 결제를 줄이고 재시도 근거를 확인합니다.', state: 'needs_operator', syncStatus: 'synced', nextAction: 'review', evidenceRefs: ['state://retry'], updatedAt: '2026-09-07T01:00:00Z', tasks: [{ taskId: 'task-1', repoKey: 'payments-api', state: 'verified', verification: 'passed', review: 'approved', merge: 'ready' }], publications: [{ intentId: 'issue-1', key: 'parent-issue', generation: 1, kind: 'issue', status: 'published', attempts: 1, url: 'https://github.com/acme/payments/issues/184' }], decisions: [{ decisionId: 'decision-1', summary: '실패 재시도는 제한된 횟수로 유지', status: 'accepted', observedAt: '2026-09-07T00:30:00Z' }], handoffs: [{ summary: '독립 확인 결과를 검토하세요', nextAction: 'approve', evidenceRefs: ['handoff://retry'] }], links: [{ kind: 'github', label: 'Parent Issue 열기', url: 'https://github.com/acme/payments/issues/184' }] }] }, { projectId: 'customer-web', name: 'Customer Web', state: 'running', syncStatus: 'synced', nextAction: 'verify', updatedAt: '2026-09-06T20:11:00Z', evidenceRefs: ['state://customer-web'], workItems: [] }],
}
