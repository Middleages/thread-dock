import { execFile } from 'node:child_process'
import { promises as fs } from 'node:fs'
import { isAbsolute } from 'node:path'

const DEFAULT_TTL_MS = 5_000
const DEFAULT_TIMEOUT_MS = 8_000
const DEFAULT_MAX_BUFFER = 512 * 1024
const SESSION_PATTERN = /^[A-Za-z0-9._-]{1,64}$/
const ALLOWED_AGENT_FIELDS = ['name', 'agent_status', 'workspace_id', 'tab_id', 'pane_id', 'cwd', 'foreground_cwd']

export class HerdrConfigError extends Error {}

const text = (value) => typeof value === 'string' ? value : value == null ? '' : String(value)
const optionalText = (value) => text(value).trim() || undefined

function repositoryKey(value) {
  const candidate = text(value).trim().replace(/^https?:\/\//i, '').replace(/^github\.com\//i, '').replace(/\.git\/?$/, '').replace(/^\/+|\/+$/g, '')
  return /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(candidate) ? `github.com/${candidate}` : undefined
}

function issueRepository(issueUrl) {
  try {
    const url = new URL(issueUrl)
    if (url.protocol !== 'https:' || url.hostname.toLowerCase() !== 'github.com') return undefined
    const parts = url.pathname.split('/').filter(Boolean)
    return parts.length >= 2 ? `github.com/${parts[0]}/${parts[1]}` : undefined
  } catch { return undefined }
}

function validateSession(value) {
  const session = text(value).trim()
  if (!SESSION_PATTERN.test(session) || session === '.' || session === '..') throw new HerdrConfigError('Each binding requires an ASCII session name of 1-64 characters.')
  return session
}

function validateUrl(value, key) {
  if (value == null || value === '') return undefined
  try {
    const url = new URL(value)
    if (url.protocol !== 'https:' || url.hostname.toLowerCase() !== 'github.com') throw new Error()
    return url.toString()
  } catch { throw new HerdrConfigError(`${key} must be a complete GitHub URL.`) }
}

function parseBinding(value) {
  if (!value || typeof value !== 'object') throw new HerdrConfigError('Each Herdr binding must be an object.')
  const session = validateSession(value.session)
  const issueUrl = validateUrl(value.issueUrl, 'issueUrl')
  const projectUrl = validateUrl(value.projectUrl, 'projectUrl')
  const repository = value.repository == null ? undefined : repositoryKey(value.repository)
  if (value.repository != null && !repository) throw new HerdrConfigError('repository must look like github.com/OWNER/REPO.')
  if (!issueUrl && !projectUrl && !repository) throw new HerdrConfigError('Each Herdr binding requires issueUrl, projectUrl, or repository.')
  const issueRepo = issueRepository(issueUrl)
  const conflict = Boolean(issueRepo && repository && issueRepo !== repository)
  return {
    ...(issueUrl ? { issueUrl } : {}), ...(projectUrl ? { projectUrl } : {}), ...(repository ? { repository } : {}), session,
    ...(optionalText(value.worktree) ? { worktree: optionalText(value.worktree) } : {}),
    ...(optionalText(value.workspaceId) ? { workspaceId: optionalText(value.workspaceId) } : {}),
    ...(optionalText(value.tabId) ? { tabId: optionalText(value.tabId) } : {}),
    ...(optionalText(value.paneId) ? { paneId: optionalText(value.paneId) } : {}),
    ...(optionalText(value.agentName) ? { agentName: optionalText(value.agentName) } : {}),
    ...(optionalText(value.role) ? { role: optionalText(value.role) } : {}),
    ...(conflict ? { conflict: 'issueUrl and repository refer to different repositories.' } : {}),
  }
}

export function parseSessionsFile(raw) {
  let value
  try { value = typeof raw === 'string' ? JSON.parse(raw) : raw } catch { throw new HerdrConfigError('THREADDOCK_SESSIONS_FILE is not valid JSON.') }
  if (!value || typeof value !== 'object' || value.version !== 1 || !Array.isArray(value.bindings)) throw new HerdrConfigError('THREADDOCK_SESSIONS_FILE must contain version=1 and a bindings array.')
  const bindings = value.bindings.map(parseBinding)
  const sessions = [...new Set(bindings.map((binding) => binding.session))]
  return { version: 1, bindings, sessions }
}

export function normalizeAgentList(payload) {
  const result = payload?.result
  if (!result || result.type !== 'agent_list' || !Array.isArray(result.agents)) throw new Error('Unexpected Herdr agent response')
  return result.agents.map((agent) => {
    if (!agent || typeof agent !== 'object') return {}
    const normalized = {}
    for (const field of ALLOWED_AGENT_FIELDS) {
      const value = optionalText(agent[field])
      if (value) normalized[field] = value
    }
    return normalized
  })
}

export function runHerdr(args, { timeoutMs = DEFAULT_TIMEOUT_MS, maxBuffer = DEFAULT_MAX_BUFFER } = {}) {
  return new Promise((resolve, reject) => {
    execFile('herdr', args, { shell: false, timeout: timeoutMs, maxBuffer }, (error, stdout) => {
      if (error) { reject(new Error('Herdr request failed')); return }
      resolve(stdout)
    })
  })
}

function pathWithin(parent, child) {
  return child === parent || child.startsWith(`${parent.endsWith('/') ? parent : `${parent}/`}`)
}

function sameField(bindingValue, agentValue) { return !bindingValue || bindingValue === agentValue }
function hasIdentity(binding) { return Boolean(binding.workspaceId && binding.tabId && binding.paneId && binding.agentName) }
function displayLocation(binding, agent) {
  return { ...(binding.session ? { session: binding.session } : {}), ...(agent?.workspace_id || binding.workspaceId ? { workspaceId: agent?.workspace_id || binding.workspaceId } : {}), ...(agent?.tab_id || binding.tabId ? { tabId: agent?.tab_id || binding.tabId } : {}), ...(agent?.pane_id || binding.paneId ? { paneId: agent?.pane_id || binding.paneId } : {}), ...(agent?.name || binding.agentName ? { agentName: agent?.name || binding.agentName } : {}), ...(agent?.cwd ? { cwd: agent.cwd } : {}) }
}

function guidanceFor(connection) {
  if (connection.status === 'missing') return '보존된 Git 변경과 handoff를 확인합니다.'
  if (connection.status !== 'connected') return '세션 위치와 관찰을 먼저 재확인합니다.'
  if (connection.agentStatus === 'working') return '해당 세션으로 돌아가 관찰합니다.'
  if (connection.agentStatus === 'blocked') return '세션의 질문과 필요한 승인을 먼저 확인합니다.'
  if (connection.agentStatus === 'idle' || connection.agentStatus === 'done') return '최신 GitHub 기록과 남은 일을 확인합니다.'
  return '세션 위치와 관찰을 먼저 재확인합니다.'
}

function nextActionFor(connection) {
  if (connection.status !== 'connected') return connection.status === 'missing' ? 'check_handoff' : 'recheck'
  if (connection.agentStatus === 'working') return 'observe'
  if (connection.agentStatus === 'blocked') return 'inspect_question'
  if (connection.agentStatus === 'idle' || connection.agentStatus === 'done') return 'check_github'
  return 'recheck'
}

function handoffFor(connection) {
  const location = connection.location
  const where = [location.session && `세션 ${location.session}`, location.workspaceId && `workspace ${location.workspaceId}`, location.tabId && `tab ${location.tabId}`, location.paneId && `pane ${location.paneId}`, location.cwd && `cwd ${location.cwd}`].filter(Boolean).join(' · ')
  const state = connection.agentStatus ? `관찰 상태 ${connection.agentStatus}` : `연결 상태 ${connection.status}`
  const observed = connection.observedAt ? `관찰 시각 ${connection.observedAt}` : '성공한 관찰 시각 없음'
  return `${where || '지정된 Herdr 위치'} · ${observed} · ${state} · 다음 행동: ${guidanceFor(connection)}`
}

export function createHerdrMonitor({ env = process.env, readFile = (path) => fs.readFile(path, 'utf8'), realpath = (path) => fs.realpath(path), run = runHerdr, now = () => Date.now(), ttlMs = DEFAULT_TTL_MS } = {}) {
  const cache = new Map()
  let inFlight
  let revision = 0

  async function observeSession(session, currentTime, notices) {
    const cached = cache.get(session)
    if (cached && cached.nextAttemptAt > currentTime) return { ...cached, stale: cached.stale }
    if (cached?.agents && currentTime - cached.observedAt < ttlMs) return { ...cached, stale: false }
    try {
      const output = await run(['--session', session, 'agent', 'list'], { timeoutMs: DEFAULT_TIMEOUT_MS, maxBuffer: DEFAULT_MAX_BUFFER })
      const agents = normalizeAgentList(JSON.parse(output))
      const observedAt = currentTime
      const result = { session, agents, observedAt, nextAttemptAt: currentTime + ttlMs, status: 'fresh', stale: false }
      cache.set(session, result)
      return result
    } catch {
      notices.push(`Herdr 세션 ${session}을 조회하지 못했습니다. 세션과 Herdr 연결을 다시 확인하세요.`)
      const result = cached?.agents ? { ...cached, nextAttemptAt: currentTime + ttlMs, status: 'cached', stale: true } : { session, agents: [], observedAt: undefined, nextAttemptAt: currentTime + ttlMs, status: 'offline', stale: true }
      cache.set(session, result)
      return result
    }
  }

  async function load() {
    const observedAt = new Date(now()).toISOString()
    const file = env.THREADDOCK_SESSIONS_FILE?.trim()
    if (!file) return { source: 'herdr', schemaVersion: 1, revision: ++revision, observedAt, status: 'disabled', syncStatus: 'disabled', state: 'unknown', freshness: { state: 'stale', syncStatus: 'disabled', observedAt }, nextAction: 'configure', notices: ['Herdr 연결 파일이 설정되지 않았습니다. THREADDOCK_SESSIONS_FILE에 읽을 연결 JSON을 지정하세요.'], sessions: [], connections: [], unconnectedAgents: [], links: [] }
    if (!isAbsolute(file)) return { source: 'herdr', schemaVersion: 1, revision: ++revision, observedAt, status: 'unverified', syncStatus: 'setup_required', state: 'needs_operator', freshness: { state: 'stale', syncStatus: 'setup_required', observedAt }, nextAction: 'configure', notices: ['THREADDOCK_SESSIONS_FILE은 절대 경로여야 합니다.'], sessions: [], connections: [], unconnectedAgents: [], links: [] }
    let config
    try { config = parseSessionsFile(await readFile(file, 'utf8')) } catch (error) {
      const message = error instanceof HerdrConfigError ? error.message : 'Herdr 연결 파일을 읽지 못했습니다.'
      return { source: 'herdr', schemaVersion: 1, revision: ++revision, observedAt, status: 'unverified', syncStatus: 'setup_required', state: 'needs_operator', freshness: { state: 'stale', syncStatus: 'setup_required', observedAt }, nextAction: 'configure', notices: [message], sessions: [], connections: [], unconnectedAgents: [], links: [] }
    }
    if (env.HERDR_ENV !== '1') {
      const connections = config.bindings.map((binding) => {
        const connection = { ...binding, status: 'offline', location: displayLocation(binding), nextAction: 'recheck' }
        connection.handoff = handoffFor(connection)
        return connection
      })
      return { source: 'herdr', schemaVersion: 1, revision: ++revision, observedAt, status: 'offline', syncStatus: 'offline', state: 'unknown', freshness: { state: 'stale', syncStatus: 'offline', observedAt }, nextAction: 'recheck', notices: ['Herdr 상태는 HERDR_ENV=1인 Herdr pane에서만 조회합니다.'], sessions: [], connections, unconnectedAgents: [], links: [] }
    }
    const notices = []
    const currentTime = now()
    const observedSessions = await Promise.all(config.sessions.map((session) => observeSession(session, currentTime, notices)))
    const bySession = new Map(observedSessions.map((entry) => [entry.session, entry]))
    const realpathCache = new Map()
    const canonicalPath = async (path) => {
      if (!path) return undefined
      if (!realpathCache.has(path)) realpathCache.set(path, realpath(path).catch(() => undefined))
      return realpathCache.get(path)
    }
    const connections = []
    const matched = new Set()
    for (const binding of config.bindings) {
      const source = bySession.get(binding.session)
      let status = binding.conflict ? 'conflict' : source?.status === 'offline' ? 'offline' : source?.status === 'cached' ? 'cached' : 'missing'
      let agent
      if (!binding.conflict && source?.status !== 'offline' && !hasIdentity(binding)) status = 'unverified'
      if (!binding.conflict && source?.agents?.length) {
        if (!hasIdentity(binding)) status = 'unverified'
        else {
          const candidates = source.agents.filter((candidate) => sameField(binding.agentName, candidate.name) && sameField(binding.workspaceId, candidate.workspace_id) && sameField(binding.tabId, candidate.tab_id) && sameField(binding.paneId, candidate.pane_id))
          agent = candidates.length === 1 ? candidates[0] : undefined
          if (candidates.length > 1) status = 'unverified'
          else if (!agent && source.agents.some((candidate) => binding.paneId && candidate.pane_id === binding.paneId)) status = 'conflict'
          else if (agent) {
            status = source.status === 'cached' ? 'cached' : 'connected'
            if (!binding.worktree && binding.role !== 'coordinator') status = 'unverified'
            else if (binding.worktree) {
              const [root, cwd] = await Promise.all([canonicalPath(binding.worktree), canonicalPath(agent.cwd || agent.foreground_cwd)])
              if (!root || !cwd) status = 'unverified'
              else if (!pathWithin(root, cwd)) status = 'conflict'
            }
            matched.add(`${binding.session}:${agent.pane_id || agent.name || JSON.stringify(agent)}`)
          }
        }
      }
      const location = displayLocation(binding, agent)
      const connection = { ...binding, status, location, ...(agent?.agent_status ? { agentStatus: agent.agent_status } : {}), ...(source?.observedAt ? { observedAt: new Date(source.observedAt).toISOString() } : {}), nextAction: '' }
      connection.nextAction = nextActionFor(connection)
      connection.handoff = handoffFor(connection)
      connections.push(connection)
    }
    const unconnectedAgents = observedSessions.flatMap((session) => session.agents.map((agent) => ({ session: session.session, ...agent, connected: matched.has(`${session.session}:${agent.pane_id || agent.name || JSON.stringify(agent)}`) }))).filter((agent) => !agent.connected).map(({ connected, ...agent }) => agent)
    const lastSyncedAt = observedSessions.map((session) => session.observedAt).filter(Boolean).sort().at(-1)
    const hasFailure = observedSessions.some((session) => session.status === 'offline')
    const hasCache = observedSessions.some((session) => session.status === 'cached')
    const status = hasFailure ? (lastSyncedAt ? 'cached' : 'offline') : hasCache ? 'cached' : 'fresh'
    return { source: 'herdr', schemaVersion: 1, revision: ++revision, observedAt, status, syncStatus: status === 'fresh' ? 'synced' : status === 'cached' ? 'degraded' : 'offline', state: status === 'fresh' ? 'running' : 'stale', freshness: { state: status === 'fresh' ? 'fresh' : 'stale', syncStatus: status === 'fresh' ? 'synced' : status === 'cached' ? 'degraded' : 'offline', observedAt, ...(lastSyncedAt ? { lastSyncedAt: new Date(lastSyncedAt).toISOString() } : {}) }, nextAction: status === 'fresh' ? 'observe' : 'recheck', notices, sessions: observedSessions.map(({ session, agents, observedAt: lastObservedAt, status: sessionStatus }) => ({ session, status: sessionStatus, ...(lastObservedAt ? { observedAt: new Date(lastObservedAt).toISOString() } : {}), agents, links: [{ kind: 'herdr', label: `Herdr ${session}`, url: `herdr://session/${encodeURIComponent(session)}` }] })), connections, unconnectedAgents, links: [] }
  }

  return { getSnapshot: () => { if (!inFlight) inFlight = load().finally(() => { inFlight = undefined }); return inFlight } }
}
