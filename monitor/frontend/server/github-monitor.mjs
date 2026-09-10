import { execFile } from 'node:child_process'

const DEFAULT_TTL_MS = 60_000
const DEFAULT_TIMEOUT_MS = 15_000
const DEFAULT_MAX_BUFFER = 2 * 1024 * 1024
const REPO_PATTERN = /^([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)$/
const PROJECT_PATTERN = /^https:\/\/github\.com\/(users|orgs)\/([A-Za-z0-9_.-]+)\/projects\/(\d+)\/?$/

export class MonitorConfigError extends Error {}

export function parseMonitorConfig(env = process.env) {
  const repos = parseList(env.THREADDOCK_REPOS, 'THREADDOCK_REPOS', (value) => {
    const match = REPO_PATTERN.exec(value)
    if (!match) throw new MonitorConfigError(`THREADDOCK_REPOS must contain OWNER/REPO entries (for example acme/app): ${value}`)
    return { owner: match[1], repo: match[2] }
  })
  const projects = parseList(env.THREADDOCK_PROJECTS, 'THREADDOCK_PROJECTS', (value) => {
    const match = PROJECT_PATTERN.exec(value)
    if (!match) throw new MonitorConfigError('THREADDOCK_PROJECTS must contain URLs such as https://github.com/users/OWNER/projects/N or https://github.com/orgs/OWNER/projects/N')
    return { owner: match[2], number: Number(match[3]), url: value }
  })
  const unknown = Object.keys(env).filter((key) => key.startsWith('THREADDOCK_') && !['THREADDOCK_REPOS', 'THREADDOCK_PROJECTS', 'THREADDOCK_SESSIONS_FILE'].includes(key))
  if (unknown.length > 0) throw new MonitorConfigError(`Unknown ThreadDock monitor configuration: ${unknown.join(', ')}. Use THREADDOCK_REPOS, THREADDOCK_PROJECTS, and optional THREADDOCK_SESSIONS_FILE.`)
  return { repos, projects }
}

function parseList(raw, name, parser) {
  if (raw == null || raw.trim() === '') return []
  return raw.split(',').map((item) => item.trim()).filter(Boolean).map(parser)
}

export function runGh(args, { timeoutMs = DEFAULT_TIMEOUT_MS, maxBuffer = DEFAULT_MAX_BUFFER } = {}) {
  return new Promise((resolve, reject) => {
    execFile('gh', args, { shell: false, timeout: timeoutMs, maxBuffer }, (error, stdout) => {
      if (error) {
        reject(new Error('GitHub CLI request failed'))
        return
      }
      resolve(stdout)
    })
  })
}

const text = (value) => typeof value === 'string' ? value : value == null ? '' : String(value)
const stateFor = (value) => {
  const state = text(value).trim().toUpperCase()
  return state ? state.toLowerCase() : 'unknown'
}
const safeUrl = (value) => typeof value === 'string' && /^https:\/\/github\.com\//.test(value) ? value : undefined
const bodyUrls = (value) => {
  const matches = text(value).match(/https?:\/\/[^\s)\]}>,]+/g) || []
  return [...new Set(matches.map((url) => url.replace(/[.,;:!?]+$/, '')))]
}

function baseWork({ workId, title, body, url, state, updatedAt, github, sourceMeta }) {
  const observedAt = sourceMeta?.observedAt ? new Date(sourceMeta.observedAt).toISOString() : undefined
  const enrichedGithub = { ...github, ...(observedAt ? { observedAt } : {}), ...(sourceMeta?.stale ? { stale: true } : {}) }
  return {
    workId, title: text(title) || workId, ...(body ? { request: text(body) } : {}), state, syncStatus: sourceMeta?.stale ? 'stale' : 'synced', nextAction: 'review', evidenceRefs: url ? [url] : [], updatedAt,
    tasks: [], publications: [], links: [...(url ? [{ kind: enrichedGithub.kind, label: enrichedGithub.kind === 'pull_request' ? 'Pull request' : enrichedGithub.kind === 'issue' ? 'Issue' : 'GitHub', url }] : []), ...bodyUrls(body).filter((bodyUrl) => bodyUrl !== url).map((bodyUrl) => ({ kind: 'evidence', label: '본문 링크', url: bodyUrl }))], github: enrichedGithub,
  }
}

function checkDetails(rollup) {
  if (!Array.isArray(rollup)) return []
  return rollup.map((check) => ({ name: text(check?.name || check?.context || check?.workflowName) || 'check', status: text(check?.status || check?.state), conclusion: text(check?.conclusion || check?.state), ...(safeUrl(check?.detailsUrl || check?.targetUrl) ? { url: check.detailsUrl || check.targetUrl } : {}) }))
}

