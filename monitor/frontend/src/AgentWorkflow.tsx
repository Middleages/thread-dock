import { useEffect, useState } from 'react'
import type { AgentWorkflow as AgentWorkflowType, WorkItem } from './types'
import { dateFor, executionStateLabel, labelFor, verificationStatusLabel } from './monitor-presentation'
import './agent-workflow.css'

export function AgentWorkflow({
  workflow,
  work,
}: {
  workflow?: AgentWorkflowType
  work?: WorkItem
}) {
  const [expanded, setExpanded] = useState(false)
  useEffect(() => setExpanded(false), [work?.workId])

  if (!workflow || !workflow.agents || workflow.agents.length === 0) {
    return (
      <section className="agent-workflow empty-state" aria-labelledby="agent-workflow-title">
        <div className="agent-workflow-row">
          <div className="agent-workflow-copy">
            <strong id="agent-workflow-title">Agent 워크플로우</strong>
            <span className="agent-workflow-summary-text">관찰된 Agent 워크플로우가 없습니다.</span>
          </div>
        </div>
      </section>
    )
  }

  const vState = workflow.verificationState
  const vStatus = vState?.status ?? 'unverified'
  const vLabel = verificationStatusLabel(vStatus)
  const isVerified = vStatus === 'verified'
  const isPartial = vStatus === 'partial'
  const detailID = `agent-workflow-detail-${work?.workId ?? 'main'}`
  const candidateSHA = workflow.candidateRevision || vState?.anchorSha
  const codes = vState?.codes ?? []
  const hasAwaitingPR = codes.includes('awaiting_pr')

  return (
    <section className={`agent-workflow${expanded ? ' expanded' : ''}`} aria-labelledby="agent-workflow-title">
      <div className="agent-workflow-row">
        <div className="agent-workflow-copy">
          <strong id="agent-workflow-title">Agent 워크플로우</strong>
          <span
            className={`status-mark ${isVerified ? 'success' : 'attention'}`}
            aria-hidden="true"
          />
          <span className={`agent-workflow-badge ${vStatus}`}>
            {vLabel}
          </span>
          {vState?.summary && (
            <span className="agent-workflow-summary-text">
              {vState.summary}
            </span>
          )}
          <span>
            실행: {executionStateLabel(workflow.executionState)}
          </span>
          {candidateSHA && (
            <span className="agent-evidence-tag">
              SHA {candidateSHA.slice(0, 7)}
            </span>
          )}
          {workflow.provenance?.observedAt && (
            <small>관찰 {dateFor(workflow.provenance.observedAt)}</small>
          )}
        </div>
        <button
          type="button"
          aria-expanded={expanded}
          aria-controls={detailID}
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? '접기' : '자세히'}
        </button>
      </div>

      {expanded && (
        <div id={detailID} className="agent-workflow-detail">
          {hasAwaitingPR && (
            <div className="agent-workflow-helper-notice" role="status">
              <strong>PR 생성 대기</strong>: 로컬 워크트리에서 구현 중이며, PR이 생성되면 검증이 시작됩니다.
            </div>
          )}

          {workflow.provenance?.fallback && (
            <div className="agent-workflow-notices" role="note">
              <p>저장소 코디네이터 세션 폴백으로 관찰되었습니다.</p>
            </div>
          )}

          {workflow.provenance?.partial && !workflow.provenance?.fallback && (
            <div className="agent-workflow-notices" role="note">
              <p>복수 세션 바인딩 등으로 인한 부분 관찰 상태입니다.</p>
            </div>
          )}

          {workflow.provenance?.stale && (
            <div className="agent-workflow-notices" role="note">
              <p>관찰 정보가 최신 상태가 아닐 수 있습니다.</p>
            </div>
          )}

          {workflow.notices && workflow.notices.length > 0 && (
            <div className="agent-workflow-notices" role="note">
              {workflow.notices.map((notice, idx) => (
                <p key={`${notice}-${idx}`}>{notice}</p>
              ))}
            </div>
          )}

          <section aria-labelledby="workflow-overview-heading">
            <h3 id="workflow-overview-heading">상태 요약</h3>
            <div className="evidence-row">
              <div>
                <strong>실행 상태</strong>
                <small>Herdr Agent 관찰 결과</small>
              </div>
              <span>{executionStateLabel(workflow.executionState)}</span>
            </div>
            {workflow.githubWorkState && (
              <div className="evidence-row">
                <div>
                  <strong>GitHub 작업 상태</strong>
                  <small>Issue / PR 상태</small>
                </div>
                <span>{labelFor(workflow.githubWorkState)}</span>
              </div>
            )}
            <div className="evidence-row">
              <div>
                <strong>검증 상태</strong>
                <small>{vState?.summary || '요약 없음'}</small>
                {codes.length > 0 && (
                  <div className="agent-codes-list">
                    {codes.map((code) => (
                      <span key={code} className="agent-code-tag">
                        {code}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              <span className={`agent-workflow-badge ${vStatus}`}>{vLabel}</span>
            </div>
            {candidateSHA && (
              <div className="evidence-row">
                <div>
                  <strong>후보 커밋 (Candidate Revision)</strong>
                  <small>검증 기준 커밋 해시</small>
                </div>
                <span className="agent-evidence-tag">{candidateSHA}</span>
              </div>
            )}
          </section>

          <section aria-labelledby="workflow-lineage-heading">
            <h3 id="workflow-lineage-heading">에이전트 계통 (Lineage)</h3>
            <div className="agent-lineage-list ruled-list">
              {workflow.agents.map((agent) => {
                const isChild = Boolean(agent.parentAgentId)
                return (
                  <div
                    key={agent.agentId}
                    className={`agent-lineage-item${isChild ? ' child' : ''}`}
                  >
                    <div className="agent-lineage-identity">
                      {isChild && (
                        <span className="child-indicator" aria-label="하위 에이전트">
                          ↳
                        </span>
                      )}
                      <strong>{agent.agentId}</strong>
                      {agent.role && (
                        <span className="agent-code-tag">{agent.role}</span>
                      )}
                      {agent.parentAgentId && (
                        <small className="agent-lineage-meta">
                          (상위: {agent.parentAgentId})
                        </small>
                      )}
                      {agent.executionRef && (
                        <small className="agent-lineage-meta">
                          {agent.executionRef}
                        </small>
                      )}
                    </div>
                    <div className="agent-lineage-status">
                      <span
                        className={`status-mark ${
                          agent.execution === 'completed'
                            ? 'success'
                            : 'attention'
                        }`}
                        aria-hidden="true"
                      />
                      <span>{executionStateLabel(agent.execution)}</span>
                    </div>
                  </div>
                )
              })}
            </div>
          </section>

          {((workflow.tasks && workflow.tasks.length > 0) ||
            (workflow.results && workflow.results.length > 0)) && (
            <section aria-labelledby="workflow-results-heading">
              <h3 id="workflow-results-heading">작업 실행 및 결과</h3>
              <div className="ruled-list">
                {workflow.tasks.map((task) => (
                  <div key={task.taskId} className="evidence-row">
                    <div>
                      <strong>{task.objective || task.taskId}</strong>
                      <small>
                        Task ID: {task.taskId}
                        {task.agentId ? ` · Agent: ${task.agentId}` : ''}
                        {task.role ? ` · Role: ${task.role}` : ''}
                      </small>
                    </div>
                    <span>{task.role || 'task'}</span>
                  </div>
                ))}
                {workflow.results.map((result) => (
                  <div key={result.resultId} className="evidence-row">
                    <div>
                      <strong>{result.summary || result.resultId}</strong>
                      <small>
                        Result ID: {result.resultId}
                        {result.findings && result.findings.length > 0
                          ? ` · ${result.findings.join(', ')}`
                          : ''}
                      </small>
                    </div>
                    <span>
                      {executionStateLabel(result.status)} ·{' '}
                      {verificationStatusLabel(result.verification)}
                    </span>
                  </div>
                ))}
              </div>
            </section>
          )}

          {workflow.evidenceRefs && workflow.evidenceRefs.length > 0 && (
            <section aria-labelledby="workflow-evidence-heading">
              <h3 id="workflow-evidence-heading">수집된 검증 증거</h3>
              <div className="agent-evidence-tags">
                {workflow.evidenceRefs.map((ref, idx) => (
                  <span key={`${ref}-${idx}`} className="agent-evidence-tag">
                    {ref}
                  </span>
                ))}
              </div>
            </section>
          )}

          {workflow.provenance && (
            <div className="agent-lineage-meta" style={{ marginTop: '8px' }}>
              <small>
                출처: {workflow.provenance.source || 'herdr+github'}
                {workflow.provenance.scope ? ` · 범위: ${workflow.provenance.scope}` : ''}
                {workflow.provenance.capturedAt
                  ? ` · 수집: ${dateFor(workflow.provenance.capturedAt)}`
                  : ''}
              </small>
            </div>
          )}
        </div>
      )}
    </section>
  )
}
