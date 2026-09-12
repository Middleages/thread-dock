import { useEffect, useState } from 'react'
import type { HerdrConnection, HerdrSnapshot, Link, Project, WorkItem } from './types'
import { MarkdownBody } from './MarkdownBody'
import { ProjectReferences } from './ProjectReferences'
import { WorkTable } from './WorkTable'
import { toolboxKeyFor } from './project-key'
import { isSafeExternalURL, openExternalURL } from './safe-url'
import { connectionsForWork, dateFor, herdrGuidance, labelFor } from './monitor-presentation'
import './project-detail.css'

declare global {
  interface Window { runtime?: { BrowserOpenURL?: (url: string) => void; ClipboardSetText?: (text: string) => Promise<boolean> } }
}

const statusTone = (value: string) => value === 'needs_operator' || value === 'failed' || value === 'blocked' ? 'attention' : value === 'completed' || value === 'verified' || value === 'synced' || value === 'running' ? 'success' : 'progress'
const StatusMark = ({ value }: { value: string }) => <span className={`status-mark ${statusTone(value)}`} aria-hidden="true" />

function ExternalLink({ url, label }: { url: string; label: string }) {
  if (!isSafeExternalURL(url)) return <span className="blocked-link">{label}</span>
  const onClick = (event: React.MouseEvent<HTMLAnchorElement>) => {
    if (window.runtime?.BrowserOpenURL) { event.preventDefault(); openExternalURL(url) }
  }
  return <a href={url} target="_blank" rel="noreferrer" onClick={onClick}>{label}</a>
}

async function copyText(value: string) {
  const wailsClipboard = window.runtime?.ClipboardSetText
  if (wailsClipboard) {
    if (!(await wailsClipboard(value))) throw new Error('clipboard rejected')
    return
  }
  if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable')
  await navigator.clipboard.writeText(value)
}

function buildHandoffText(work?: WorkItem, herdrConnections: HerdrConnection[] = []) {
  if (!work) return ''
  const handoff = work.handoffs?.[0]
  const links = work.links?.filter((link) => isSafeExternalURL(link.url)).map((link) => `${link.label}: ${link.url}`) ?? []
  const nextAction = work.nextAction && work.nextAction !== 'review' ? `다음 행동: ${labelFor(work.nextAction)}` : ''
  return [`업무: ${work.title}`, nextAction, handoff ? `handoff: ${handoff.summary}` : '', ...work.evidenceRefs.map((ref) => `근거: ${ref}`), ...links, ...herdrConnections.map((connection) => `Herdr: ${connection.handoff}`)].filter(Boolean).join('\n')
}

function GithubEvidence({ work }: { work: WorkItem }) {
  const github = work.github
  if (!github) return null
  const checks = github.checks ?? []
  const fields = Object.entries(github.fields ?? {})
  return <section aria-labelledby="github-state-title"><h3 id="github-state-title">GitHub 상태</h3><div className="ruled-list">
    <div className="evidence-row"><div><strong>{github.kind === 'pull_request' ? 'Pull request' : github.kind === 'issue' ? 'Issue' : 'Project item'}</strong><small>{github.number ? `#${github.number}` : '번호 없음'} · 관찰 {dateFor(github.observedAt)}</small></div><span>{labelFor((github.state ?? 'unknown').toLowerCase())}</span></div>
    {github.reviewDecision && <div className="evidence-row"><div><strong>리뷰</strong></div><span>{labelFor(github.reviewDecision)}</span></div>}
    {checks.length > 0 && <div className="evidence-row"><div><strong>검사</strong><small>{checks.map((check) => check.name).join(' · ')}</small></div><span>{checks.map((check) => labelFor((check.conclusion || check.status || 'unknown').toLowerCase())).join(' · ')}</span></div>}
    {github.relatedIssueUrls && github.relatedIssueUrls.length > 0 && <div className="evidence-row"><div><strong>연결된 Issue</strong></div><span>{github.relatedIssueUrls.map((url) => <ExternalLink key={url} url={url} label="Issue" />)}</span></div>}
    {github.relatedPullRequestUrls && github.relatedPullRequestUrls.length > 0 && <div className="evidence-row"><div><strong>연결된 PR</strong></div><span>{github.relatedPullRequestUrls.map((url) => <ExternalLink key={url} url={url} label="PR" />)}</span></div>}
    {fields.length > 0 && <div className="evidence-row"><div><strong>Project 필드</strong></div><span>{fields.map(([name, value]) => <span key={name}>{name}: {value || '값 없음'}</span>)}</span></div>}
  </div></section>
}

function EvidenceList({ work, project }: { work: WorkItem; project: Project }) {
  const links = [...(project.links ?? []), ...(work.links ?? [])]
  return <div className="evidence-sections">
    {work.github && <GithubEvidence work={work} />}
    {work.decisions && work.decisions.length > 0 && <section aria-labelledby="decisions-title"><h3 id="decisions-title">최근 결정</h3><div className="ruled-list">{work.decisions.map((decision) => <div className="evidence-row" key={decision.decisionId}><div><strong>{decision.summary}</strong><small>{dateFor(decision.observedAt)}</small></div><span>{labelFor(decision.status)}</span></div>)}</div></section>}
    {work.handoffs && work.handoffs.length > 0 && <section aria-labelledby="handoffs-title"><h3 id="handoffs-title">최근 handoff</h3><div className="ruled-list">{work.handoffs.map((handoff, index) => <div className="evidence-row" key={`${handoff.summary}-${index}`}><div><strong>{handoff.summary}</strong><small>{handoff.evidenceRefs.join(' · ')}</small></div><span>{labelFor(handoff.nextAction)}</span></div>)}</div></section>}
    <section aria-labelledby="links-title"><h3 id="links-title">관련 링크</h3><div className="link-list">{links.length > 0 ? links.map((link) => <ExternalLink key={`${link.kind}-${link.url}`} url={link.url} label={link.label} />) : <p className="muted">연결된 GitHub 링크가 없습니다.</p>}</div></section>
  </div>
}

