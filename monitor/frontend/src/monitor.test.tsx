import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App, sortProjects } from './App'
import type { Project, Snapshot } from './types'

const project = (overrides: Partial<Project> = {}): Project => ({
  projectId: 'project-1', name: 'Payments', state: 'running', syncStatus: 'synced', nextAction: 'review',
  evidenceRefs: ['state://project-1'], updatedAt: '2026-09-07T01:00:00Z', workItems: [{
    workId: 'work-1', title: 'Retry payment failures', request: 'Reduce duplicate retries', state: 'needs_operator',
    syncStatus: 'synced', nextAction: 'review', evidenceRefs: ['state://work-1'], updatedAt: '2026-09-07T01:00:00Z',
    tasks: [{ taskId: 'task-1', repoKey: 'app', state: 'verified', verification: 'passed', review: 'approved', merge: 'ready' }],
    publications: [{ intentId: 'intent-1', key: 'parent-issue', generation: 1, kind: 'issue', status: 'published', attempts: 1, url: 'https://github.com/acme/app/issues/1' }],
    decisions: [{ decisionId: 'decision-1', summary: 'Use bounded retries', status: 'accepted' }],
    handoffs: [{ summary: 'Review the change', nextAction: 'approve', evidenceRefs: ['handoff://1'] }],
    links: [{ kind: 'github', label: 'Parent Issue', url: 'https://github.com/acme/app/issues/1' }],
  }], ...overrides,
})

const snapshot = (projects: Project[], overrides: Partial<Snapshot> = {}): Snapshot => ({
  schemaVersion: 2, revision: 3, observedAt: '2026-09-07T01:00:00Z',
  freshness: { state: 'fresh', syncStatus: 'synced' }, state: 'running', syncStatus: 'synced', nextAction: 'review',
  evidenceRefs: [], projects, ...overrides,
})

describe('monitor list and detail', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => { cleanup(); vi.useRealTimers() })

  it('sorts a copied project list with needs-operator first without mutating the snapshot', () => {
    const source = [project({ projectId: 'ordinary', name: 'Ordinary', state: 'running' }), project({ projectId: 'attention', name: 'Attention', state: 'needs_operator', updatedAt: '2026-09-07T00:00:00Z' })]
    const original = source.slice()
    expect(sortProjects(source).map((item) => item.projectId)).toEqual(['attention', 'ordinary'])
    expect(source).toEqual(original)
  })

  it('keeps the selected project across refresh and falls back to the first visible project when removed', async () => {
    const first = project({ projectId: 'first', name: 'First' })
    const second = project({ projectId: 'second', name: 'Second' })
    let current = snapshot([first, second])
    const source = vi.fn(async () => current)
    render(<App snapshotSource={source} pollIntervalMs={4000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: /Second/ }))
    expect(screen.getByRole('heading', { name: 'Second' })).toBeInTheDocument()
    current = snapshot([first, second], { revision: 4 })
    await act(async () => { await vi.advanceTimersByTimeAsync(4000) })
    expect(screen.getByRole('heading', { name: 'Second' })).toBeInTheDocument()
    current = snapshot([first], { revision: 5 })
    await act(async () => { await vi.advanceTimersByTimeAsync(4000) })
    expect(screen.getByRole('heading', { name: 'First' })).toBeInTheDocument()
  })

  it('never overlaps the four-second aggregate polls', async () => {
    let resolve!: (value: Snapshot) => void
    const source = vi.fn(() => new Promise<Snapshot>((done) => { resolve = done }))
    render(<App snapshotSource={source} pollIntervalMs={4000} />)
    expect(source).toHaveBeenCalledTimes(1)
    await act(async () => { await vi.advanceTimersByTimeAsync(12000) })
    expect(source).toHaveBeenCalledTimes(1)
    await act(async () => { resolve(snapshot([project()])); await Promise.resolve() })
    await act(async () => { await vi.advanceTimersByTimeAsync(4000) })
    expect(source).toHaveBeenCalledTimes(2)
  })

  it('renders loading, empty, error, stale and offline states with recovery copy', async () => {
    let resolve!: (value: Snapshot) => void
    const source = vi.fn(() => new Promise<Snapshot>((done) => { resolve = done }))
    render(<App snapshotSource={source} />)
    expect(screen.getByText('프로젝트 현황을 불러오는 중입니다.')).toBeInTheDocument()
    await act(async () => { resolve(snapshot([])); await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByText('표시할 프로젝트가 없습니다.')).toBeInTheDocument()
    source.mockRejectedValueOnce(new Error('WSL offline'))
    await act(async () => { await vi.advanceTimersByTimeAsync(4000) })
    expect(screen.getByText('상태를 불러오지 못했습니다.')).toBeInTheDocument()
    expect(screen.getByText(/agentctl 연결을 확인/)).toBeInTheDocument()
    const stale = snapshot([project()], { freshness: { state: 'stale', syncStatus: 'offline' }, syncStatus: 'offline', state: 'stale' })
    source.mockResolvedValueOnce(stale)
    await act(async () => { await vi.advanceTimersByTimeAsync(4000) })
    expect(screen.getByText('오래된 상태를 표시하고 있습니다.')).toBeInTheDocument()
    expect(screen.getByText('WSL 연결 오프라인')).toBeInTheDocument()
  })

  it('shows selected work detail, evidence, links and action rail', async () => {
    const source = vi.fn(async () => snapshot([project()]))
    render(<App snapshotSource={source} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByText('Retry payment failures')).toBeInTheDocument()
    expect(screen.getByText('Use bounded retries')).toBeInTheDocument()
    expect(screen.getByText('Review the change')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Parent Issue' })).toHaveAttribute('href', 'https://github.com/acme/app/issues/1')
    fireEvent.click(screen.getByRole('button', { name: '실행 중단' }))
    expect(screen.getByRole('status')).toHaveTextContent('실행 중단 요청을 Main Agent에 전달할 준비가 되었습니다.')
  })

  it('uses native keyboard buttons and omits forbidden runtime identifiers', async () => {
    const source = vi.fn(async () => snapshot([project()]))
    render(<App snapshotSource={source} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    const nav = screen.getByRole('navigation', { name: '주요 메뉴' })
    expect(within(nav).getByRole('button', { name: '작업 현황' })).toBeEnabled()
    expect(screen.queryByText(/providerSession|processId|sessionId|requestId|rawTranscript/i)).not.toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(/providerSession|processId|sessionId|requestId|rawTranscript/i)
  })
})