function repoLinks(owner, repo) {
  const base = `https://github.com/${owner}/${repo}`
  return [{ kind: 'github', label: 'Issues', url: `${base}/issues` }, { kind: 'github', label: 'PRs', url: `${base}/pulls` }, { kind: 'github', label: 'Wiki', url: `${base}/wiki` }]
}

function mapIssue(issue, sourceMeta, relatedPullRequestUrls = []) {
  const url = safeUrl(issue?.url)
  return baseWork({ workId: `issue:${url || issue?.number}`, title: issue?.title, body: issue?.body, url, state: stateFor(issue?.state), updatedAt: issue?.updatedAt, github: { kind: 'issue', number: issue?.number, url, state: text(issue?.state), relatedPullRequestUrls, ...(issue?.author?.login ? { author: issue.author.login } : {}) }, sourceMeta })
}

function mapPullRequest(pr, sourceMeta) {
  const url = safeUrl(pr?.url)
  const relatedIssueUrls = Array.isArray(pr?.closingIssuesReferences) ? pr.closingIssuesReferences.map((ref) => safeUrl(ref?.url)).filter(Boolean) : []
  return baseWork({ workId: `pr:${url || pr?.number}`, title: pr?.title, body: pr?.body, url, state: stateFor(pr?.state), updatedAt: pr?.updatedAt, github: { kind: 'pull_request', number: pr?.number, url, state: text(pr?.state), reviewDecision: text(pr?.reviewDecision), checks: checkDetails(pr?.statusCheckRollup), relatedIssueUrls }, sourceMeta })
}

function projectFieldValue(fieldValue) {
  if (fieldValue == null || typeof fieldValue !== 'object') return text(fieldValue)
  for (const key of ['name', 'value', 'text', 'number', 'date']) if (fieldValue[key] != null && text(fieldValue[key]) !== '') return text(fieldValue[key])
  if (Array.isArray(fieldValue.users)) return fieldValue.users.map((user) => text(user?.login || user?.name)).filter(Boolean).join(', ')
  if (Array.isArray(fieldValue.labels)) return fieldValue.labels.map((label) => text(label?.name || label)).filter(Boolean).join(', ')
  return ''
}

function projectFields(item) {
  const fields = {}
  for (const fieldValue of item?.fieldValues || []) {
    const name = text(fieldValue?.field?.name || fieldValue?.name)
    if (name) fields[name] = projectFieldValue(fieldValue)
  }
  const structural = new Set(['id', 'content', 'fieldValues', 'totalCount', 'type', 'number', 'title', 'body', 'url', 'repository', 'state', 'updatedAt'])
  for (const [name, value] of Object.entries(item || {})) {
    if (!structural.has(name) && value != null) {
      const rendered = projectFieldValue(value)
      if (rendered) fields[name] = rendered
    }
  }
  return fields
}

function mapProjectItem(item, boardId, index, sourceMeta) {
  const content = item?.content || {}
  const hasContent = Boolean(item?.content)
  const url = safeUrl(content?.url)
  return baseWork({ workId: `project-item:${boardId}:${url || content?.number || item?.id || index}`, title: content?.title || content?.name || 'GitHub project item (content unavailable)', body: content?.body, url, state: stateFor(content?.state), updatedAt: content?.updatedAt, github: { kind: 'project_item', number: content?.number, url, state: text(content?.state), fields: projectFields(item), contentAvailable: hasContent }, sourceMeta })
}

