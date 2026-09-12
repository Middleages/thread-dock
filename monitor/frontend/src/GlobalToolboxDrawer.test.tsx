import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import type { ComponentProps } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GlobalToolboxDrawer } from './GlobalToolboxDrawer'
import type { GlobalToolbox } from './bindings'
import type { Project } from './types'

const project = (projectId: string, name = projectId, url = `https://github.com/repo/${projectId}`): Project => ({
  projectId,
  name,
  state: 'active',
  syncStatus: 'synced',
  nextAction: 'review',
  evidenceRefs: [],
  workItems: [],
  links: [{ kind: 'github', label: 'Repository', url }],
})

const baseToolbox = (): GlobalToolbox => ({
  commands: [],
  todos: [],
})

function renderDrawer(overrides: Partial<ComponentProps<typeof GlobalToolboxDrawer>> = {}) {
  const props: ComponentProps<typeof GlobalToolboxDrawer> = {
    open: true,
    projects: [project('alpha', 'Alpha'), project('beta', 'Beta')],
    toolbox: baseToolbox(),
    loading: false,
    saving: false,
    onReload: vi.fn(),
    onSave: vi.fn(async (next: GlobalToolbox) => next),
    onClose: vi.fn(),
    ...overrides,
  }
  return { ...render(<GlobalToolboxDrawer {...props} />), props }
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  Reflect.deleteProperty(window, 'runtime')
  Reflect.deleteProperty(navigator, 'clipboard')
})

