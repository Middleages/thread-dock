import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { AgentWorkflow } from './AgentWorkflow'
import type { AgentWorkflow as AgentWorkflowType, WorkItem } from './types'

const baseWork: WorkItem = {
  workId: 'pr:102',
  title: 'Implement native agent results',
  state: 'open',
  syncStatus: 'synced',
  nextAction: 'review',
  evidenceRefs: ['github:checks:ci/build', 'pr:102:comment:1'],
  links: [],
}

const baseWorkflow = (overrides: Partial<AgentWorkflowType> = {}): AgentWorkflowType => ({
  executionState: 'completed',
  githubWorkState: 'open',
  verificationState: {
    status: 'verified',
    codes: ['reviewer_accepted', 'ci_passed'],
    summary: '검증 완료 (리뷰어 승인 및 CI 통과)',
    anchorSha: '8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d',
  },
  candidateRevision: '8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d',
  evidenceRefs: ['github:checks:ci/build', 'pr:102:comment:1'],
  notices: [],
  provenance: {
    source: 'herdr+github',
    observedAt: '2026-11-02T03:00:00Z',
    scope: 'issue',
  },
  agents: [
    {
      agentId: 'ParentLead',
      role: 'coordinator',
      runtime: 'herdr',
      executionRef: 'sess-parent/pane-1',
      execution: 'completed',
      provenance: { source: 'herdr' },
    },
    {
      agentId: 'ChildWorker',
      parentAgentId: 'ParentLead',
      role: 'reviewer',
      runtime: 'herdr',
      executionRef: 'sess-child/pane-2',
      execution: 'completed',
      provenance: { source: 'herdr' },
    },
  ],
  tasks: [
    {
      taskId: 'task-102',
      agentId: 'ChildWorker',
      role: 'reviewer',
      objective: 'Review candidate commit',
    },
  ],
  results: [
    {
      resultId: 'result-102',
      taskId: 'task-102',
      agentId: 'ChildWorker',
      status: 'completed',
      verification: 'verified',
      summary: '리뷰어 승인 검증 완료',
      findings: ['ACCEPT verdict on 8e54c92'],
    },
  ],
  ...overrides,
})

