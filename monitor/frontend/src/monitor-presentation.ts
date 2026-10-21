import type { HerdrAgent, HerdrConnection, HerdrSnapshot, Link, Project, WorkItem } from './types'

const stateLabel: Record<string, string> = {
  needs_operator: '판단 필요', running: '정상', completed: '완료', verified: '검증 완료',
  review: '확인', synced: '동기화됨', stale: '오래된 상태', offline: '오프라인',
  failed: '확인 필요', blocked: '판단 필요', ready: '준비됨', passed: '통과', approved: '승인됨', verify: '검증',
  published: '발행 완료', accepted: '승인됨', approve: '승인',
  open: '열림', closed: '닫힘', merged: '병합됨', unknown: '알 수 없음', pending: '대기 중', success: '성공', failure: '실패', neutral: '중립',
  REVIEW_REQUIRED: '리뷰 필요', APPROVED: '승인됨', CHANGES_REQUESTED: '변경 요청',
  working: '작업 중', idle: '대기 중', done: '완료', fresh: '최신', cached: '캐시된 관찰', degraded: '저하됨', setup_required: '설정 필요', disabled: '사용 중지', missing: '대상 없음', conflict: '불일치', unverified: '확인 필요', connected: '연결됨', observe: '돌아가 관찰', recheck: '먼저 재확인', configure: '연결 파일 설정',
}

export const labelFor = (value: string) => Object.prototype.hasOwnProperty.call(stateLabel, value) ? stateLabel[value] : value.replaceAll('_', ' ')
export const dateFor = (value?: string) => value ? new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '시각 없음'

export function herdrAgentDisplayName(agent: HerdrAgent) {
  const explicitName = agent.name?.trim()
  if (explicitName) return explicitName
  const workspaceLabel = agent.workspace_label?.trim()
  if (workspaceLabel) return workspaceLabel
  const cwd = agent.cwd?.trim() || agent.foreground_cwd?.trim() || ''
  const trimmedCWD = cwd.replace(/\/+$/, '')
  const basename = trimmedCWD.split('/').pop()?.trim() ?? ''
  return basename || '이름 없음'
}

function repositoryFor(url?: string) {
  if (!url) return undefined
  try {
    const parsed = new URL(url)
    const parts = parsed.pathname.split('/').filter(Boolean)
    return parsed.protocol === 'https:' && parsed.host && !parsed.username && !parsed.password && !parsed.search && !parsed.hash && parts.length >= 2 ? `${parsed.host.toLowerCase()}/${parts[0].toLowerCase()}/${parts[1].toLowerCase()}` : undefined
  } catch { return undefined }
}

function repositoryKey(value?: string) {
  if (!value?.trim()) return ''
  try {
    const parsed = new URL(/^https:\/\//i.test(value) ? value : `https://${value}`)
    const parts = parsed.pathname.split('/').filter(Boolean)
    return parsed.protocol === 'https:' && parsed.host && !parsed.username && !parsed.password && !parsed.search && !parsed.hash && parts.length === 2 ? `${parsed.host.toLowerCase()}/${parts[0].toLowerCase()}/${parts[1].toLowerCase()}` : ''
  } catch { return '' }
}

function primaryWorkURL(work: WorkItem) {
  return work.github?.url ?? work.links?.find((link) => ['github', 'issue', 'pull_request', 'project_item'].includes(link.kind))?.url
}

function locatorKey(value?: string) {
  if (!value) return ''
  try {
    const parsed = new URL(value)
    if (parsed.protocol !== 'https:' || !parsed.host || parsed.username || parsed.password || parsed.search || parsed.hash) return ''
    const parts = parsed.pathname.split('/').filter(Boolean)
    if (parts.length !== 4 || !/^\d+$/.test(parts[3]) || Number(parts[3]) <= 0) return ''
    const route = parts[2].toLowerCase() === 'pulls' ? 'pull' : parts[2].toLowerCase()
    if (route === 'projects') {
      if (!['users', 'orgs'].includes(parts[0].toLowerCase())) return ''
    } else if (!['issues', 'pull'].includes(route)) {
      return ''
    }
    return `${parsed.protocol}//${parsed.host.toLowerCase()}/${parts[0].toLowerCase()}/${parts[1].toLowerCase()}/${route}/${parts[3]}`
  } catch { return '' }
}

function isTrustedProjectLink(link: Link) {
  if (link.kind !== 'github' || link.label !== 'Project') return false
  try {
    const parsed = new URL(link.url)
    const parts = parsed.pathname.split('/').filter(Boolean)
    return parsed.protocol === 'https:' && Boolean(parsed.hostname) && parts.length === 4 && ['users', 'orgs'].includes(parts[0]) && parts[2] === 'projects' && /^\d+$/.test(parts[3])
  } catch { return false }
}

export function connectionsForWork(work: WorkItem | undefined, project: Project | undefined, herdr: HerdrSnapshot | undefined) {
  if (!work || !herdr) return []
  const primaryURL = primaryWorkURL(work)
  const repository = repositoryFor(primaryURL)
  const projectURLs = new Set((project?.links ?? []).filter(isTrustedProjectLink).map((link) => link.url))
  const primaryIsIssue = work.github?.kind === 'issue' || (!work.github?.kind && Boolean(primaryURL && /\/issues\//.test(primaryURL)))
  const issueLocators = [primaryIsIssue ? primaryURL : undefined, ...(work.github?.relatedIssueUrls ?? [])].map(locatorKey).filter(Boolean)
  const issueConnections = herdr.connections.filter((connection) => issueLocators.includes(locatorKey(connection.issueUrl)))
  if (issueConnections.length > 0) return [...new Map(issueConnections.map((connection) => [
    [connection.session, connection.workspaceId, connection.tabId, connection.paneId, connection.agentName].join('|'), connection,
  ])).values()]
  const projectConnections = herdr.connections.filter((connection) => connection.projectUrl && [...projectURLs].some((url) => locatorKey(url) === locatorKey(connection.projectUrl)))
  if (projectConnections.length > 0) return projectConnections
  return herdr.connections.filter((connection) => !connection.issueUrl && !connection.projectUrl && connection.role === 'coordinator' && Boolean(repository) && repositoryKey(connection.repository) === repositoryKey(repository))
}

export function herdrGuidance(connection: HerdrConnection) {
  if (connection.status === 'connected' && connection.agentStatus === 'working') return '해당 세션에서 작업 중입니다.'
  if (connection.status === 'connected' && connection.agentStatus === 'blocked') return '세션의 질문과 필요한 승인을 확인하세요.'
  if (connection.status === 'connected' && (connection.agentStatus === 'idle' || connection.agentStatus === 'done')) return 'GitHub 기록과 남은 작업을 확인하세요.'
  if (connection.status === 'missing') return '보존된 변경과 handoff를 확인하세요.'
  return '세션 위치와 관찰 상태를 재확인하세요.'
}
