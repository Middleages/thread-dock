import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App, sortProjects } from './App'
import type { Project, Snapshot } from './types'
import { isSafeExternalURL } from './safe-url'

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
  afterEach(() => { cleanup(); vi.useRealTimers(); Reflect.deleteProperty(window, 'runtime'); Reflect.deleteProperty(navigator, 'clipboard') })

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
    expect(screen.getByText(/Windows Wails Monitor 설정과 연결을 확인/)).toBeInTheDocument()
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
    expect(screen.queryByRole('heading', { name: '저장소별 작업' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '발행 상태' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '자동화 작업' })).not.toBeInTheDocument()
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async () => undefined } })
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(screen.getByRole('status')).toHaveTextContent('선택한 작업 정보를 클립보드에 복사했습니다.')
  })

  it('renders GitHub issue and project metadata while omitting legacy work sections', async () => {
    const githubWork = { ...project().workItems[0], tasks: [], publications: [], github: { kind: 'pull_request', number: 9, url: 'https://github.com/acme/app/pull/9', state: 'OPEN', reviewDecision: 'REVIEW_REQUIRED', checks: [{ name: 'build', conclusion: 'SUCCESS' }], fields: { status: 'In progress', priority: 'P1' } } }
    const githubProject = project({ source: 'github', links: [{ kind: 'github', label: 'Issues', url: 'https://github.com/acme/app/issues' }, { kind: 'github', label: 'PRs', url: 'https://github.com/acme/app/pulls' }, { kind: 'github', label: 'Wiki', url: 'https://github.com/acme/app/wiki' }], workItems: [githubWork] })
    const source = vi.fn(async () => snapshot([githubProject], { source: 'github', notices: ['GitHub 일부 조회 안내'] }))
    render(<App snapshotSource={source} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('heading', { name: 'GitHub 상태' })).toBeInTheDocument()
    expect(screen.getByText('리뷰 필요')).toBeInTheDocument()
    expect(screen.getByText('성공')).toBeInTheDocument()
    expect(screen.getByText('Project 필드')).toBeInTheDocument()
    expect(screen.getByText('In progress')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '저장소별 작업' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '발행 상태' })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Wiki' })).toHaveAttribute('href', 'https://github.com/acme/app/wiki')
  })

  it('shows the selected work Herdr connection with location guidance and includes it in copied work info', async () => {
    const herdr = { source: 'herdr', schemaVersion: 1, revision: 1, observedAt: '2026-09-10T01:00:00Z', status: 'fresh', syncStatus: 'synced', freshness: { state: 'fresh', syncStatus: 'synced' }, notices: [], sessions: [{ session: 'feature-a', status: 'fresh', observedAt: '2026-09-10T01:00:00Z', agents: [{ name: 'Luna', agent_status: 'working', workspace_id: 'w1', tab_id: 't1', pane_id: 'p1', cwd: '/repo/src' }] }], connections: [{ issueUrl: 'https://github.com/acme/app/issues/1', repository: 'github.com/acme/app', session: 'feature-a', workspaceId: 'w1', tabId: 't1', paneId: 'p1', agentName: 'Luna', status: 'connected', agentStatus: 'working', observedAt: '2026-09-10T01:00:00Z', nextAction: 'observe', handoff: '세션 feature-a · workspace w1 · pane p1 · cwd /repo/src · 관찰 상태 working · 다음 행동: observe', location: { session: 'feature-a', workspaceId: 'w1', tabId: 't1', paneId: 'p1', agentName: 'Luna', cwd: '/repo/src' } }], unconnectedAgents: [{ session: 'feature-a', name: 'Reviewer', agent_status: 'idle', pane_id: 'p2' }] }
    const writeText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    render(<App snapshotSource={vi.fn(async () => snapshot([project()], { herdr }))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('heading', { name: 'Herdr 연결', level: 2 })).toBeInTheDocument()
    expect(screen.getAllByText('feature-a').length).toBeGreaterThan(0)
    expect((document.body.textContent?.match(/\/repo\/src/g) ?? [])).toHaveLength(1)
    expect(screen.getByText('연결되지 않은 Agent')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('feature-a'))
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('다음 행동: observe'))
  })

  it('keeps a local Herdr panel visible when GitHub has no projects', async () => {
    const herdr = { source: 'herdr', schemaVersion: 1, revision: 1, observedAt: '2026-09-10T01:00:00Z', status: 'fresh', syncStatus: 'synced', freshness: { state: 'fresh', syncStatus: 'synced' }, notices: [], sessions: [{ session: 'coordinator', status: 'fresh', agents: [{ name: 'Sol', agent_status: 'idle', pane_id: 'p1' }] }], connections: [], unconnectedAgents: [{ session: 'coordinator', name: 'Sol', agent_status: 'idle', pane_id: 'p1' }] }
    render(<App snapshotSource={vi.fn(async () => snapshot([], { herdr, source: 'github', syncStatus: 'offline' }))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('heading', { name: 'Herdr 연결' })).toBeInTheDocument()
    expect(screen.getByText('coordinator')).toBeInTheDocument()
    expect(screen.getByText('Sol')).toBeInTheDocument()
  })

  it('scopes coordinator and issue connections to the selected primary project and issue', async () => {
    const issueA = 'https://github.com/acme/app/issues/1'
    const issueB = 'https://github.com/acme/app/issues/2'
    const makeHerdrConnection = (session: string, issueUrl?: string) => ({ issueUrl, repository: 'github.com/acme/app', session, role: issueUrl ? 'feature' : 'coordinator', workspaceId: `workspace-${session}`, tabId: `tab-${session}`, paneId: `pane-${session}`, agentName: session, status: 'connected', agentStatus: 'idle', nextAction: 'check_github', handoff: `세션 ${session}` })
    const workA = { ...project().workItems[0], github: { kind: 'issue', url: issueA, state: 'OPEN' }, links: [{ kind: 'issue', label: 'Issue A', url: issueA }, { kind: 'evidence', label: 'Mentioned B', url: issueB }] }
    const workB = { ...project().workItems[0], workId: 'work-b', title: 'Issue B', github: { kind: 'issue', url: issueB, state: 'OPEN' }, links: [{ kind: 'issue', label: 'Issue B', url: issueB }] }
    const projectA = project({ projectId: 'a', name: 'App A', source: 'github', links: [{ kind: 'github', label: 'Issues', url: 'https://github.com/acme/app/issues' }], workItems: [workA] })
    const projectB = project({ projectId: 'b', name: 'App B', source: 'github', links: [{ kind: 'github', label: 'Issues', url: 'https://github.com/acme/app/issues' }], workItems: [workB] })
    const herdr = { source: 'herdr', schemaVersion: 1, revision: 1, observedAt: '2026-09-10T01:00:00Z', status: 'fresh', syncStatus: 'synced', freshness: { state: 'fresh', syncStatus: 'synced' }, notices: [], sessions: [], connections: [makeHerdrConnection('central'), makeHerdrConnection('issue-a', issueA), makeHerdrConnection('issue-b', issueB)], unconnectedAgents: [] }
    const writeText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    render(<App snapshotSource={vi.fn(async () => snapshot([projectA, projectB], { herdr }))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.queryByText('central')).not.toBeInTheDocument()
    expect(screen.getAllByText('issue-a').length).toBeGreaterThan(0)
    expect(screen.queryByText('issue-b')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('세션 issue-a'))
    fireEvent.click(screen.getByRole('button', { name: /App B/ }))
    expect(screen.queryByText('central')).not.toBeInTheDocument()
    expect(screen.getAllByText('issue-b').length).toBeGreaterThan(0)
    expect(screen.queryByText('issue-a')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(writeText).toHaveBeenLastCalledWith(expect.stringContaining('세션 issue-b'))
  })

  it('uses exclusive issue then project then repository binding precedence', async () => {
    const issue = 'https://github.com/acme/app/issues/1'
    const board = 'https://github.com/orgs/acme/projects/7'
    const work = { ...project().workItems[0], github: { kind: 'issue', url: issue, state: 'OPEN' }, links: [{ kind: 'github', label: 'Issue', url: issue }] }
    const selected = project({ links: [{ kind: 'github', label: 'Project', url: board }], workItems: [work] })
    const connection = (session: string, extra: Record<string, string>) => ({ session, repository: 'github.com/acme/app', role: extra.projectUrl || extra.issueUrl ? 'feature' : 'coordinator', status: 'connected', nextAction: 'check_github', handoff: `세션 ${session}`, ...extra })
    const herdr = { source: 'herdr', schemaVersion: 1, revision: 1, observedAt: '2026-09-10T01:00:00Z', status: 'fresh', syncStatus: 'synced', freshness: { state: 'fresh', syncStatus: 'synced' }, notices: [], sessions: [], connections: [connection('issue', { issueUrl: issue }), connection('project', { projectUrl: board }), connection('central', {})], unconnectedAgents: [] }
    const writeText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    render(<App snapshotSource={vi.fn(async () => snapshot([selected], { herdr }))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getAllByText('issue').length).toBeGreaterThan(0)
    expect(screen.queryByText('project')).not.toBeInTheDocument()
    expect(screen.queryByText('central')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('세션 issue'))
  })

  it('does not treat an evidence-only project link as a Project binding', async () => {
    const issue = 'https://github.com/acme/app/issues/1'
    const evidenceProject = 'https://github.com/orgs/acme/projects/99'
    const work = { ...project().workItems[0], github: { kind: 'issue', url: issue, state: 'OPEN' }, links: [{ kind: 'issue', label: 'Issue', url: issue }] }
    const selected = project({ links: [{ kind: 'evidence', label: 'Mentioned Project', url: evidenceProject }], workItems: [work] })
    const makeConnection = (session: string, extra: Record<string, string>) => ({ session, repository: 'github.com/acme/app', role: extra.projectUrl ? 'feature' : 'coordinator', status: 'connected', nextAction: 'check_github', handoff: `세션 ${session}`, ...extra })
    const herdr = { source: 'herdr', schemaVersion: 1, revision: 1, observedAt: '2026-09-10T01:00:00Z', status: 'fresh', syncStatus: 'synced', freshness: { state: 'fresh', syncStatus: 'synced' }, notices: [], sessions: [], connections: [makeConnection('evidence-project', { projectUrl: evidenceProject }), makeConnection('central', {})], unconnectedAgents: [] }
    render(<App snapshotSource={vi.fn(async () => snapshot([selected], { herdr }))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.queryByText('evidence-project')).not.toBeInTheDocument()
    expect(screen.getAllByText('central').length).toBeGreaterThan(0)
  })

  it('removes legacy task and publication presentation while retaining decisions and handoff records', async () => {
    const raw = project({
      workItems: [{ ...project().workItems[0], nextAction: 'approve', tasks: [{ ...project().workItems[0].tasks![0], state: 'verify', verification: 'verify', review: 'accepted' }, { ...project().workItems[0].tasks![0], taskId: 'task-2', state: 'verified', verification: 'verify', review: 'accepted' }], publications: [{ ...project().workItems[0].publications![0], status: 'published' }], decisions: [{ ...project().workItems[0].decisions![0], status: 'accepted' }], handoffs: [{ ...project().workItems[0].handoffs![0], nextAction: 'approve' }] }],
    })
    const source = vi.fn(async () => snapshot([raw]))
    render(<App snapshotSource={source} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.queryByRole('heading', { name: '저장소별 작업' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '발행 상태' })).not.toBeInTheDocument()
    expect(screen.getAllByText('승인됨', { exact: true })).toHaveLength(1)
    expect(screen.getAllByText('승인', { exact: true })).toHaveLength(1)
    expect(document.body.textContent).not.toMatch(/\bverify\b|\bpublished\b|\baccepted\b/)
  })

  it('uses the selected work identity for detail and action links after a project shrinks', async () => {
    const first = project({ projectId: 'first', name: 'First', workItems: [project().workItems[0], { ...project().workItems[0], workId: 'work-2', title: 'Second work', links: [{ kind: 'github', label: 'Second Issue', url: 'https://github.com/acme/app/issues/2' }] }] })
    const second = project({ projectId: 'second', name: 'Second', workItems: [{ ...project().workItems[0], workId: 'work-3', title: 'Only work', links: [{ kind: 'github', label: 'Only Issue', url: 'https://github.com/acme/app/issues/3' }] }] })
    const source = vi.fn(async () => snapshot([first, second]))
    render(<App snapshotSource={source} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('tab', { name: 'Second work' }))
    fireEvent.click(screen.getByRole('button', { name: /Second 1개 업무/ }))
    expect(screen.getByRole('heading', { name: 'Only work' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Only Issue' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'GitHub에서 보기' })).toBeInTheDocument()
  })

  it('accepts only absolute http(s) external URLs', () => {
    expect(isSafeExternalURL('https://github.com/acme/app')).toBe(true)
    expect(isSafeExternalURL('http://ghe.local/acme/app')).toBe(true)
    for (const value of ['javascript:alert(1)', 'file:///etc/passwd', 'data:text/html,hi', '/relative', 'custom://host', 'not a url']) expect(isSafeExternalURL(value)).toBe(false)
  })

  it('does not assign href for a rejected external link', async () => {
    const unsafe = project({ workItems: [{ ...project().workItems[0], links: [{ kind: 'unsafe', label: 'Unsafe link', url: 'javascript:alert(1)' }] }] })
    const source = vi.fn(async () => snapshot([unsafe]))
    render(<App snapshotSource={source} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.queryByRole('link', { name: 'Unsafe link' })).not.toBeInTheDocument()
    expect(screen.getByText('Unsafe link')).toBeInTheDocument()
  })

  it('shows bounded recovery copy when work-info clipboard is unavailable', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
    const source = vi.fn(async () => snapshot([project()]))
    render(<App snapshotSource={source} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    expect(screen.getByRole('status')).toHaveTextContent('클립보드를 사용할 수 없습니다.')
  })

  it('shows bounded recovery copy when work-info clipboard write fails', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async () => { throw new Error('denied') } } })
    const source = vi.fn(async () => snapshot([project()]))
    render(<App snapshotSource={source} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(screen.getByRole('status')).toHaveTextContent('작업 정보를 복사하지 못했습니다.')
  })

  it('uses the Wails clipboard and reports success only for a true result', async () => {
    const clipboardSetText = vi.fn(async () => true)
    Object.defineProperty(window, 'runtime', { configurable: true, value: { ClipboardSetText: clipboardSetText } })
    const browserWriteText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: browserWriteText } })
    render(<App snapshotSource={vi.fn(async () => snapshot([project()]))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(clipboardSetText).toHaveBeenCalledWith(expect.stringContaining('Retry payment failures'))
    expect(browserWriteText).not.toHaveBeenCalled()
    expect(screen.getByRole('status')).toHaveTextContent('선택한 작업 정보를 클립보드에 복사했습니다.')
  })

  it('treats a false Wails clipboard result as failure without browser fallback', async () => {
    const clipboardSetText = vi.fn(async () => false)
    Object.defineProperty(window, 'runtime', { configurable: true, value: { ClipboardSetText: clipboardSetText } })
    const browserWriteText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: browserWriteText } })
    render(<App snapshotSource={vi.fn(async () => snapshot([project()]))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(clipboardSetText).toHaveBeenCalledTimes(1)
    expect(browserWriteText).not.toHaveBeenCalled()
    expect(screen.getByRole('status')).toHaveTextContent('작업 정보를 복사하지 못했습니다.')
  })

  it('treats a thrown Wails clipboard call as failure without browser fallback', async () => {
    const clipboardSetText = vi.fn(async () => { throw new Error('denied') })
    Object.defineProperty(window, 'runtime', { configurable: true, value: { ClipboardSetText: clipboardSetText } })
    const browserWriteText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: browserWriteText } })
    render(<App snapshotSource={vi.fn(async () => snapshot([project()]))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(clipboardSetText).toHaveBeenCalledTimes(1)
    expect(browserWriteText).not.toHaveBeenCalled()
    expect(screen.getByRole('status')).toHaveTextContent('작업 정보를 복사하지 못했습니다.')
  })

  it('uses browser clipboard only when the Wails clipboard API is absent', async () => {
    const browserWriteText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: browserWriteText } })
    render(<App snapshotSource={vi.fn(async () => snapshot([project()]))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(browserWriteText).toHaveBeenCalledWith(expect.stringContaining('Retry payment failures'))
    expect(screen.getByRole('status')).toHaveTextContent('선택한 작업 정보를 클립보드에 복사했습니다.')
  })

  it('does not report clipboard success for empty work info', async () => {
    const clipboardSetText = vi.fn(async () => true)
    Object.defineProperty(window, 'runtime', { configurable: true, value: { ClipboardSetText: clipboardSetText } })
    const browserWriteText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: browserWriteText } })
    render(<App snapshotSource={vi.fn(async () => snapshot([]))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(clipboardSetText).not.toHaveBeenCalled()
    expect(browserWriteText).not.toHaveBeenCalled()
    expect(screen.getByRole('status')).toHaveTextContent('복사할 작업 정보가 없습니다.')
  })

  it('marks the local connection as degraded when nested Herdr freshness is stale', async () => {
    const herdr = { source: 'herdr', schemaVersion: 1, revision: 1, observedAt: '2026-09-10T01:00:00Z', status: 'offline', syncStatus: 'offline', freshness: { state: 'stale', syncStatus: 'offline' }, notices: [], sessions: [], connections: [], unconnectedAgents: [] }
    render(<App snapshotSource={vi.fn(async () => snapshot([project()], { herdr }))} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    const nav = screen.getByRole('navigation', { name: '주요 메뉴' })
    expect(within(nav).getByText(/로컬 연결 확인 필요/)).toBeInTheDocument()
    expect(nav.querySelector('.status-mark')).toHaveClass('attention')
  })

  it('keeps only the product identity in the top navigation and omits forbidden runtime identifiers', async () => {
    const source = vi.fn(async () => snapshot([project()]))
    render(<App snapshotSource={source} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    const nav = screen.getByRole('navigation', { name: '주요 메뉴' })
    expect(within(nav).getByLabelText('ThreadDock Monitor')).toBeInTheDocument()
    for (const label of ['작업 현황', '저장소', '자동화 작업', '완료 기록', '설정']) expect(screen.queryByRole('button', { name: label })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '자동화 작업' })).not.toBeInTheDocument()
    expect(screen.queryByText(/providerSession|processId|sessionId|requestId|rawTranscript/i)).not.toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(/providerSession|processId|sessionId|requestId|rawTranscript/i)
  })
})