function HerdrConnections({ connections }: { connections: HerdrConnection[] }) {
  if (connections.length === 0) return <p className="muted">선택한 업무에 명시된 Herdr 연결이 없습니다.</p>
  return <div className="ruled-list">{connections.map((connection, index) => <div className="evidence-row" key={`${connection.session}-${connection.paneId ?? index}`}><div><strong>{connection.session}</strong><small>{[connection.location?.workspaceId && `workspace ${connection.location.workspaceId}`, connection.location?.tabId && `tab ${connection.location.tabId}`, connection.location?.paneId && `pane ${connection.location.paneId}`, connection.location?.cwd && `cwd ${connection.location.cwd}`].filter(Boolean).join(' · ') || '위치 없음'}{connection.observedAt ? ` · 관찰 ${dateFor(connection.observedAt)}` : ''}</small></div><span>{labelFor(connection.status)}{connection.agentStatus ? ` · ${labelFor(connection.agentStatus)}` : ''}<small>{herdrGuidance(connection)}</small></span></div>)}</div>
}

export function ProjectDetail({ project, selectedWorkId, herdr, projectTodoCount, onSelectWork, onOpenProjectTodos, onStatus }: { project: Project; selectedWorkId: string | null; herdr?: HerdrSnapshot; projectTodoCount: number; onSelectWork: (id: string) => void; onOpenProjectTodos: (projectKey: string) => void; onStatus: (message: string) => void }) {
  const [tab, setTab] = useState<'work' | 'references'>('work')
  useEffect(() => setTab('work'), [project.projectId])
  const work = project.workItems.find((item) => item.workId === selectedWorkId) ?? project.workItems[0]
  const workURL = work?.links?.find((link) => isSafeExternalURL(link.url))
  const identity = work?.github?.number ? `${work.github.kind === 'pull_request' ? 'PR' : work.github.kind === 'issue' ? 'Issue' : 'Project'} #${work.github.number}` : work ? '작업 선택됨' : '작업 없음'
  const copyHandoff = async () => {
    const text = buildHandoffText(work, connectionsForWork(work, project, herdr))
    if (!text) { onStatus('복사할 작업 정보가 없습니다.'); return }
    const wailsClipboard = window.runtime?.ClipboardSetText
    if (wailsClipboard) {
      try { if (await wailsClipboard(text)) onStatus('선택한 작업 정보를 클립보드에 복사했습니다.'); else onStatus('작업 정보를 복사하지 못했습니다. GitHub 링크를 열어 내용을 확인하세요.') } catch { onStatus('작업 정보를 복사하지 못했습니다. GitHub 링크를 열어 내용을 확인하세요.') }
      return
    }
    if (!navigator.clipboard?.writeText) { onStatus('클립보드를 사용할 수 없습니다. 작업 정보를 직접 선택해 복사하세요.'); return }
    try { await navigator.clipboard.writeText(text); onStatus('선택한 작업 정보를 클립보드에 복사했습니다.') } catch { onStatus('작업 정보를 복사하지 못했습니다. GitHub 링크를 열어 내용을 확인하세요.') }
  }
  return <section className="detail" aria-labelledby="detail-title">
    <div className="detail-heading"><div><h2 id="detail-title">{project.name}</h2><p>최근 동기화 {dateFor(project.updatedAt)} · {labelFor(project.syncStatus)}</p><p className="work-identity">{identity}</p></div>{work && <span className="state-badge"><StatusMark value={work.state} />{labelFor(work.state)}</span>}</div>
    {project.notices && project.notices.length > 0 && <div className="project-notices">{project.notices.map((notice, index) => <p key={`${notice}-${index}`}>{notice}</p>)}</div>}
    <div className="detail-actions"><button type="button" className="primary-action" onClick={() => void copyHandoff()}>작업 정보 복사</button>{workURL && <button type="button" className="secondary-action" onClick={() => openExternalURL(workURL.url)}>GitHub에서 보기</button>}{projectTodoCount > 0 && <button type="button" className="todo-shortcut" aria-label="할 일 보기" onClick={() => onOpenProjectTodos(toolboxKeyFor(project))}><strong>할 일 {projectTodoCount}개</strong><span>보기</span></button>}</div>
    <div className="project-detail-tabs" role="tablist" aria-label="프로젝트 상세 보기">
      <button type="button" role="tab" aria-selected={tab === 'work'} onClick={() => setTab('work')}>업무</button>
      <button type="button" role="tab" aria-selected={tab === 'references'} onClick={() => setTab('references')}>자료</button>
    </div>
    {tab === 'references' ? <ProjectReferences projectKey={toolboxKeyFor(project)} projectName={project.name} onStatus={onStatus} /> : work ? <><WorkTable items={project.workItems} selected={work.workId} onSelect={onSelectWork} /><article className="work-summary"><h3>{work.title}</h3>{work.request && <MarkdownBody value={work.request} />}{work.blocker && <p className="blocker"><strong>보존된 변경</strong> {work.blocker}</p>}</article><EvidenceList work={work} project={project} /></> : <section className="empty-detail"><p>아직 표시할 업무가 없습니다.</p></section>}
  </section>
}