export function createGitHubMonitor({ env = process.env, run = runGh, now = () => Date.now(), ttlMs = DEFAULT_TTL_MS } = {}) {
  const cache = new Map()
  let inFlight
  let revision = 0
  let config
  try { config = parseMonitorConfig(env) } catch (error) { config = { error: error instanceof MonitorConfigError ? error.message : 'Invalid monitor configuration' } }

  async function readSource(key, args, notices, validate = () => true) {
    const currentTime = now()
    const cached = cache.get(key)
    if (cached && cached.nextAttemptAt > currentTime) {
      return { data: cached.data, stale: Boolean(cached.stale), observedAt: cached.observedAt }
    }
    if (cached?.data != null && currentTime - cached.observedAt < ttlMs) return { data: cached.data, stale: false, observedAt: cached.observedAt }
    try {
      const output = await run(args)
      const data = JSON.parse(output)
      if (!validate(data)) throw new Error('Unexpected GitHub response')
      cache.set(key, { data, observedAt: currentTime, nextAttemptAt: currentTime + ttlMs, stale: false })
      return { data, stale: false, observedAt: currentTime }
    } catch {
      notices.push(`${key} 조회에 실패했습니다. 마지막 성공 시각이 있으면 보존된 데이터를 표시합니다.`)
      if (cached?.data != null) {
        cache.set(key, { ...cached, nextAttemptAt: currentTime + ttlMs, stale: true })
        return { data: cached.data, stale: true, observedAt: cached.observedAt }
      }
      cache.set(key, { data: null, observedAt: undefined, nextAttemptAt: currentTime + ttlMs, stale: true })
      return { data: null, stale: true, observedAt: undefined }
    }
  }

  async function fetchRepo(repo, notices) {
    const key = `${repo.owner}/${repo.repo}`
    const [issuesResult, prsResult] = await Promise.all([
      readSource(`${key} issues`, ['issue', 'list', '--repo', key, '--state', 'all', '--limit', '100', '--json', 'number,title,url,state,body,updatedAt'], notices, Array.isArray),
      readSource(`${key} prs`, ['pr', 'list', '--repo', key, '--state', 'all', '--limit', '100', '--json', 'number,title,url,state,body,updatedAt,reviewDecision,statusCheckRollup,closingIssuesReferences'], notices, Array.isArray),
    ])
    const issues = Array.isArray(issuesResult.data) ? issuesResult.data : []
    const prs = Array.isArray(prsResult.data) ? prsResult.data : []
    const updatedAt = Math.max(issuesResult.observedAt || 0, prsResult.observedAt || 0)
    const prsByIssueUrl = new Map()
    for (const pr of prs) for (const issueUrl of (Array.isArray(pr?.closingIssuesReferences) ? pr.closingIssuesReferences : []).map((ref) => safeUrl(ref?.url)).filter(Boolean)) {
      const urls = prsByIssueUrl.get(issueUrl) || []
      urls.push(safeUrl(pr?.url))
      prsByIssueUrl.set(issueUrl, urls.filter(Boolean))
    }
    const workItems = [...issues.map((issue) => mapIssue(issue, issuesResult, prsByIssueUrl.get(safeUrl(issue?.url)) || [])), ...prs.map((pr) => mapPullRequest(pr, prsResult))]
    const partial = issuesResult.stale || prsResult.stale
    const observedAt = updatedAt ? new Date(updatedAt).toISOString() : undefined
    const project = { projectId: `repo:${key}`, name: key, source: 'github', state: partial ? 'stale' : 'running', syncStatus: partial ? 'degraded' : 'synced', nextAction: 'review', evidenceRefs: [`https://github.com/${key}`], updatedAt: observedAt, observedAt, links: repoLinks(repo.owner, repo.repo), workItems, ...(partial ? { notices: [`${key}의 일부 GitHub 조회가 오래된 상태입니다.`] } : {}) }
    if (issues.length >= 100) notices.push(`${key} 이슈 조회 결과가 최대 100개로 제한됐습니다.`)
    if (prs.length >= 100) notices.push(`${key} PR 조회 결과가 최대 100개로 제한됐습니다.`)
    return { project, stale: issuesResult.stale || prsResult.stale }
  }

  async function fetchBoard(board, notices) {
    const key = `board:${board.owner}/${board.number}`
    const result = await readSource(key, ['project', 'item-list', String(board.number), '--owner', board.owner, '--format', 'json', '--limit', '100'], notices, (value) => value && typeof value === 'object' && Array.isArray(value.items))
    const payload = result.data && typeof result.data === 'object' ? result.data : {}
    const items = Array.isArray(payload.items) ? payload.items : []
    if (Number(payload.totalCount) > 100 || items.length >= 100) notices.push(`${key} 조회 결과가 최대 100개로 제한됐습니다.`)
    const inaccessible = items.filter((item) => !item?.content).length
    if (inaccessible) notices.push(`${key}에서 ${inaccessible}개 항목의 내용을 확인할 수 없습니다.`)
    const observedAt = result.observedAt ? new Date(result.observedAt).toISOString() : undefined
    return { project: { projectId: key, name: `GitHub Project ${board.owner}/${board.number}`, source: 'github', state: result.stale ? 'stale' : 'running', syncStatus: result.stale ? 'degraded' : 'synced', nextAction: 'review', evidenceRefs: [board.url], updatedAt: observedAt, observedAt, links: [{ kind: 'github', label: 'Project', url: board.url }], workItems: items.map((item, index) => mapProjectItem(item, `${board.owner}/${board.number}`, index, result)), ...(result.stale ? { notices: [`${key} 조회가 오래된 상태입니다.`] } : {}) }, stale: result.stale }
  }

  async function load() {
    if (config.error) return { source: 'github', notices: [config.error, 'GitHub Monitor를 사용하려면 THREADDOCK_REPOS=OWNER/REPO[,OWNER/REPO]를 설정하고 Vite를 다시 시작하세요.'], projects: [], schemaVersion: 2, revision: ++revision, observedAt: new Date(now()).toISOString(), freshness: { state: 'stale', syncStatus: 'setup_required' }, state: 'needs_operator', syncStatus: 'setup_required', nextAction: 'setup', evidenceRefs: [] }
    if (config.repos.length === 0 && config.projects.length === 0) return { source: 'github', notices: ['GitHub Monitor가 설정되지 않았습니다. THREADDOCK_REPOS=OWNER/REPO[,OWNER/REPO]를 설정하고 Vite를 다시 시작하세요.'], projects: [], schemaVersion: 2, revision: ++revision, observedAt: new Date(now()).toISOString(), freshness: { state: 'stale', syncStatus: 'setup_required' }, state: 'needs_operator', syncStatus: 'setup_required', nextAction: 'setup', evidenceRefs: [] }
    const notices = []
    const results = await Promise.all([...config.repos.map((repo) => fetchRepo(repo, notices)), ...config.projects.map((board) => fetchBoard(board, notices))])
    const stale = results.some((result) => result.stale)
    const observedAt = new Date(now()).toISOString()
    const lastSyncedAt = results.map((result) => result.project.updatedAt).filter(Boolean).sort().at(-1)
    return { source: 'github', notices, projects: results.map((result) => result.project), schemaVersion: 2, revision: ++revision, observedAt, freshness: { state: stale ? 'stale' : 'fresh', syncStatus: stale ? 'degraded' : 'synced', observedAt, ...(lastSyncedAt ? { lastSyncedAt } : {}) }, state: stale ? 'stale' : 'running', syncStatus: stale ? 'degraded' : 'synced', nextAction: 'review', evidenceRefs: results.flatMap((result) => result.project.evidenceRefs) }
  }

  return { getSnapshot: () => { if (!inFlight) inFlight = load().finally(() => { inFlight = undefined }); return inFlight } }
}

