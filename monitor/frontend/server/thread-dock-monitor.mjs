export function createThreadDockMonitor({ github, herdr }) {
  let inFlight
  async function load() {
    const [githubResult, herdrResult] = await Promise.allSettled([Promise.resolve().then(() => github.getSnapshot()), Promise.resolve().then(() => herdr.getSnapshot())])
    const now = new Date().toISOString()
    const githubSnapshot = githubResult.status === 'fulfilled' ? githubResult.value : {
      source: 'github', schemaVersion: 2, revision: 0, observedAt: now, projects: [], notices: ['GitHub Monitor를 불러오지 못했습니다. 마지막 성공 기록을 확인하세요.'], state: 'stale', syncStatus: 'offline', nextAction: 'recheck', evidenceRefs: [], freshness: { state: 'stale', syncStatus: 'offline', observedAt: now },
    }
    const herdrSnapshot = herdrResult.status === 'fulfilled' ? herdrResult.value : {
      source: 'herdr', schemaVersion: 1, revision: 0, observedAt: now, status: 'offline', state: 'unknown', syncStatus: 'offline', nextAction: 'recheck', notices: ['Herdr Monitor를 불러오지 못했습니다. 세션과 Herdr 연결을 다시 확인하세요.'], sessions: [], connections: [], unconnectedAgents: [], links: [], freshness: { state: 'stale', syncStatus: 'offline', observedAt: now },
    }
    return { ...githubSnapshot, herdr: herdrSnapshot }
  }
  return { getSnapshot: () => { if (!inFlight) inFlight = load().finally(() => { inFlight = undefined }); return inFlight } }
}
