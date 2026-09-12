import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ProjectDetail } from './ProjectDetail'
import type { Project } from './types'

vi.mock('./bindings', async () => {
  const actual = await vi.importActual<typeof import('./bindings')>('./bindings')
  return { ...actual, getProjectReferences: vi.fn(async () => ({ references: [] })), saveProjectReferences: vi.fn(async (_key, value) => value), openToolboxReference: vi.fn() }
})

const project = (overrides: Partial<Project> = {}): Project => ({
  projectId: 'repo:acme/app', name: 'acme/app', state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [],
  updatedAt: '2026-09-11T04:00:00Z', links: [{ kind: 'github', label: 'Repository', url: 'https://github.com/repo/acme/app' }],
  workItems: [{ workId: 'issue:7', title: 'Improve retries', state: 'needs_operator', syncStatus: 'synced', nextAction: 'approve', evidenceRefs: ['state://7'],
    request: 'Reduce duplicate retries', handoffs: [{ summary: 'Review the change', nextAction: 'approve', evidenceRefs: ['handoff://7'] }], decisions: [], links: [{ kind: 'issue', label: 'Issue #7', url: 'https://github.com/acme/app/issues/7' }], github: { kind: 'issue', number: 7, url: 'https://github.com/acme/app/issues/7', state: 'OPEN', checks: [], contentAvailable: true } }],
  ...overrides,
})

afterEach(() => { cleanup(); Reflect.deleteProperty(window, 'runtime'); Reflect.deleteProperty(navigator, 'clipboard') })

describe('ProjectDetail', () => {
  it('keeps project tabs to 업무 and 자료 and shows issue identity', async () => {
    render(<ProjectDetail project={project()} selectedWorkId="issue:7" projectTodoCount={0} herdr={undefined} onSelectWork={vi.fn()} onOpenProjectTodos={vi.fn()} onStatus={vi.fn()} />)
    expect(screen.getByRole('tab', { name: '업무' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '자료' })).toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Toolbox' })).not.toBeInTheDocument()
    expect(screen.getAllByText('Issue #7').length).toBeGreaterThan(0)
    fireEvent.click(screen.getByRole('tab', { name: '자료' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('heading', { name: '자료' })).toBeInTheDocument()
  })

  it('copies handoff text and only shows the safe current GitHub action', async () => {
    const writeText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    render(<ProjectDetail project={project()} selectedWorkId="issue:7" projectTodoCount={0} onSelectWork={vi.fn()} onOpenProjectTodos={vi.fn()} onStatus={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'GitHub에서 보기' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '작업 정보 복사' }))
    await act(async () => { await Promise.resolve() })
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('업무: Improve retries'))
  })

  it('shows the incomplete Todo count and opens the global drawer on the project key', () => {
    const onOpenProjectTodos = vi.fn()
    render(<ProjectDetail project={project()} selectedWorkId="issue:7" projectTodoCount={2} onSelectWork={vi.fn()} onOpenProjectTodos={onOpenProjectTodos} onStatus={vi.fn()} />)
    expect(screen.getByText('할 일 2개')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '할 일 보기' }))
    expect(onOpenProjectTodos).toHaveBeenCalledWith('github.com/repo:acme/app')
  })

  it('does not render a GitHub action for an unsafe work URL', () => {
    const unsafe = project({ workItems: [{ ...project().workItems[0], links: [{ kind: 'issue', label: 'Unsafe', url: 'javascript:alert(1)' }], github: { kind: 'issue', number: 7, url: 'javascript:alert(1)', state: 'OPEN', checks: [] } }] })
    render(<ProjectDetail project={unsafe} selectedWorkId="issue:7" projectTodoCount={0} onSelectWork={vi.fn()} onOpenProjectTodos={vi.fn()} onStatus={vi.fn()} />)
    expect(screen.queryByRole('button', { name: 'GitHub에서 보기' })).not.toBeInTheDocument()
  })
})
