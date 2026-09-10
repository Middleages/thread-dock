import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Project, Snapshot, SnapshotSource, WorkItem } from './types'
import { getMonitorSnapshot } from './bindings'
import { isSafeExternalURL, openExternalURL } from './safe-url'
import './styles.css'

export const sortProjects = (projects: Project[]): Project[] => [...projects].sort((a, b) => {
  const attention = (state: string) => state === 'needs_operator' ? 0 : 1
  const priority = attention(a.state) - attention(b.state)
  if (priority !== 0) return priority
  return (Date.parse(b.updatedAt ?? '') || 0) - (Date.parse(a.updatedAt ?? '') || 0)
})

const stateLabel: Record<string, string> = {
  needs_operator: '판단 필요', running: '진행 중', completed: '완료', verified: '검증 완료',
  review: '독립 확인', synced: '동기화됨', stale: '오래된 상태', offline: '오프라인',
  failed: '확인 필요', blocked: '판단 필요', ready: '준비됨', passed: '통과', approved: '승인됨',
}

const labelFor = (value: string) => stateLabel[value] ?? value.replaceAll('_', ' ')
const timeFor = (value?: string) => value ? new Intl.DateTimeFormat('ko-KR', { hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—'
const dateFor = (value?: string) => value ? new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '시각 없음'

declare global {
  interface Window { runtime?: { BrowserOpenURL?: (url: string) => void } }
}

function ExternalLink({ url, label }: { url: string; label: string }) {
  if (!isSafeExternalURL(url)) return <span className="blocked-link">{label}</span>
  const onClick = (event: React.MouseEvent<HTMLAnchorElement>) => {
    if (window.runtime?.BrowserOpenURL) { event.preventDefault(); openExternalURL(url) }
  }
  return <a href={url} target="_blank" rel="noreferrer" onClick={onClick}>{label}</a>
}

function StatusMark({ value }: { value: string }) {
  const tone = value === 'needs_operator' || value === 'failed' || value === 'blocked' ? 'attention' : value === 'completed' || value === 'verified' || value === 'synced' ? 'success' : 'progress'
  return <span className={`status-mark ${tone}`} aria-hidden="true" />
}

function LoadingState() {
  return <div className="loading-state" role="status" aria-live="polite"><span className="skeleton skeleton-title" /><span className="skeleton skeleton-row" /><span className="skeleton skeleton-row" /><p>프로젝트 현황을 불러오는 중입니다.</p></div>
}

function ProjectList({ projects, selected, onSelect }: { projects: Project[]; selected: string | null; onSelect: (id: string) => void }) {
  return <section className="ledger" aria-labelledby="ledger-title">
    <div className="section-heading"><div><h1 id="ledger-title">오늘의 작업 원장</h1><p>판단이 필요한 프로젝트와 최근 변경을 먼저 보여줍니다.</p></div><span className="count-label">{projects.length}개 프로젝트</span></div>
    <div className="project-table" role="table" aria-label="프로젝트 목록">
      <div className="table-head" role="row"><span role="columnheader">최근 활동</span><span role="columnheader">프로젝트</span><span role="columnheader">현재 단계</span><span role="columnheader">다음 행동</span></div>
      {projects.map((project) => <button key={project.projectId} type="button" className={`project-row${selected === project.projectId ? ' selected' : ''}`} aria-pressed={selected === project.projectId} onClick={() => onSelect(project.projectId)} autoFocus={selected === project.projectId && projects[0] === project}>
        <time role="cell" dateTime={project.updatedAt}>{timeFor(project.updatedAt)}</time>
        <span role="cell" className="project-name"><strong>{project.name}</strong><small>{project.workItems.length}개 업무 · {dateFor(project.updatedAt)}</small></span>
        <span role="cell" className="state-cell"><StatusMark value={project.state} />{labelFor(project.state)}</span>
        <span role="cell" className="next-action">{labelFor(project.nextAction)}</span>
      </button>)}
    </div>
  </section>
}

function EvidenceList({ work }: { work: WorkItem }) {
  return <div className="evidence-sections">
    <section aria-labelledby="tasks-title"><h3 id="tasks-title">저장소별 작업</h3>{work.tasks.length === 0 ? <p className="muted">등록된 하위 작업이 없습니다.</p> : <div className="ruled-list">{work.tasks.map((task) => <div className="evidence-row" key={task.taskId}><div><strong>{task.repoKey}</strong><small>{task.taskId}</small></div><span>{labelFor(task.state)} · {task.verification ? labelFor(task.verification) : '검증 대기'}</span></div>)}</div>}</section>
    <section aria-labelledby="publication-title"><h3 id="publication-title">발행 상태</h3><div className="ruled-list">{work.publications.map((publication) => <div className="evidence-row" key={publication.intentId}><div><strong>{publication.key}</strong><small>{publication.kind} · {publication.attempts}회 시도</small></div><span>{labelFor(publication.status)}{publication.url && <ExternalLink url={publication.url} label="열기" />}</span></div>)}</div></section>
    {work.decisions && work.decisions.length > 0 && <section aria-labelledby="decisions-title"><h3 id="decisions-title">최근 결정</h3><div className="ruled-list">{work.decisions.map((decision) => <div className="evidence-row" key={decision.decisionId}><div><strong>{decision.summary}</strong><small>{dateFor(decision.observedAt)}</small></div><span>{labelFor(decision.status)}</span></div>)}</div></section>}
    {work.handoffs && work.handoffs.length > 0 && <section aria-labelledby="handoffs-title"><h3 id="handoffs-title">최근 handoff</h3><div className="ruled-list">{work.handoffs.map((handoff, index) => <div className="evidence-row" key={`${handoff.summary}-${index}`}><div><strong>{handoff.summary}</strong><small>{handoff.evidenceRefs.join(' · ')}</small></div><span>{labelFor(handoff.nextAction)}</span></div>)}</div></section>}
    <section aria-labelledby="links-title"><h3 id="links-title">관련 링크</h3><div className="link-list">{work.links && work.links.length > 0 ? work.links.map((link) => <ExternalLink key={`${link.kind}-${link.url}`} url={link.url} label={link.label} />) : <p className="muted">연결된 GitHub 링크가 없습니다.</p>}</div></section>
  </div>
}

function WorkDetail({ project, selectedWorkId, onSelectWork }: { project: Project; selectedWorkId: string | null; onSelectWork: (id: string) => void }) {
  const work = project.workItems.find((item) => item.workId === selectedWorkId) ?? project.workItems[0]
  if (!work) return <section className="detail empty-detail"><h2>{project.name}</h2><p>아직 표시할 업무가 없습니다.</p></section>
  return <section className="detail" aria-labelledby="detail-title">
    <div className="detail-heading"><div><h2 id="detail-title">{project.name}</h2><p>최근 동기화 {dateFor(project.updatedAt)} · {labelFor(project.syncStatus)}</p></div><span className="state-badge"><StatusMark value={work.state} />{labelFor(work.state)}</span></div>
    {project.workItems.length > 1 && <div className="work-tabs" role="tablist" aria-label="업무 선택">{project.workItems.map((item) => <button type="button" role="tab" aria-selected={item.workId === work.workId} key={item.workId} onClick={() => onSelectWork(item.workId)}>{item.title}</button>)}</div>}
    <article className="work-summary"><h3>{work.title}</h3>{work.request && <p>{work.request}</p>}<div className="next-step"><span>다음 행동</span><strong>{labelFor(work.nextAction)}</strong></div>{work.blocker && <p className="blocker"><strong>보존된 변경</strong> {work.blocker}</p>}</article>
    <EvidenceList work={work} />
  </section>
}

function buildHandoffText(work?: WorkItem): string {
  if (!work) return ''
  const handoff = work.handoffs?.[0]
  const links = work.links?.filter((link) => isSafeExternalURL(link.url)).map((link) => `${link.label}: ${link.url}`) ?? []
  return [`업무: ${work.title}`, `다음 행동: ${labelFor(work.nextAction)}`, handoff ? `handoff: ${handoff.summary}` : '', ...work.evidenceRefs.map((ref) => `근거: ${ref}`), ...links].filter(Boolean).join('\n')
}

function ActionRail({ snapshot, project, work, onAction }: { snapshot: Snapshot; project?: Project; work?: WorkItem; onAction: (message: string) => void }) {
  const state = snapshot.freshness.state === 'stale' || snapshot.syncStatus === 'offline'
  return <aside className="action-rail" aria-labelledby="action-title">
    <h2 id="action-title">지금 필요한 행동</h2><p className="rail-intro">중요한 요청만 여기에 표시합니다.</p>
    <div className="attention-block"><strong>{state ? '연결을 확인하세요' : work?.nextAction ? `다음 행동 · ${labelFor(work.nextAction)}` : '현재는 기다리세요'}</strong><p>{state ? '마지막으로 확인한 상태를 보존했습니다. WSL 연결을 확인한 뒤 새로고침하세요.' : project ? `${project.name}의 실행 상태와 근거를 검토하세요.` : '프로젝트를 선택하면 필요한 행동을 보여드립니다.'}</p></div>
    <button type="button" className="primary-action" onClick={async () => { if (!navigator.clipboard?.writeText) { onAction('클립보드를 사용할 수 없습니다. handoff 내용을 선택해 복사하세요.'); return } try { await navigator.clipboard.writeText(buildHandoffText(work)); onAction('선택한 업무의 handoff를 클립보드에 복사했습니다.') } catch { onAction('handoff를 복사하지 못했습니다. 근거 링크를 열어 내용을 전달하세요.') } }}>handoff 복사</button>
    {work?.links?.find((link) => isSafeExternalURL(link.url)) && <button type="button" className="secondary-action" onClick={() => { const link = work.links?.find((item) => isSafeExternalURL(item.url)); if (link) openExternalURL(link.url) }}>GitHub에서 보기</button>}
    <section className="rail-workflows" aria-labelledby="workflow-title"><h3 id="workflow-title">자동화 작업</h3><div className="workflow-row"><strong>상태 집계</strong><span className="success-text">정상</span><small>4초마다 자동 · GitHub API 호출 없음</small></div><div className="workflow-row"><strong>동기화</strong><span className={state ? 'attention-text' : 'success-text'}>{state ? '오프라인' : '정상'}</span><small>{state ? '마지막 상태 보존' : '마지막 확인 완료'}</small></div></section>
  </aside>
}

export function App({ snapshotSource = getMonitorSnapshot, pollIntervalMs = 4000 }: { snapshotSource?: SnapshotSource; pollIntervalMs?: number }) {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null)
  const [selectedWorkId, setSelectedWorkId] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState('')
  const inFlight = useRef(false)
  const refresh = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    try {
      const next = await snapshotSource()
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
  }, [snapshotSource])
  useEffect(() => { void refresh(); const timer = window.setInterval(() => void refresh(), pollIntervalMs); return () => window.clearInterval(timer) }, [refresh, pollIntervalMs])
  const projects = useMemo(() => sortProjects(snapshot?.projects ?? []), [snapshot])
  const selected = projects.find((project) => project.projectId === selectedProjectId)
  const isDegraded = snapshot?.freshness.state === 'stale' || snapshot?.syncStatus === 'offline'

  return <div className="monitor-shell">
    <nav className="top-nav" aria-label="주요 메뉴"><div className="brand" aria-label="ThreadDock Monitor">Thread<span>Dock</span></div><button type="button" className="nav-item active" autoFocus>작업 현황</button><button type="button" className="nav-item" onClick={() => setStatusMessage('저장소 연결은 상세 업무에서 확인합니다.')}>저장소</button><button type="button" className="nav-item" onClick={() => setStatusMessage('자동화 작업은 오른쪽 행동 패널에서 확인합니다.')}>자동화 작업</button><button type="button" className="nav-item" onClick={() => setStatusMessage('완료 기록은 다음 범위에서 연결됩니다.')}>완료 기록</button><button type="button" className="nav-item" onClick={() => setStatusMessage('설정은 Trusted Workstation에서 관리합니다.')}>설정</button><div className="connection"><span className="status-mark success" aria-hidden="true" />로컬 연결 {isDegraded ? '확인 필요' : '정상'}</div></nav>
    <main className="main-content">
      {loading && !snapshot ? <LoadingState /> : error && (!snapshot || snapshot.projects.length === 0) ? <section className="state-panel error-state" role="alert"><h1>상태를 불러오지 못했습니다.</h1><p>agentctl 연결을 확인하고 잠시 후 다시 시도하세요.</p><button type="button" className="secondary-action" onClick={() => void refresh()}>다시 시도</button></section> : snapshot && <>
        {isDegraded && <div className="degraded-banner" role="status"><strong>오래된 상태를 표시하고 있습니다.</strong><span><strong>WSL 연결 오프라인</strong> · 마지막으로 확인한 시각 {dateFor(snapshot.observedAt)}</span></div>}
        {projects.length === 0 ? <section className="state-panel empty-state"><h1>표시할 프로젝트가 없습니다.</h1><p>agentctl project status 결과가 도착하면 이곳에 프로젝트와 다음 행동이 표시됩니다.</p></section> : <><ProjectList projects={projects} selected={selectedProjectId} onSelect={(id) => { setSelectedProjectId(id); setSelectedWorkId(projects.find((project) => project.projectId === id)?.workItems[0]?.workId ?? null) }} />{selected && <WorkDetail project={selected} selectedWorkId={selectedWorkId} onSelectWork={setSelectedWorkId} />}</>}
      </>}
    </main>
    {snapshot && <ActionRail snapshot={snapshot} project={selected} work={selected?.workItems.find((item) => item.workId === selectedWorkId) ?? selected?.workItems[0]} onAction={setStatusMessage} />}
    <div className="live-status" role="status" aria-live="polite">{statusMessage}</div>
  </div>
}
