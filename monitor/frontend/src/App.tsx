import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { HerdrConnection, HerdrSnapshot, Project, Snapshot, SnapshotSource } from './types'
import { getGlobalToolbox, getMonitorSnapshot, getMonitorSnapshotAll, saveGlobalToolbox, type GlobalToolbox } from './bindings'
import { GlobalToolboxDrawer } from './GlobalToolboxDrawer'
import { ProjectDetail } from './ProjectDetail'
import { TopBar } from './TopBar'
import { connectionsForWork, dateFor, herdrGuidance, labelFor } from './monitor-presentation'
import { toolboxKeyFor } from './project-key'
import './styles.css'

export const sortProjects = (projects: Project[]): Project[] => [...projects].sort((a, b) => {
  const attention = (state: string) => state === 'needs_operator' ? 0 : 1
  const priority = attention(a.state) - attention(b.state)
  if (priority !== 0) return priority
  return (Date.parse(b.updatedAt ?? '') || 0) - (Date.parse(a.updatedAt ?? '') || 0)
})

const timeFor = (value?: string) => value ? new Intl.DateTimeFormat('ko-KR', { hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—'

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

function HerdrConnections({ connections }: { connections: HerdrConnection[] }) {
  if (connections.length === 0) return <p className="muted">선택한 업무에 명시된 Herdr 연결이 없습니다.</p>
  return <div className="ruled-list">{connections.map((connection, index) => <div className="evidence-row" key={`${connection.session}-${connection.paneId ?? index}`}><div><strong>{connection.session}</strong><small>{[connection.location?.workspaceId && `workspace ${connection.location.workspaceId}`, connection.location?.tabId && `tab ${connection.location.tabId}`, connection.location?.paneId && `pane ${connection.location.paneId}`, connection.location?.cwd && `cwd ${connection.location.cwd}`].filter(Boolean).join(' · ') || '위치 없음'}{connection.observedAt ? ` · 관찰 ${dateFor(connection.observedAt)}` : ''}</small></div><span>{labelFor(connection.status)}{connection.agentStatus ? ` · ${labelFor(connection.agentStatus)}` : ''}<small>{herdrGuidance(connection)}</small></span></div>)}</div>
}

function HerdrPanel({ herdr, project, work }: { herdr: HerdrSnapshot; project?: Project; work?: Project['workItems'][number] }) {
  const selected = connectionsForWork(work, project, herdr)
  return <section className="herdr-panel" aria-labelledby="herdr-panel-title"><div className="section-heading"><div><h2 id="herdr-panel-title">Herdr 연결</h2><p>마지막 관찰 {dateFor(herdr.observedAt)} · {labelFor(herdr.status)}</p></div><span className="count-label">{herdr.sessions.length}개 세션</span></div>{herdr.notices.length > 0 && <div className="project-notices">{herdr.notices.map((notice, index) => <p key={`${notice}-${index}`}>{notice}</p>)}</div>}{(selected.length > 0 || work) && <section aria-labelledby="selected-herdr-title"><h3 id="selected-herdr-title">선택 업무 위치</h3><HerdrConnections connections={selected} /></section>}<section aria-labelledby="sessions-title"><h3 id="sessions-title">관찰한 세션</h3><div className="ruled-list">{herdr.sessions.length === 0 ? <p className="muted">관찰한 세션이 없습니다.</p> : herdr.sessions.map((session) => <div className="evidence-row" key={session.session}><div><strong>{session.session}</strong><small>{session.observedAt ? `관찰 ${dateFor(session.observedAt)}` : '관찰 시각 없음'}</small></div><span>{labelFor(session.status)} · {session.agents.length}개 Agent</span></div>)}</div></section>{herdr.unconnectedAgents.length > 0 && <section aria-labelledby="unconnected-title"><h3 id="unconnected-title">연결되지 않은 Agent</h3><div className="ruled-list">{herdr.unconnectedAgents.map((agent, index) => <div className="evidence-row" key={`${agent.session}-${agent.pane_id ?? index}`}><div><strong>{agent.name || '이름 없음'}</strong><small>세션 {agent.session}{agent.pane_id ? ` · pane ${agent.pane_id}` : ''}{agent.cwd ? ` · cwd ${agent.cwd}` : ''}</small></div><span>{labelFor(agent.agent_status || 'unknown')}</span></div>)}</div></section>}</section>
}

type WorkScope = 'open' | 'all'
const degradedConnectionStates = new Set(['stale', 'offline', 'degraded', 'setup_required', 'disabled', 'unverified', 'unknown', 'failed'])
const hasDegradedConnectionState = (...states: Array<string | undefined>) => states.some((state) => state !== undefined && degradedConnectionStates.has(state))

export function App({ snapshotSource = getMonitorSnapshot, allSnapshotSource = getMonitorSnapshotAll, pollIntervalMs = 4000, onOpenSettings = () => undefined }: { snapshotSource?: SnapshotSource; allSnapshotSource?: SnapshotSource; pollIntervalMs?: number; onOpenSettings?: () => void }) {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null)
  const [selectedWorkId, setSelectedWorkId] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState('')
  const [workScope, setWorkScope] = useState<WorkScope>('open')
  const inFlight = useRef(false)
  const [toolbox, setToolbox] = useState<GlobalToolbox | null>(null)
  const [toolboxOpen, setToolboxOpen] = useState(false)
  const [toolboxProjectFilter, setToolboxProjectFilter] = useState<string | undefined>()
  const [toolboxLoading, setToolboxLoading] = useState(true)
  const [toolboxLoadError, setToolboxLoadError] = useState('')
  const [toolboxSaving, setToolboxSaving] = useState(false)
  const toolboxLoadStarted = useRef(false)
  const toolboxLoadInFlight = useRef(false)
  const toolboxSaveInFlight = useRef(false)
  const toolboxOperationRevision = useRef(0)

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

  const loadToolbox = useCallback(async () => {
    if (toolboxLoadInFlight.current || toolboxSaveInFlight.current) return
    const revision = ++toolboxOperationRevision.current
    toolboxLoadInFlight.current = true
    setToolboxLoading(true)
    setToolboxLoadError('')
    try {
      const loaded = await getGlobalToolbox()
      if (revision === toolboxOperationRevision.current) setToolbox(loaded)
    } catch (cause) {
      if (revision === toolboxOperationRevision.current) setToolboxLoadError(cause instanceof Error ? cause.message : 'Toolbox를 불러오지 못했습니다.')
    } finally {
      if (revision === toolboxOperationRevision.current) setToolboxLoading(false)
      toolboxLoadInFlight.current = false
    }
  }, [])

  const saveToolbox = useCallback(async (next: GlobalToolbox) => {
    if (toolboxSaveInFlight.current) throw new Error('Toolbox 저장이 이미 진행 중입니다.')
    const revision = ++toolboxOperationRevision.current
    toolboxSaveInFlight.current = true
    setToolboxSaving(true)
    try {
      const saved = await saveGlobalToolbox(next)
      if (revision === toolboxOperationRevision.current) setToolbox(saved)
      return saved
    } finally {
      setToolboxSaving(false)
      toolboxSaveInFlight.current = false
    }
  }, [])

  useEffect(() => {
    void refresh()
    if (workScope === 'all') return
    const timer = window.setInterval(() => void refresh(), pollIntervalMs)
    return () => window.clearInterval(timer)
  }, [refresh, pollIntervalMs, workScope])

  useEffect(() => {
    if (toolboxLoadStarted.current) return
    toolboxLoadStarted.current = true
    void loadToolbox()
  }, [loadToolbox])

  const projects = useMemo(() => sortProjects(snapshot?.projects ?? []), [snapshot])
  const selected = projects.find((project) => project.projectId === selectedProjectId)
  const selectedWork = selected?.workItems.find((item) => item.workId === selectedWorkId) ?? selected?.workItems[0]
  const isGithub = snapshot?.source === 'github'
  const isDegraded = snapshot?.freshness.state === 'stale' || snapshot?.syncStatus === 'offline' || snapshot?.syncStatus === 'degraded' || snapshot?.syncStatus === 'setup_required'
  const isHerdrDegraded = snapshot?.herdr ? hasDegradedConnectionState(snapshot.herdr.status, snapshot.herdr.state, snapshot.herdr.syncStatus, snapshot.herdr.freshness.state, snapshot.herdr.freshness.syncStatus) : false
  const isLocalConnectionDegraded = isDegraded || isHerdrDegraded
  const projectTodoCount = selected ? toolbox?.todos.filter((todo) => !todo.done && todo.projectKey === toolboxKeyFor(selected)).length ?? 0 : 0
  const openToolbox = () => { if (toolboxLoadError && !toolboxLoadInFlight.current) void loadToolbox(); setToolboxProjectFilter(undefined); setToolboxOpen(true) }
  const openProjectTodos = (projectKey: string) => { if (toolboxLoadError && !toolboxLoadInFlight.current) void loadToolbox(); setToolboxProjectFilter(projectKey); setToolboxOpen(true) }

  return <div className="monitor-shell">
    <TopBar connectionDegraded={isLocalConnectionDegraded} onOpenToolbox={openToolbox} onOpenSettings={onOpenSettings} />
    <main className="main-content">
      {loading && !snapshot ? <LoadingState /> : error && (!snapshot || snapshot.projects.length === 0) ? <section className="state-panel error-state" role="alert"><h1>상태를 불러오지 못했습니다.</h1><p>{isGithub ? 'GitHub Monitor 연결을 확인하고 잠시 후 다시 시도하세요.' : 'Windows Wails Monitor 설정과 연결을 확인하고 잠시 후 다시 시도하세요.'}</p><button type="button" className="secondary-action" onClick={() => void refresh()}>다시 시도</button></section> : snapshot && <>
        {isDegraded && <div className="degraded-banner" role="status"><strong>{isGithub ? 'GitHub 조회 결과 일부가 오래되었습니다.' : '오래된 상태를 표시하고 있습니다.'}</strong><span><strong>{isGithub ? 'GitHub 연결 확인 필요' : 'WSL 연결 오프라인'}</strong> · 마지막 관찰 {dateFor(snapshot.observedAt)}{snapshot.freshness.lastSyncedAt ? ` · 마지막 성공 ${dateFor(snapshot.freshness.lastSyncedAt)}` : ''}</span></div>}
        {snapshot.notices && snapshot.notices.length > 0 && <section className="monitor-notices" aria-label="GitHub Monitor 안내">{snapshot.notices.map((notice, index) => <p key={`${notice}-${index}`}>{notice}</p>)}</section>}
        <div className="monitor-toolbar">{isGithub && <><div className="work-scope" aria-label="업무 조회 범위"><button type="button" className={`scope-action${workScope === 'open' ? ' active' : ''}`} aria-pressed={workScope === 'open'} onClick={() => setWorkScope('open')}>열린 항목</button><button type="button" className={`scope-action${workScope === 'all' ? ' active' : ''}`} aria-pressed={workScope === 'all'} onClick={() => setWorkScope('all')}>전체 보기</button></div><button type="button" className="secondary-action refresh-action" onClick={() => void refresh()}>GitHub 새로고침</button></>}</div>
        {projects.length === 0 ? <section className="state-panel empty-state"><h1>{isGithub && snapshot.syncStatus === 'setup_required' ? 'GitHub Monitor 설정이 필요합니다.' : '표시할 프로젝트가 없습니다.'}</h1><p>{isGithub ? 'Windows Wails Monitor 설정에서 저장소·Projects URL·WSL 배포판을 지정하면 Issue, PR, Project 정보를 표시합니다.' : 'Windows Wails Monitor가 관찰 결과를 반환하면 이곳에 프로젝트와 작업이 표시됩니다.'}</p>{isGithub && <button type="button" className="secondary-action" onClick={() => void refresh()}>다시 확인</button>}</section> : <><ProjectList projects={projects} selected={selectedProjectId} onSelect={(id) => { setSelectedProjectId(id); setSelectedWorkId(projects.find((project) => project.projectId === id)?.workItems[0]?.workId ?? null) }} />{selected && <ProjectDetail project={selected} selectedWorkId={selectedWorkId} herdr={snapshot.herdr} projectTodoCount={projectTodoCount} onSelectWork={setSelectedWorkId} onOpenProjectTodos={openProjectTodos} onStatus={setStatusMessage} />}</>}
        {snapshot.herdr && <HerdrPanel herdr={snapshot.herdr} project={selected} work={selectedWork} />}
      </>}
    </main>
    <GlobalToolboxDrawer open={toolboxOpen} projects={projects} initialProjectKey={toolboxProjectFilter} toolbox={toolbox} loadError={toolboxLoadError || undefined} loading={toolboxLoading} saving={toolboxSaving} onReload={() => void loadToolbox()} onSave={saveToolbox} onClose={() => setToolboxOpen(false)} onStatus={setStatusMessage} />
    <div className="live-status" role="status" aria-live="polite">{statusMessage}</div>
  </div>
}