describe('GlobalToolboxDrawer', () => {
  it('returns no dialog while closed', () => {
    renderDrawer({ open: false })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('opens on commands and switches to todos', () => {
    renderDrawer()
    expect(screen.getByRole('tab', { name: '명령어' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('heading', { name: '명령어' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    expect(screen.getByRole('tab', { name: '할 일' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('heading', { name: '할 일' })).toBeInTheDocument()
  })

  it('uses initial project shortcut to open todos with a project filter', () => {
    renderDrawer({ initialProjectKey: 'github.com/beta' })
    expect(screen.getByRole('tab', { name: '할 일' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('combobox', { name: '할 일 범위' })).toHaveValue('project:github.com/beta')
  })

  it('deduplicates project choices while preserving their first order', () => {
    renderDrawer({ projects: [project('alpha', 'Alpha'), project('alpha', 'Alias'), project('beta', 'Beta')] })
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    expect(screen.getAllByRole('option', { name: 'Alpha' })).toHaveLength(2)
    expect(screen.queryByRole('option', { name: 'Alias' })).not.toBeInTheDocument()
    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual(['전체', '공통', 'Alpha', 'Beta', '공통', 'Alpha', 'Beta'])
  })

  it('reapplies the shortcut when the drawer is reopened or the project changes', () => {
    const view = renderDrawer()
    const props = view.props
    view.rerender(<GlobalToolboxDrawer {...props} open={false} />)
    view.rerender(<GlobalToolboxDrawer {...props} open initialProjectKey="github.com/beta" />)
    expect(screen.getByRole('tab', { name: '할 일' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('combobox', { name: '할 일 범위' })).toHaveValue('project:github.com/beta')
    view.rerender(<GlobalToolboxDrawer {...props} open initialProjectKey="github.com/alpha" />)
    expect(screen.getByRole('combobox', { name: '할 일 범위' })).toHaveValue('project:github.com/alpha')
  })

  it('dismisses from close, backdrop and Escape but not inside clicks', () => {
    const onClose = vi.fn()
    renderDrawer({ onClose })
    const dialog = screen.getByRole('dialog')
    fireEvent.click(within(dialog).getByRole('tab', { name: '명령어' }))
    expect(onClose).not.toHaveBeenCalled()
    fireEvent.click(screen.getByTestId('toolbox-backdrop'))
    expect(onClose).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(2)
    fireEvent.click(screen.getByRole('button', { name: '닫기' }))
    expect(onClose).toHaveBeenCalledTimes(3)
  })

  it('adds and copies a command without offering execution', async () => {
    const onSave = vi.fn(async (next: GlobalToolbox) => next)
    const clipboard = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: clipboard } })
    renderDrawer({ onSave })

    fireEvent.change(screen.getByRole('textbox', { name: '명령어 이름' }), { target: { value: '로그 확인' } })
    fireEvent.change(screen.getByRole('textbox', { name: '명령어 내용' }), { target: { value: 'herdr status' } })
    fireEvent.change(screen.getByRole('textbox', { name: '명령어 메모' }), { target: { value: '상태를 읽습니다' } })
    fireEvent.click(screen.getByRole('button', { name: '명령어 추가' }))
    await act(async () => { await Promise.resolve() })
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ commands: [expect.objectContaining({ label: '로그 확인', command: 'herdr status', note: '상태를 읽습니다' })] }))
    expect(screen.queryByRole('button', { name: /실행/ })).not.toBeInTheDocument()

    const saved = onSave.mock.calls[0][0]
    cleanup()
    renderDrawer({ toolbox: saved, onSave })
    fireEvent.click(screen.getByRole('button', { name: '명령어 복사: 로그 확인' }))
    await act(async () => { await Promise.resolve() })
    expect(clipboard).toHaveBeenCalledWith('herdr status')
  })

  it('adds, filters and completes todos with incomplete items shown by default', async () => {
    const onSave = vi.fn(async (next: GlobalToolbox) => next)
    renderDrawer({
      toolbox: { commands: [], todos: [{ id: 'done', text: '완료된 일', done: true, projectKey: 'github.com/repo:alpha' }] },
      onSave,
    })
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    expect(screen.queryByText('완료된 일')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('checkbox', { name: '완료 항목 포함' }))
    expect(screen.getByText('완료된 일')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('checkbox', { name: '완료된 일' }))
    await act(async () => { await Promise.resolve() })
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ todos: [expect.objectContaining({ id: 'done', done: false })] }))

    fireEvent.change(screen.getByRole('textbox', { name: '할 일 내용' }), { target: { value: '공통 할 일' } })
    fireEvent.click(screen.getByRole('button', { name: '할 일 추가' }))
    await act(async () => { await Promise.resolve() })
    const latest = onSave.mock.calls.at(-1)?.[0]
    expect(latest?.todos).toEqual(expect.arrayContaining([expect.objectContaining({ text: '공통 할 일' })]))
    expect(latest?.todos.find((todo) => todo.text === '공통 할 일')).not.toHaveProperty('projectKey')
  })

  it('keeps unknown project keys visible and offers reassignment', async () => {
    const onSave = vi.fn(async (next: GlobalToolbox) => next)
    renderDrawer({ toolbox: { commands: [], todos: [{ id: 'unknown', text: '옛 업무', done: false, projectKey: 'retired-project' }] }, onSave })
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    expect(screen.getAllByText('알 수 없는 프로젝트 (retired-project)').length).toBeGreaterThan(0)
    fireEvent.change(screen.getByRole('combobox', { name: '옛 업무 프로젝트' }), { target: { value: 'project:github.com/alpha' } })
    await act(async () => { await Promise.resolve() })
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ todos: [expect.objectContaining({ projectKey: 'github.com/alpha' })] }))
  })

  it('does not mutate while loading, unavailable, or saving, and retries load errors', () => {
    const onReload = vi.fn()
    const onSave = vi.fn(async (next: GlobalToolbox) => next)
    renderDrawer({ toolbox: null, loading: true, onReload, onSave })
    expect(screen.getByText('Toolbox를 불러오는 중입니다.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '명령어 추가' })).toBeDisabled()

    cleanup()
    renderDrawer({ toolbox: null, loading: false, loadError: '읽기 실패', onReload, onSave })
    expect(screen.getByRole('alert')).toHaveTextContent('읽기 실패')
    fireEvent.click(screen.getByRole('button', { name: '다시 불러오기' }))
    expect(onReload).toHaveBeenCalledTimes(1)
  })

  it('preserves draft input and committed data when save fails', async () => {
    const onSave = vi.fn(async () => { throw new Error('저장 실패') })
    renderDrawer({ onSave })
    const label = screen.getByRole('textbox', { name: '명령어 이름' })
    fireEvent.change(label, { target: { value: '보존할 초안' } })
    fireEvent.change(screen.getByRole('textbox', { name: '명령어 내용' }), { target: { value: 'echo keep' } })
    fireEvent.click(screen.getByRole('button', { name: '명령어 추가' }))
    await act(async () => { await Promise.resolve() })
    expect(screen.getByRole('alert')).toHaveTextContent('저장 실패')
    expect(label).toHaveValue('보존할 초안')
    expect(screen.queryByText('보존할 초안')).not.toBeInTheDocument()
  })

  it('traps focus in the dialog and restores the opener on close', () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()
    const onClose = vi.fn()
    renderDrawer({ onClose })
    expect(screen.getByRole('button', { name: '닫기' })).toHaveFocus()
    const dialog = screen.getByRole('dialog')
    const focusables = dialog.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled])')
    const last = focusables[focusables.length - 1]
    last.focus()
    fireEvent.keyDown(document, { key: 'Tab' })
    expect(document.activeElement).toBe(screen.getByRole('button', { name: '닫기' }))
    fireEvent.click(screen.getByRole('button', { name: '닫기' }))
    expect(onClose).toHaveBeenCalled()
  })
})
