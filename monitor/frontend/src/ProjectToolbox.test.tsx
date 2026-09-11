import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ProjectToolbox } from './ProjectToolbox'
import { getProjectToolbox, saveProjectToolbox } from './bindings'

vi.mock('./bindings', async () => {
  const actual = await vi.importActual<typeof import('./bindings')>('./bindings')
  return { ...actual, getProjectToolbox: vi.fn(), saveProjectToolbox: vi.fn(), openToolboxReference: vi.fn() }
})

const mockedGet = vi.mocked(getProjectToolbox)
const mockedSave = vi.mocked(saveProjectToolbox)

describe('ProjectToolbox', () => {
  afterEach(() => { cleanup(); vi.clearAllMocks(); Reflect.deleteProperty(navigator, 'clipboard') })

  it('loads project-local references, commands and checklist without agent automation', async () => {
    mockedGet.mockResolvedValue({
      references: [{ id: 'r1', label: 'Confluence', type: 'web', target: 'https://example.com/wiki' }],
      commands: [{ id: 'c1', label: 'psql', command: 'psql -d app' }],
      checklist: [{ id: 'k1', text: 'Smoke test', done: false }],
    })
    render(<ProjectToolbox projectKey="github.example/repo:acme/app" projectName="acme/app" />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })

    expect(mockedGet).toHaveBeenCalledWith('github.example/repo:acme/app')
    expect(screen.getByText('Confluence')).toBeInTheDocument()
    expect(screen.getByText('psql -d app')).toBeInTheDocument()
    expect(screen.getByText('Smoke test')).toBeInTheDocument()
    expect(screen.getByText('Agent에 자동 전달하지 않음')).toBeInTheDocument()
  })

  it('copies commands instead of executing them', async () => {
    mockedGet.mockResolvedValue({ references: [], commands: [{ id: 'c1', label: 'psql', command: 'psql -d app' }], checklist: [] })
    const writeText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    render(<ProjectToolbox projectKey="project" projectName="Project" />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })

    fireEvent.click(screen.getByRole('button', { name: '복사' }))
    await act(async () => { await Promise.resolve() })
    expect(writeText).toHaveBeenCalledWith('psql -d app')
  })

  it('persists checklist toggles immediately', async () => {
    mockedGet.mockResolvedValue({ references: [], commands: [], checklist: [{ id: 'k1', text: 'Smoke test', done: false }] })
    mockedSave.mockImplementation(async (_key, value) => value)
    render(<ProjectToolbox projectKey="project" projectName="Project" />)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })

    fireEvent.click(screen.getByRole('checkbox'))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(mockedSave).toHaveBeenCalledWith('project', expect.objectContaining({ checklist: [{ id: 'k1', text: 'Smoke test', done: true }] }))
  })
})