describe('AgentWorkflow', () => {
  afterEach(cleanup)

  it('renders verified state with explicit wording 확인됨, summary badge, and candidate SHA', () => {
    const { container } = render(<AgentWorkflow workflow={baseWorkflow()} work={baseWork} />)

    expect(screen.getByText('Agent 워크플로우')).toBeInTheDocument()
    expect(screen.getByText('확인됨')).toBeInTheDocument()
    expect(screen.getByText('검증 완료 (리뷰어 승인 및 CI 통과)')).toBeInTheDocument()
    expect(screen.getByText(/실행: 실행 완료/)).toBeInTheDocument()
    expect(screen.getByText('SHA 8e54c92')).toBeInTheDocument()
    expect(container.querySelector('.status-mark')).toHaveClass('success')
    expect(screen.getByRole('button', { name: '자세히' })).toHaveAttribute('aria-expanded', 'false')
  })

  it('renders partial verification state with explicit wording 부분 확인', () => {
    const { container } = render(
      <AgentWorkflow
        workflow={baseWorkflow({
          executionState: 'running',
          verificationState: {
            status: 'partial',
            codes: ['child_incomplete'],
            summary: '하위 에이전트 작업 미완료로 부분 확인',
            anchorSha: '8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d',
          },
        })}
        work={baseWork}
      />
    )

    expect(screen.getByText('부분 확인')).toBeInTheDocument()
    expect(screen.getByText('하위 에이전트 작업 미완료로 부분 확인')).toBeInTheDocument()
    expect(screen.getByText(/실행: 실행 중/)).toBeInTheDocument()
    expect(container.querySelector('.status-mark')).toHaveClass('attention')
  })

  it('renders unverified state with explicit wording 미확인 and displays awaiting_pr helper notice on expand', () => {
    const { container } = render(
      <AgentWorkflow
        workflow={baseWorkflow({
          candidateRevision: '',
          verificationState: {
            status: 'unverified',
            codes: ['awaiting_pr'],
            summary: 'PR 생성 대기 중 (로컬 작업 중)',
            anchorSha: '',
          },
        })}
        work={baseWork}
      />
    )

    expect(screen.getByText('미확인')).toBeInTheDocument()
    expect(screen.getByText('PR 생성 대기 중 (로컬 작업 중)')).toBeInTheDocument()
    expect(container.querySelector('.status-mark')).toHaveClass('attention')

    // Before expand: helper notice not visible
    expect(screen.queryByText(/PR 생성 대기:/)).not.toBeInTheDocument()

    // Expand
    fireEvent.click(screen.getByRole('button', { name: '자세히' }))
    expect(screen.getByText(/PR 생성 대기/)).toBeInTheDocument()
    expect(screen.getByText(/로컬 워크트리에서 구현 중이며/)).toBeInTheDocument()
  })

  it('shows agent lineage tree with parent-child lineage when expanded', () => {
    render(<AgentWorkflow workflow={baseWorkflow()} work={baseWork} />)

    // Expand details
    fireEvent.click(screen.getByRole('button', { name: '자세히' }))

    expect(screen.getByText('에이전트 계통 (Lineage)')).toBeInTheDocument()
    expect(screen.getByText('ParentLead')).toBeInTheDocument()
    expect(screen.getByText('ChildWorker')).toBeInTheDocument()
    expect(screen.getByText('(상위: ParentLead)')).toBeInTheDocument()
    expect(screen.getByText('↳')).toBeInTheDocument()
  })

  it('shows candidate SHA and evidence references in details view', () => {
    render(<AgentWorkflow workflow={baseWorkflow()} work={baseWork} />)

    fireEvent.click(screen.getByRole('button', { name: '자세히' }))

    expect(screen.getByText('수집된 검증 증거')).toBeInTheDocument()
    expect(screen.getByText('github:checks:ci/build')).toBeInTheDocument()
    expect(screen.getByText('pr:102:comment:1')).toBeInTheDocument()
    expect(screen.getByText('8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d')).toBeInTheDocument()
  })

  it('displays fallback and partial notices from provenance', () => {
    render(
      <AgentWorkflow
        workflow={baseWorkflow({
          provenance: {
            source: 'herdr',
            fallback: true,
            partial: true,
            stale: true,
          },
          notices: ['세션 바인딩 모호성 경고'],
        })}
        work={baseWork}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: '자세히' }))

    expect(screen.getByText('저장소 코디네이터 세션 폴백으로 관찰되었습니다.')).toBeInTheDocument()
    expect(screen.getByText('관찰 정보가 최신 상태가 아닐 수 있습니다.')).toBeInTheDocument()
    expect(screen.getByText('세션 바인딩 모호성 경고')).toBeInTheDocument()
  })

  it('renders clean empty state for legacy work item without agent workflow', () => {
    render(<AgentWorkflow workflow={undefined} work={baseWork} />)

    expect(screen.getByText('Agent 워크플로우')).toBeInTheDocument()
    expect(screen.getByText('관찰된 Agent 워크플로우가 없습니다.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '자세히' })).not.toBeInTheDocument()
  })

  it('distinguishes task result verification label from workflow verification', () => {
    render(
      <AgentWorkflow
        workflow={baseWorkflow({
          verificationState: {
            status: 'unverified',
            summary: '검증 대기 중',
          },
        })}
        work={baseWork}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: '자세히' }))

    // Top-level workflow says 미확인
    expect(screen.getByText('미확인')).toBeInTheDocument()
    // Result explicitly says '결과 검증: 확인됨'
    expect(screen.getByText(/결과 검증: 확인됨/)).toBeInTheDocument()
  })

  it('contains no mutation or action controls (read-only)', () => {
    render(<AgentWorkflow workflow={baseWorkflow()} work={baseWork} />)

    fireEvent.click(screen.getByRole('button', { name: '자세히' }))

    // Only the expand/collapse button should exist
    const buttons = screen.getAllByRole('button')
    expect(buttons).toHaveLength(1)
    expect(buttons[0]).toHaveTextContent('접기')

    // Must not have create/retry/approve/reassign controls
    expect(screen.queryByText(/재시도/)).not.toBeInTheDocument()
    expect(screen.queryByText(/승인하기/)).not.toBeInTheDocument()
    expect(screen.queryByText(/중지/)).not.toBeInTheDocument()
  })
})
