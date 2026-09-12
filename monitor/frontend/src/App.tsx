import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { HerdrConnection, HerdrSnapshot, Link, Project, Snapshot, SnapshotSource, WorkItem } from './types'
import { getMonitorSnapshot, getMonitorSnapshotAll } from './bindings'
import { MarkdownBody } from './MarkdownBody'
import { ProjectToolbox } from './ProjectToolbox'
import { WorkTable } from './WorkTable'
import { isSafeExternalURL, openExternalURL } from './safe-url'
import { toolboxKeyFor } from './project-key'
import './styles.css'

export const sortProjects = (projects: Project[]): Project[] => [...projects].sort((a, b) => {
  const attention = (state: string) => state === 'needs_operator' ? 0 : 1
  const priority = attention(a.state) - attention(b.state)
  if (priority !== 0) return priority
  return (Date.parse(b.updatedAt ?? '') || 0) - (Date.parse(a.updatedAt ?? '') || 0)
})

const stateLabel: Record<string, string> = {
  needs_operator: '판단 필요', running: '정상', completed: '완료', verified: '검증 완료',
  review: '확인', synced: '동기화됨', stale: '오래된 상태', offline: '오프라인',
  failed: '확인 필요', blocked: '판단 필요', ready: '준비됨', passed: '통과', approved: '승인됨', verify: '검증',
  published: '발행 완료', accepted: '승인됨', approve: '승인',
  open: '열림', closed: '닫힘', merged: '병합됨', unknown: '알 수 없음', pending: '대기 중', success: '성공', failure: '실패', neutral: '중립',
  REVIEW_REQUIRED: '리뷰 필요', APPROVED: '승인됨', CHANGES_REQUESTED: '변경 요청',
  working: '작업 중', idle: '대기 중', done: '완료', fresh: '최신', cached: '캐시된 관찰', missing: '대상 없음', conflict: '불일치', unverified: '확인 필요', connected: '연결됨', observe: '돌아가 관찰', recheck: '먼저 재확인', configure: '연결 파일 설정',
}

