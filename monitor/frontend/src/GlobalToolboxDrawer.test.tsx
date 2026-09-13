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

const projectWithoutLink = (projectId: string, name = projectId): Project => ({
  projectId,
  name,
  state: 'active',
  syncStatus: 'synced',
  nextAction: 'review',
  evidenceRefs: [],
  workItems: [],
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

  it('preserves a linked project key when checking a todo', async () => {
    const onSave = vi.fn(async (next: GlobalToolbox) => next)
    renderDrawer({ toolbox: { commands: [], todos: [
      { id: 'linked', text: '연결된 일', done: false, projectKey: 'github.com/alpha' },
      { id: 'unknown-linked', text: '알 수 없는 연결 일', done: false, projectKey: 'retired-project' },
    ] }, onSave })
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    fireEvent.click(screen.getByRole('checkbox', { name: '연결된 일' }))
    await act(async () => { await Promise.resolve() })
    expect(onSave).toHaveBeenLastCalledWith(expect.objectContaining({ todos: [
      { id: 'linked', text: '연결된 일', done: true, projectKey: 'github.com/alpha' },
      { id: 'unknown-linked', text: '알 수 없는 연결 일', done: false, projectKey: 'retired-project' },
    ] }))

    cleanup()
    onSave.mockClear()
    renderDrawer({ toolbox: { commands: [], todos: [{ id: 'unknown-linked', text: '알 수 없는 연결 일', done: false, projectKey: 'retired-project' }] }, onSave })
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    fireEvent.click(screen.getByRole('checkbox', { name: '알 수 없는 연결 일' }))
    await act(async () => { await Promise.resolve() })
    expect(onSave).toHaveBeenLastCalledWith(expect.objectContaining({ todos: [{ id: 'unknown-linked', text: '알 수 없는 연결 일', done: true, projectKey: 'retired-project' }] }))
  })

  it('switches between all, common, and project todo filters', () => {
    renderDrawer({ toolbox: { commands: [], todos: [
      { id: 'common', text: '공통 일', done: false },
      { id: 'alpha', text: 'Alpha 일', done: false, projectKey: 'github.com/alpha' },
      { id: 'beta', text: 'Beta 일', done: false, projectKey: 'github.com/beta' },
    ] } })
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    expect(screen.getByText('공통 일')).toBeInTheDocument()
    expect(screen.getByText('Alpha 일')).toBeInTheDocument()
    expect(screen.getByText('Beta 일')).toBeInTheDocument()
    const filter = screen.getByRole('combobox', { name: '할 일 범위' })
    fireEvent.change(filter, { target: { value: 'common' } })
    expect(screen.getByText('공통 일')).toBeInTheDocument()
    expect(screen.queryByText('Alpha 일')).not.toBeInTheDocument()
    fireEvent.change(filter, { target: { value: 'project:github.com/alpha' } })
    expect(screen.getByText('Alpha 일')).toBeInTheDocument()
    expect(screen.queryByText('공통 일')).not.toBeInTheDocument()
  })

  it('creates a todo with exactly the selected project key', async () => {
    const onSave = vi.fn(async (next: GlobalToolbox) => next)
    renderDrawer({ onSave })
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    fireEvent.change(screen.getByRole('combobox', { name: '새 할 일 프로젝트' }), { target: { value: 'project:github.com/beta' } })
    fireEvent.change(screen.getByRole('textbox', { name: '할 일 내용' }), { target: { value: 'Beta 준비' } })
    fireEvent.click(screen.getByRole('button', { name: '할 일 추가' }))
    await act(async () => { await Promise.resolve() })
    expect(onSave).toHaveBeenLastCalledWith(expect.objectContaining({ todos: [expect.objectContaining({ text: 'Beta 준비', done: false, projectKey: 'github.com/beta' })] }))
  })

  it('deletes commands and todos through the controlled save', async () => {
    const onSave = vi.fn(async (next: GlobalToolbox) => next)
    renderDrawer({ toolbox: { commands: [{ id: 'cmd', label: '상태', command: 'herdr status' }], todos: [{ id: 'todo', text: '삭제할 일', done: false }] }, onSave })
    fireEvent.click(screen.getByRole('button', { name: '명령어 삭제: 상태' }))
    await act(async () => { await Promise.resolve() })
    expect(onSave).toHaveBeenLastCalledWith({ commands: [], todos: [{ id: 'todo', text: '삭제할 일', done: false }] })
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    fireEvent.click(screen.getByRole('button', { name: '삭제' }))
    await act(async () => { await Promise.resolve() })
    expect(onSave).toHaveBeenLastCalledWith({ commands: [{ id: 'cmd', label: '상태', command: 'herdr status' }], todos: [] })
  })

  it('keeps real all/common project keys distinct from the common filter', () => {
    renderDrawer({
      projects: [projectWithoutLink('all', 'All 프로젝트'), projectWithoutLink('common', 'Common 프로젝트')],
      toolbox: { commands: [], todos: [
        { id: 'all', text: 'All key 일', done: false, projectKey: 'all' },
        { id: 'common', text: 'Common key 일', done: false, projectKey: 'common' },
        { id: 'shared', text: '공통 일', done: false },
      ] },
    })
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    const filter = screen.getByRole('combobox', { name: '할 일 범위' })
    fireEvent.change(filter, { target: { value: 'common' } })
    expect(screen.getByText('공통 일')).toBeInTheDocument()
    expect(screen.queryByText('All key 일')).not.toBeInTheDocument()
    fireEvent.change(filter, { target: { value: 'project:all' } })
    expect(screen.getByText('All key 일')).toBeInTheDocument()
    expect(screen.queryByText('공통 일')).not.toBeInTheDocument()
    fireEvent.change(filter, { target: { value: 'project:common' } })
    expect(screen.getByText('Common key 일')).toBeInTheDocument()
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

  it('can reassign an unknown todo to common without retaining its raw key', async () => {
    const onSave = vi.fn(async (next: GlobalToolbox) => next)
    renderDrawer({ toolbox: { commands: [], todos: [{ id: 'unknown', text: '옛 업무', done: false, projectKey: 'retired-project' }] }, onSave })
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    fireEvent.change(screen.getByRole('combobox', { name: '옛 업무 프로젝트' }), { target: { value: 'common' } })
    await act(async () => { await Promise.resolve() })
    const saved = onSave.mock.calls.at(-1)?.[0]
    expect(saved?.todos[0]).not.toHaveProperty('projectKey')
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

  it('does not use browser clipboard after native false or rejection', async () => {
    const browserClipboard = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: browserClipboard } })
    const nativeClipboard = vi.fn(async () => false)
    Object.defineProperty(window, 'runtime', { configurable: true, value: { ClipboardSetText: nativeClipboard } })
    renderDrawer({ toolbox: { commands: [{ id: 'cmd', label: '상태', command: 'herdr status' }], todos: [] } })
    fireEvent.click(screen.getByRole('button', { name: '명령어 복사: 상태' }))
    await act(async () => { await Promise.resolve() })
    expect(browserClipboard).not.toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent('명령어를 복사하지 못했습니다.')

    cleanup()
    const rejectingNative = vi.fn(async () => { throw new Error('native failure') })
    Object.defineProperty(window, 'runtime', { configurable: true, value: { ClipboardSetText: rejectingNative } })
    renderDrawer({ toolbox: { commands: [{ id: 'cmd', label: '상태', command: 'herdr status' }], todos: [] } })
    fireEvent.click(screen.getByRole('button', { name: '명령어 복사: 상태' }))
    await act(async () => { await Promise.resolve() })
    expect(browserClipboard).not.toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent('명령어를 복사하지 못했습니다.')
  })

  it('blocks duplicate in-flight saves and all mutations while saving', async () => {
    let resolveSave!: (value: GlobalToolbox) => void
    const onSave = vi.fn(() => new Promise<GlobalToolbox>((resolve) => { resolveSave = resolve }))
    renderDrawer({ onSave })
    fireEvent.change(screen.getByRole('textbox', { name: '명령어 이름' }), { target: { value: '상태' } })
    fireEvent.change(screen.getByRole('textbox', { name: '명령어 내용' }), { target: { value: 'herdr status' } })
    const add = screen.getByRole('button', { name: '명령어 추가' })
    fireEvent.click(add)
    fireEvent.click(add)
    expect(onSave).toHaveBeenCalledTimes(1)
    expect(add).toBeDisabled()
    resolveSave({ commands: [], todos: [] })
    await act(async () => { await Promise.resolve() })

    cleanup()
    renderDrawer({ saving: true, onSave })
    expect(screen.getByRole('button', { name: '명령어 추가' })).toBeDisabled()
    expect(onSave).toHaveBeenCalledTimes(1)
  })

  it('traps focus in the dialog and restores the opener on controlled close', () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()
    const onClose = vi.fn()
    const view = renderDrawer({ onClose })
    expect(screen.getByRole('button', { name: '닫기' })).toHaveFocus()
    const dialog = screen.getByRole('dialog')
    const focusables = dialog.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled])')
    const last = focusables[focusables.length - 1]
    last.focus()
    fireEvent.keyDown(document, { key: 'Tab' })
    expect(document.activeElement).toBe(screen.getByRole('button', { name: '닫기' }))
    fireEvent.click(screen.getByRole('button', { name: '닫기' }))
    view.rerender(<GlobalToolboxDrawer {...view.props} open={false} onClose={onClose} />)
    expect(opener).toHaveFocus()
    expect(onClose).toHaveBeenCalled()
  })

  it('restores focus and removes listeners when unmounted while open', () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()
    const { unmount } = renderDrawer()
    unmount()
    expect(opener).toHaveFocus()
    fireEvent.keyDown(document, { key: 'Escape' })
  })
})