function isLoopbackAddress(address) {
  return address === '127.0.0.1' || address === '::1' || address === '::ffff:127.0.0.1'
}

function isAllowedBrowserRequest(req) {
  if (!isLoopbackAddress(req.socket?.remoteAddress)) return false
  const hostHeader = text(req.headers?.host).toLowerCase()
  const hostMatch = /^(localhost|127\.0\.0\.1)(?::(\d+))?$|^(\[::1\]|::1)(?::(\d+))?$/.exec(hostHeader)
  if (!hostMatch) return false
  const host = hostMatch[1] || hostMatch[3]
  const hostPort = hostMatch[2] || hostMatch[4] || ''
  const origin = req.headers?.origin
  if (!origin) return true
  try {
    const originUrl = new URL(origin)
    const originHost = originUrl.hostname.toLowerCase().replace(/[\[\]]/g, '')
    const expectedHost = host.replace(/[\[\]]/g, '')
    const defaultPort = originUrl.protocol === 'https:' ? '443' : '80'
    const originPort = originUrl.port || defaultPort
    return ['http:', 'https:'].includes(originUrl.protocol) && originHost === expectedHost && (!hostPort || originPort === hostPort)
  } catch {
    return false
  }
}

export function createGitHubMonitorMiddleware(monitor) {
  return async (req, res, next) => {
    let pathname
    try { pathname = new URL(req.url || '/', 'http://localhost').pathname } catch { pathname = '' }
    if (pathname !== '/api/github-monitor') { next?.(); return }
    if (!isAllowedBrowserRequest(req)) { res.statusCode = 403; res.end('Forbidden'); return }
    if (req.method !== 'GET') { res.statusCode = 405; res.setHeader('Allow', 'GET'); res.end('Method not allowed'); return }
    try {
      const snapshot = await monitor.getSnapshot()
      res.statusCode = 200
      res.setHeader('Content-Type', 'application/json; charset=utf-8')
      res.end(JSON.stringify(snapshot))
    } catch {
      res.statusCode = 500
      res.setHeader('Content-Type', 'application/json; charset=utf-8')
      res.end(JSON.stringify({ error: 'GitHub Monitor를 불러오지 못했습니다.' }))
    }
  }
}