const labelFor = (value: string) => stateLabel[value] ?? value.replaceAll('_', ' ')
const timeFor = (value?: string) => value ? new Intl.DateTimeFormat('ko-KR', { hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—'
const dateFor = (value?: string) => value ? new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '시각 없음'

declare global {
  interface Window { runtime?: { BrowserOpenURL?: (url: string) => void; ClipboardSetText?: (text: string) => Promise<boolean> } }
}

function ExternalLink({ url, label }: { url: string; label: string }) {
  if (!isSafeExternalURL(url)) return <span className="blocked-link">{label}</span>
  const onClick = (event: React.MouseEvent<HTMLAnchorElement>) => {
    if (window.runtime?.BrowserOpenURL) { event.preventDefault(); openExternalURL(url) }
  }
  return <a href={url} target="_blank" rel="noreferrer" onClick={onClick}>{label}</a>
}

function StatusMark({ value }: { value: string }) {
  const tone = value === 'needs_operator' || value === 'failed' || value === 'blocked' ? 'attention' : value === 'completed' || value === 'verified' || value === 'synced' || value === 'running' ? 'success' : 'progress'
  return <span className={`status-mark ${tone}`} aria-hidden="true" />
}

function LoadingState() {
  return <div className="loading-state" role="status" aria-live="polite"><span className="skeleton skeleton-title" /><span className="skeleton skeleton-row" /><span className="skeleton skeleton-row" /><p>프로젝트 현황을 불러오는 중입니다.</p></div>
}

function ProjectList({ projects, selected, onSelect }: { projects: Project[]; selected: string | null; onSelect: (id: string) => void }) {
  return <section className="ledger" aria-labelledby="ledger-title">
    <div className="section-heading"><div><h1 id="ledger-title">작업 현황</h1><p>열린 Issue와 PR을 확인하고 작업을 선택하세요.</p></div><span className="count-label">{projects.length}개 프로젝트</span></div>
    <div className="project-table" role="table" aria-label="프로젝트 목록">
      <div className="table-head" role="row"><span role="columnheader">최근 활동</span><span role="columnheader">프로젝트</span><span role="columnheader">조회 상태</span></div>
      {projects.map((project) => <button key={project.projectId} type="button" className={`project-row${selected === project.projectId ? ' selected' : ''}`} aria-pressed={selected === project.projectId} onClick={() => onSelect(project.projectId)} autoFocus={selected === project.projectId && projects[0] === project}>
        <time role="cell" dateTime={project.updatedAt}>{timeFor(project.updatedAt)}</time>
        <span role="cell" className="project-name"><strong>{project.name}</strong><small>{project.workItems.length}개 업무 · {dateFor(project.updatedAt)}</small></span>
        <span role="cell" className="state-cell"><StatusMark value={project.state} />{labelFor(project.state)}</span>
      </button>)}
    </div>
  </section>
}

function GithubEvidence({ work }: { work: WorkItem }) {
  const github = work.github
  if (!github) return null
  const checks = github.checks ?? []
  const fields = Object.entries(github.fields ?? {})
  return <>
    <section aria-labelledby="github-state-title"><h3 id="github-state-title">GitHub 상태</h3><div className="ruled-list">
      <div className="evidence-row"><div><strong>{github.kind === 'pull_request' ? 'Pull request' : github.kind === 'issue' ? 'Issue' : 'Project item'}</strong><small>{github.number ? `#${github.number}` : '번호 없음'} · 관찰 {dateFor(github.observedAt)}</small></div><span>{labelFor((github.state ?? 'unknown').toLowerCase())}</span></div>
      {github.reviewDecision && <div className="evidence-row"><div><strong>리뷰</strong></div><span>{labelFor(github.reviewDecision)}</span></div>}
      {checks.length > 0 && <div className="evidence-row"><div><strong>검사</strong><small>{checks.map((check) => check.name).join(' · ')}</small></div><span>{checks.map((check) => labelFor((check.conclusion || check.status || 'unknown').toLowerCase())).join(' · ')}</span></div>}
      {github.relatedIssueUrls && github.relatedIssueUrls.length > 0 && <div className="evidence-row"><div><strong>연결된 Issue</strong></div><span>{github.relatedIssueUrls.map((url) => <ExternalLink key={url} url={url} label="Issue" />)}</span></div>}
      {github.relatedPullRequestUrls && github.relatedPullRequestUrls.length > 0 && <div className="evidence-row"><div><strong>연결된 PR</strong></div><span>{github.relatedPullRequestUrls.map((url) => <ExternalLink key={url} url={url} label="PR" />)}</span></div>}
    </div></section>
    {fields.length > 0 && <section aria-labelledby="github-fields-title"><h3 id="github-fields-title">Project 필드</h3><div className="ruled-list">{fields.map(([name, value]) => <div className="evidence-row" key={name}><strong>{name}</strong><span>{value || '값 없음'}</span></div>)}</div></section>}
  </>
}

function EvidenceList({ work, project }: { work: WorkItem; project?: Project }) {
  return <div className="evidence-sections">
    {work.github && <GithubEvidence work={work} />}
    {work.decisions && work.decisions.length > 0 && <section aria-labelledby="decisions-title"><h3 id="decisions-title">최근 결정</h3><div className="ruled-list">{work.decisions.map((decision) => <div className="evidence-row" key={decision.decisionId}><div><strong>{decision.summary}</strong><small>{dateFor(decision.observedAt)}</small></div><span>{labelFor(decision.status)}</span></div>)}</div></section>}
    {work.handoffs && work.handoffs.length > 0 && <section aria-labelledby="handoffs-title"><h3 id="handoffs-title">최근 handoff</h3><div className="ruled-list">{work.handoffs.map((handoff, index) => <div className="evidence-row" key={`${handoff.summary}-${index}`}><div><strong>{handoff.summary}</strong><small>{handoff.evidenceRefs.join(' · ')}</small></div><span>{labelFor(handoff.nextAction)}</span></div>)}</div></section>}
    <section aria-labelledby="links-title"><h3 id="links-title">관련 링크</h3><div className="link-list">{[...(project?.links ?? []), ...(work.links ?? [])].length > 0 ? [...(project?.links ?? []), ...(work.links ?? [])].map((link) => <ExternalLink key={`${link.kind}-${link.url}`} url={link.url} label={link.label} />) : <p className="muted">연결된 GitHub 링크가 없습니다.</p>}</div></section>
  </div>
}

function repositoryFor(url?: string) {
  if (!url) return undefined
  try {
    const parsed = new URL(url)
    const parts = parsed.pathname.split('/').filter(Boolean)
    return parsed.protocol === 'https:' && parsed.hostname && parts.length >= 2 ? `${parsed.hostname}/${parts[0]}/${parts[1]}` : undefined
  } catch { return undefined }
}

function primaryWorkURL(work: WorkItem) {
  return work.github?.url ?? work.links?.find((link) => ['github', 'issue', 'pull_request', 'project_item'].includes(link.kind))?.url
}

function isTrustedProjectLink(link: Link) {
  if (link.kind !== 'github' || link.label !== 'Project') return false
  try {
    const parsed = new URL(link.url)
    const parts = parsed.pathname.split('/').filter(Boolean)
    return parsed.protocol === 'https:' && Boolean(parsed.hostname) && parts.length === 4 && ['users', 'orgs'].includes(parts[0]) && parts[2] === 'projects' && /^\d+$/.test(parts[3])
  } catch { return false }
}

function connectionsForWork(work: WorkItem | undefined, project: Project | undefined, herdr: HerdrSnapshot | undefined) {
  if (!work || !herdr) return []
  const primaryURL = primaryWorkURL(work)
  const repository = repositoryFor(primaryURL)
  const projectURLs = new Set((project?.links ?? []).filter(isTrustedProjectLink).map((link) => link.url))
  const issueConnections = herdr.connections.filter((connection) => connection.issueUrl && connection.issueUrl === primaryURL)
  if (issueConnections.length > 0) return issueConnections
  const projectConnections = herdr.connections.filter((connection) => connection.projectUrl && projectURLs.has(connection.projectUrl))
  if (projectConnections.length > 0) return projectConnections
  return herdr.connections.filter((connection) => !connection.issueUrl && !connection.projectUrl && connection.role === 'coordinator' && Boolean(repository) && connection.repository === repository)
}

function herdrGuidance(connection: HerdrConnection) {
  if (connection.status === 'connected' && connection.agentStatus === 'working') return '해당 세션에서 작업 중입니다.'
  if (connection.status === 'connected' && connection.agentStatus === 'blocked') return '세션의 질문과 필요한 승인을 확인하세요.'
  if (connection.status === 'connected' && (connection.agentStatus === 'idle' || connection.agentStatus === 'done')) return 'GitHub 기록과 남은 작업을 확인하세요.'
  if (connection.status === 'missing') return '보존된 변경과 handoff를 확인하세요.'
  return '세션 위치와 관찰 상태를 재확인하세요.'
}

function HerdrConnections({ connections }: { connections: HerdrConnection[] }) {
  if (connections.length === 0) return <p className="muted">선택한 업무에 명시된 Herdr 연결이 없습니다.</p>
  return <div className="ruled-list">{connections.map((connection, index) => <div className="evidence-row" key={`${connection.session}-${connection.paneId ?? index}`}><div><strong>{connection.session}</strong><small>{[connection.location?.workspaceId && `workspace ${connection.location.workspaceId}`, connection.location?.tabId && `tab ${connection.location.tabId}`, connection.location?.paneId && `pane ${connection.location.paneId}`, connection.location?.cwd && `cwd ${connection.location.cwd}`].filter(Boolean).join(' · ') || '위치 없음'}{connection.observedAt ? ` · 관찰 ${dateFor(connection.observedAt)}` : ''}</small></div><span>{labelFor(connection.status)}{connection.agentStatus ? ` · ${labelFor(connection.agentStatus)}` : ''}<small>{herdrGuidance(connection)}</small></span></div>)}</div>
}

type ProjectDetailTab = 'work' | 'toolbox'

function WorkDetail({ project, selectedWorkId, onSelectWork, onStatus }: { project: Project; selectedWorkId: string | null; onSelectWork: (id: string) => void; onStatus: (message: string) => void }) {
  const [detailTab, setDetailTab] = useState<ProjectDetailTab>('work')
  useEffect(() => setDetailTab('work'), [project.projectId])
  const work = project.workItems.find((item) => item.workId === selectedWorkId) ?? project.workItems[0]
  return <section className="detail" aria-labelledby="detail-title">
    <div className="detail-heading"><div><h2 id="detail-title">{project.name}</h2><p>최근 동기화 {dateFor(project.updatedAt)} · {labelFor(project.syncStatus)}</p></div>{work && <span className="state-badge"><StatusMark value={work.state} />{labelFor(work.state)}</span>}</div>
    {project.notices && project.notices.length > 0 && <div className="project-notices">{project.notices.map((notice, index) => <p key={`${notice}-${index}`}>{notice}</p>)}</div>}
    <div className="project-detail-tabs" role="tablist" aria-label="프로젝트 상세 보기">
      <button type="button" role="tab" aria-selected={detailTab === 'work'} onClick={() => setDetailTab('work')}>업무</button>
      <button type="button" role="tab" aria-selected={detailTab === 'toolbox'} onClick={() => setDetailTab('toolbox')}>Toolbox</button>
    </div>
    {detailTab === 'toolbox'
      ? <ProjectToolbox projectKey={toolboxKeyFor(project)} projectName={project.name} onStatus={onStatus} />
      : work
        ? <><WorkTable items={project.workItems} selected={work.workId} onSelect={onSelectWork} /><article className="work-summary"><h3>{work.title}</h3>{work.request && <MarkdownBody value={work.request} />}{work.blocker && <p className="blocker"><strong>보존된 변경</strong> {work.blocker}</p>}</article><EvidenceList work={work} project={project} /></>
        : <section className="empty-detail"><p>아직 표시할 업무가 없습니다.</p></section>}
  </section>
}

function buildHandoffText(work?: WorkItem, herdrConnections: HerdrConnection[] = []): string {
  if (!work) return ''
  const handoff = work.handoffs?.[0]
  const links = work.links?.filter((link) => isSafeExternalURL(link.url)).map((link) => `${link.label}: ${link.url}`) ?? []
  const nextAction = work.nextAction && work.nextAction !== 'review' ? `다음 행동: ${labelFor(work.nextAction)}` : ''
  return [`업무: ${work.title}`, nextAction, handoff ? `handoff: ${handoff.summary}` : '', ...work.evidenceRefs.map((ref) => `근거: ${ref}`), ...links, ...herdrConnections.map((connection) => `Herdr: ${connection.handoff}`)].filter(Boolean).join('\n')
}

const degradedConnectionStates = new Set(['stale', 'offline', 'degraded', 'setup_required', 'disabled', 'unverified', 'unknown', 'failed'])
const hasDegradedConnectionState = (...states: Array<string | undefined>) => states.some((state) => state !== undefined && degradedConnectionStates.has(state))

function ActionRail({ snapshot, project, work, herdr, onAction }: { snapshot: Snapshot; project?: Project; work?: WorkItem; herdr?: HerdrSnapshot; onAction: (message: string) => void }) {
  const state = snapshot.freshness.state === 'stale' || snapshot.syncStatus === 'offline' || snapshot.syncStatus === 'degraded' || snapshot.syncStatus === 'setup_required'
  const github = snapshot.source === 'github'
  const workIdentity = work?.github?.number ? `${work.github.kind === 'pull_request' ? 'PR' : work.github.kind === 'issue' ? 'Issue' : 'Project'} #${work.github.number}` : work ? '작업 선택됨' : '작업을 선택하세요'
  return <aside className="action-rail" aria-labelledby="action-title">
    <h2 id="action-title">선택한 작업</h2><p className="rail-intro">GitHub 근거와 연결된 Herdr 위치를 빠르게 확인합니다.</p>
    <div className="attention-block"><strong>{state ? '연결 확인 필요' : workIdentity}</strong><p>{state ? github ? 'GitHub 조회에 실패했거나 일부 결과가 오래되었습니다. 마지막 성공 데이터를 확인하세요.' : '마지막으로 확인한 상태를 보존했습니다. WSL 연결을 확인한 뒤 새로고침하세요.' : project ? `${project.name}의 GitHub 상태와 Herdr 연결을 확인할 수 있습니다.` : '프로젝트를 선택하면 작업 정보를 보여드립니다.'}</p></div>
    <button type="button" className="primary-action" onClick={async () => {
      const handoffText = buildHandoffText(work, connectionsForWork(work, project, herdr))
      if (!handoffText) { onAction('복사할 작업 정보가 없습니다.'); return }
      const wailsClipboard = window.runtime?.ClipboardSetText
      if (wailsClipboard) {
        try { if (await wailsClipboard(handoffText)) onAction('선택한 작업 정보를 클립보드에 복사했습니다.'); else onAction('작업 정보를 복사하지 못했습니다. GitHub 링크를 열어 내용을 확인하세요.') } catch { onAction('작업 정보를 복사하지 못했습니다. GitHub 링크를 열어 내용을 확인하세요.') }
        return
      }
      if (!navigator.clipboard?.writeText) { onAction('클립보드를 사용할 수 없습니다. 작업 정보를 직접 선택해 복사하세요.'); return }
      try { await navigator.clipboard.writeText(handoffText); onAction('선택한 작업 정보를 클립보드에 복사했습니다.') } catch { onAction('작업 정보를 복사하지 못했습니다. GitHub 링크를 열어 내용을 확인하세요.') }
    }}>작업 정보 복사</button>
    {work?.links?.find((link) => isSafeExternalURL(link.url)) && <button type="button" className="secondary-action" onClick={() => { const link = work.links?.find((item) => isSafeExternalURL(item.url)); if (link) openExternalURL(link.url) }}>GitHub에서 보기</button>}
  </aside>
}

function HerdrPanel({ herdr, project, work }: { herdr: HerdrSnapshot; project?: Project; work?: WorkItem }) {
  const selected = connectionsForWork(work, project, herdr)
  return <section className="herdr-panel" aria-labelledby="herdr-panel-title"><div className="section-heading"><div><h2 id="herdr-panel-title">Herdr 연결</h2><p>마지막 관찰 {dateFor(herdr.observedAt)} · {labelFor(herdr.status)}</p></div><span className="count-label">{herdr.sessions.length}개 세션</span></div>{herdr.notices.length > 0 && <div className="project-notices">{herdr.notices.map((notice, index) => <p key={`${notice}-${index}`}>{notice}</p>)}</div>}{(selected.length > 0 || work) && <section aria-labelledby="selected-herdr-title"><h3 id="selected-herdr-title">선택 업무 위치</h3><HerdrConnections connections={selected} /></section>}<section aria-labelledby="sessions-title"><h3 id="sessions-title">관찰한 세션</h3><div className="ruled-list">{herdr.sessions.length === 0 ? <p className="muted">관찰한 세션이 없습니다.</p> : herdr.sessions.map((session) => <div className="evidence-row" key={session.session}><div><strong>{session.session}</strong><small>{session.observedAt ? `관찰 ${dateFor(session.observedAt)}` : '관찰 시각 없음'}</small></div><span>{labelFor(session.status)} · {session.agents.length}개 Agent</span></div>)}</div></section>{herdr.unconnectedAgents.length > 0 && <section aria-labelledby="unconnected-title"><h3 id="unconnected-title">연결되지 않은 Agent</h3><div className="ruled-list">{herdr.unconnectedAgents.map((agent, index) => <div className="evidence-row" key={`${agent.session}-${agent.pane_id ?? index}`}><div><strong>{agent.name || '이름 없음'}</strong><small>세션 {agent.session}{agent.pane_id ? ` · pane ${agent.pane_id}` : ''}{agent.cwd ? ` · cwd ${agent.cwd}` : ''}</small></div><span>{labelFor(agent.agent_status || 'unknown')}</span></div>)}</div></section>}</section>
}

type WorkScope = 'open' | 'all'

export function App({ snapshotSource = getMonitorSnapshot, allSnapshotSource = getMonitorSnapshotAll, pollIntervalMs = 4000 }: { snapshotSource?: SnapshotSource; allSnapshotSource?: SnapshotSource; pollIntervalMs?: number }) {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null)
  const [selectedWorkId, setSelectedWorkId] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState('')
  const [workScope, setWorkScope] = useState<WorkScope>('open')
  const inFlight = useRef(false)
  const refresh = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    try {
      const next = await (workScope === 'all' ? allSnapshotSource : snapshotSource)()
      setSnapshot(next)
      setError(null)
      setSelectedProjectId((current) => {
        const selectedProject = next.projects.find((project) => project.projectId === current) ?? sortProjects(next.projects)[0]
        setSelectedWorkId((workId) => selectedProject?.workItems.some((work) => work.workId === workId) ? workId : selectedProject?.workItems[0]?.workId ?? null)
        return selectedProject?.projectId ?? null
      })
    } catch (cause) {
      setError(cause instanceof Error ? cause : new Error('unknown monitor error'))
    } finally {
      setLoading(false)
      inFlight.current = false
    }
  }, [allSnapshotSource, snapshotSource, workScope])
  useEffect(() => {
    void refresh()
    if (workScope === 'all') return
    const timer = window.setInterval(() => void refresh(), pollIntervalMs)
    return () => window.clearInterval(timer)
  }, [refresh, pollIntervalMs, workScope])
  const projects = useMemo(() => sortProjects(snapshot?.projects ?? []), [snapshot])
  const selected = projects.find((project) => project.projectId === selectedProjectId)
  const isGithub = snapshot?.source === 'github'
  const isDegraded = snapshot?.freshness.state === 'stale' || snapshot?.syncStatus === 'offline' || snapshot?.syncStatus === 'degraded' || snapshot?.syncStatus === 'setup_required'
  const isHerdrDegraded = snapshot?.herdr ? hasDegradedConnectionState(snapshot.herdr.status, snapshot.herdr.state, snapshot.herdr.syncStatus, snapshot.herdr.freshness.state, snapshot.herdr.freshness.syncStatus) : false
  const isLocalConnectionDegraded = isDegraded || isHerdrDegraded

  return <div className="monitor-shell">
    <nav className="top-nav" aria-label="주요 메뉴"><div className="brand" aria-label="ThreadDock Monitor">Thread<span>Dock</span></div><div className="connection"><span className={`status-mark ${isLocalConnectionDegraded ? 'attention' : 'success'}`} aria-hidden="true" />로컬 연결 {isLocalConnectionDegraded ? '확인 필요' : '정상'}</div></nav>
    <main className="main-content">
      {loading && !snapshot ? <LoadingState /> : error && (!snapshot || snapshot.projects.length === 0) ? <section className="state-panel error-state" role="alert"><h1>상태를 불러오지 못했습니다.</h1><p>{isGithub ? 'GitHub Monitor 연결을 확인하고 잠시 후 다시 시도하세요.' : 'Windows Wails Monitor 설정과 연결을 확인하고 잠시 후 다시 시도하세요.'}</p><button type="button" className="secondary-action" onClick={() => void refresh()}>다시 시도</button></section> : snapshot && <>
        {isDegraded && <div className="degraded-banner" role="status"><strong>{isGithub ? 'GitHub 조회 결과 일부가 오래되었습니다.' : '오래된 상태를 표시하고 있습니다.'}</strong><span><strong>{isGithub ? 'GitHub 연결 확인 필요' : 'WSL 연결 오프라인'}</strong> · 마지막 관찰 {dateFor(snapshot.observedAt)}{snapshot.freshness.lastSyncedAt ? ` · 마지막 성공 ${dateFor(snapshot.freshness.lastSyncedAt)}` : ''}</span></div>}
        {snapshot.notices && snapshot.notices.length > 0 && <section className="monitor-notices" aria-label="GitHub Monitor 안내">{snapshot.notices.map((notice, index) => <p key={`${notice}-${index}`}>{notice}</p>)}</section>}
        <div className="monitor-toolbar">{isGithub && <><div className="work-scope" aria-label="업무 조회 범위"><button type="button" className={`scope-action${workScope === 'open' ? ' active' : ''}`} aria-pressed={workScope === 'open'} onClick={() => setWorkScope('open')}>열린 항목</button><button type="button" className={`scope-action${workScope === 'all' ? ' active' : ''}`} aria-pressed={workScope === 'all'} onClick={() => setWorkScope('all')}>전체 보기</button></div><button type="button" className="secondary-action refresh-action" onClick={() => void refresh()}>GitHub 새로고침</button></>}</div>
        {projects.length === 0 ? <section className="state-panel empty-state"><h1>{isGithub && snapshot.syncStatus === 'setup_required' ? 'GitHub Monitor 설정이 필요합니다.' : '표시할 프로젝트가 없습니다.'}</h1><p>{isGithub ? 'Windows Wails Monitor 설정에서 저장소·Projects URL·WSL 배포판을 지정하면 Issue, PR, Project 정보를 표시합니다.' : 'Windows Wails Monitor가 관찰 결과를 반환하면 이곳에 프로젝트와 작업이 표시됩니다.'}</p>{isGithub && <button type="button" className="secondary-action" onClick={() => void refresh()}>다시 확인</button>}</section> : <><ProjectList projects={projects} selected={selectedProjectId} onSelect={(id) => { setSelectedProjectId(id); setSelectedWorkId(projects.find((project) => project.projectId === id)?.workItems[0]?.workId ?? null) }} />{selected && <WorkDetail project={selected} selectedWorkId={selectedWorkId} onSelectWork={setSelectedWorkId} onStatus={setStatusMessage} />}</>}
        {snapshot.herdr && <HerdrPanel herdr={snapshot.herdr} project={selected} work={selected?.workItems.find((item) => item.workId === selectedWorkId) ?? selected?.workItems[0]} />}
      </>}
    </main>
    {snapshot && <ActionRail snapshot={snapshot} project={selected} work={selected?.workItems.find((item) => item.workId === selectedWorkId) ?? selected?.workItems[0]} herdr={snapshot.herdr} onAction={setStatusMessage} />}
    <div className="live-status" role="status" aria-live="polite">{statusMessage}</div>
  </div>
}
