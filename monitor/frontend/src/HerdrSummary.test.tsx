import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { HerdrSummary } from './HerdrSummary'
import type { HerdrSnapshot, Project, WorkItem } from './types'

const work: WorkItem = { workId: 'issue:1', title: 'Issue work', state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], github: { kind: 'issue', url: 'https://github.com/acme/app/issues/1', state: 'OPEN' }, links: [] }
const project: Project = { projectId: 'repo:acme/app', name: 'acme/app', state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], workItems: [work], links: [] }
const baseHerdr = (overrides: Partial<HerdrSnapshot> = {}): HerdrSnapshot => ({
  source: 'herdr', schemaVersion: 1, revision: 1, observedAt: '2026-09-11T01:00:00Z', status: 'fresh', syncStatus: 'synced', freshness: { state: 'fresh', syncStatus: 'synced' }, notices: [],
  sessions: [{ session: 'feature-123', status: 'fresh', observedAt: '2026-09-11T01:00:00Z', agents: [{ name: 'Luna', agent_status: 'working', workspace_id: 'workspace-1', tab_id: 'tab-1', pane_id: 'pane-1', cwd: '/repo/src' }] }],
  connections: [{ issueUrl: 'https://github.com/acme/app/issues/1', repository: 'github.com/acme/app', session: 'feature-123', workspaceId: 'workspace-1', tabId: 'tab-1', paneId: 'pane-1', agentName: 'Luna', status: 'connected', agentStatus: 'working', observedAt: '2026-09-11T01:00:00Z', nextAction: 'observe', handoff: 'review', location: { session: 'feature-123', workspaceId: 'workspace-1', tabId: 'tab-1', paneId: 'pane-1', cwd: '/repo/src' } }],
  unconnectedAgents: [], ...overrides,
})

describe('HerdrSummary', () => {
  afterEach(cleanup)

  it('starts collapsed with one execution summary and exposes the selected working session', () => {
    render(<HerdrSummary herdr={baseHerdr()} project={project} work={work} />)
    expect(screen.getByText('실행 상태')).toBeInTheDocument()
    expect(screen.getByText(/feature-123/)).toBeInTheDocument()
    expect(screen.getByText(/작업 중/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '자세히' })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('/repo/src')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '자세히' }))
    expect(screen.getByRole('button', { name: '접기' })).toHaveAttribute('aria-expanded', 'true')
    expect(document.body).toHaveTextContent('/repo/src')
    fireEvent.click(screen.getByRole('button', { name: '접기' }))
    expect(screen.queryByText('/repo/src')).not.toBeInTheDocument()
  })

  it.each([
    ['blocked', '판단 필요'], ['offline', '오프라인'], ['stale', '오래된 상태'], ['unverified', '확인 필요'], ['missing', '대상 없음'],
  ])('keeps %s visible while collapsed', (state, expected) => {
    const { container } = render(<HerdrSummary herdr={baseHerdr({ status: state, freshness: { state, syncStatus: state } })} project={project} work={work} />)
    expect(screen.getByText(new RegExp(expected))).toBeInTheDocument()
    expect(container.querySelector('.status-mark')).toHaveClass('attention')
    expect(screen.getByRole('button', { name: '자세히' })).toHaveAttribute('aria-expanded', 'false')
  })

  it('shows the most severe selected connection instead of hiding later blocked state', () => {
    const connections = [
      { ...baseHerdr().connections[0], session: 'offline-session', status: 'offline', agentStatus: 'idle', paneId: 'pane-2' },
      { ...baseHerdr().connections[0], session: 'blocked-session', status: 'stale', agentStatus: 'blocked', paneId: 'pane-3' },
    ]
    render(<HerdrSummary herdr={baseHerdr({ connections })} project={project} work={work} />)
    expect(screen.getByText(/판단 필요 · blocked-session/)).toBeInTheDocument()
    expect(screen.queryByText('workspace-1')).not.toBeInTheDocument()
  })

  it('reveals notices, observed sessions, and unconnected agents only on expand', () => {
    const herdr = baseHerdr({ notices: ['Herdr 연결을 확인하세요'], unconnectedAgents: [{ session: 'feature-123', name: 'Reviewer', agent_status: 'idle', pane_id: 'pane-9' }] })
    render(<HerdrSummary herdr={herdr} project={project} work={work} />)
    expect(screen.queryByText('Herdr 연결을 확인하세요')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '자세히' }))
    expect(screen.getByText('Herdr 연결을 확인하세요')).toBeInTheDocument()
    expect(screen.getByText('관찰한 세션')).toBeInTheDocument()
    expect(screen.getByText('연결되지 않은 Agent')).toBeInTheDocument()
    expect(screen.getByText('Reviewer')).toBeInTheDocument()
  })

  it('keeps local Herdr status visible without a GitHub project or matching work', () => {
    render(<HerdrSummary herdr={baseHerdr({ status: 'offline', connections: [] })} />)
    expect(screen.getByText('실행 상태')).toBeInTheDocument()
    expect(document.body).toHaveTextContent('오프라인 · 연결 없음')
    expect(screen.getByRole('button', { name: '자세히' })).toBeInTheDocument()
  })
})
