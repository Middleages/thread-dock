import { useEffect, useMemo, useState } from 'react'
import type { HerdrConnection, HerdrSnapshot, Project, WorkItem } from './types'
import { connectionsForWork, dateFor, herdrGuidance, labelFor } from './monitor-presentation'
import './herdr-summary.css'

const severity: Record<string, number> = { blocked: 5, failed: 5, offline: 4, stale: 3, unverified: 2, missing: 2, unknown: 1, fresh: 0, synced: 0, connected: 0 }
const severeState = (value?: string) => value && severity[value] > 0 ? value : undefined
const stateRank = (value?: string) => value ? severity[value] ?? 0 : 0

function locationText(connection: HerdrConnection) {
  return [connection.location?.workspaceId && `workspace ${connection.location.workspaceId}`, connection.location?.tabId && `tab ${connection.location.tabId}`, connection.location?.paneId && `pane ${connection.location.paneId}`, connection.location?.cwd && `cwd ${connection.location.cwd}`].filter(Boolean).join(' · ') || '위치 없음'
}

function connectionState(connection: HerdrConnection) {
  return severeState(connection.status) ?? severeState(connection.agentStatus) ?? (connection.status === 'connected' ? 'connected' : connection.status || 'unknown')
}

export function HerdrSummary({ herdr, project, work }: { herdr: HerdrSnapshot; project?: Project; work?: WorkItem }) {
  const [expanded, setExpanded] = useState(false)
  useEffect(() => setExpanded(false), [project?.projectId, work?.workId])
  const selected = useMemo(() => connectionsForWork(work, project, herdr), [herdr, project, work])
  const summary = useMemo(() => {
    const snapshotStates = [herdr.status, herdr.state, herdr.syncStatus, herdr.freshness.state, herdr.freshness.syncStatus]
    const snapshotState = snapshotStates.filter(Boolean).sort((a, b) => stateRank(b) - stateRank(a))[0] ?? 'unknown'
    const chosen = selected.slice().sort((a, b) => stateRank(connectionState(b)) - stateRank(connectionState(a)))[0]
    const chosenState = chosen ? connectionState(chosen) : snapshotState
    const state = stateRank(chosenState) >= stateRank(snapshotState) ? chosenState : snapshotState
    return { state, chosen }
  }, [herdr, selected])
  const chosen = summary.chosen
  const agentLabel = chosen?.agentStatus ? labelFor(chosen.agentStatus) : ''
  const summaryLabel = chosen ? `${labelFor(summary.state)} · ${chosen.session}${agentLabel ? ` · ${agentLabel}` : ''}` : `${labelFor(summary.state)} · 연결 없음`
  const detailID = 'herdr-summary-detail'

  return <section className={`herdr-summary${expanded ? ' expanded' : ''}`} aria-labelledby="herdr-summary-title">
    <div className="herdr-summary-row">
      <div className="herdr-summary-copy"><strong id="herdr-summary-title">실행 상태</strong><span className={`status-mark ${summary.state === 'blocked' || summary.state === 'failed' || summary.state === 'offline' || summary.state === 'stale' || summary.state === 'unverified' ? 'attention' : 'success'}`} aria-hidden="true" /><span>{summaryLabel}</span><small>관찰 {dateFor(herdr.observedAt)}</small></div>
      <button type="button" aria-expanded={expanded} aria-controls={detailID} onClick={() => setExpanded((value) => !value)}>{expanded ? '접기' : '자세히'}</button>
    </div>
    {expanded && <div id={detailID} className="herdr-summary-detail">
      {herdr.notices.length > 0 && <div className="herdr-summary-notices" role="note">{herdr.notices.map((notice, index) => <p key={`${notice}-${index}`}>{notice}</p>)}</div>}
      {(selected.length > 0 || work) && <section aria-labelledby="herdr-selected-title"><h3 id="herdr-selected-title">선택 업무 위치</h3><div className="ruled-list">{selected.length === 0 ? <p className="muted">선택한 업무에 명시된 Herdr 연결이 없습니다.</p> : selected.map((connection, index) => <div className="evidence-row" key={`${connection.session}-${connection.paneId ?? index}`}><div><strong>{connection.session}</strong><small>{locationText(connection)}{connection.observedAt ? ` · 관찰 ${dateFor(connection.observedAt)}` : ''}</small></div><span>{labelFor(connection.status)}{connection.agentStatus ? ` · ${labelFor(connection.agentStatus)}` : ''}<small>{herdrGuidance(connection)}</small></span></div>)}</div></section>}
      <section aria-labelledby="herdr-sessions-title"><h3 id="herdr-sessions-title">관찰한 세션</h3><div className="ruled-list">{herdr.sessions.length === 0 ? <p className="muted">관찰한 세션이 없습니다.</p> : herdr.sessions.map((session) => <div className="evidence-row" key={session.session}><div><strong>{session.session}</strong><small>{session.observedAt ? `관찰 ${dateFor(session.observedAt)}` : '관찰 시각 없음'}</small></div><span>{labelFor(session.status)} · {session.agents.length}개 Agent</span></div>)}</div></section>
      {herdr.unconnectedAgents.length > 0 && <section aria-labelledby="herdr-unconnected-title"><h3 id="herdr-unconnected-title">연결되지 않은 Agent</h3><div className="ruled-list">{herdr.unconnectedAgents.map((agent, index) => <div className="evidence-row" key={`${agent.session}-${agent.pane_id ?? index}`}><div><strong>{agent.name || '이름 없음'}</strong><small>세션 {agent.session}{agent.pane_id ? ` · pane ${agent.pane_id}` : ''}{agent.cwd ? ` · cwd ${agent.cwd}` : ''}</small></div><span>{labelFor(agent.agent_status || 'unknown')}</span></div>)}</div></section>}
    </div>}
  </section>
}
