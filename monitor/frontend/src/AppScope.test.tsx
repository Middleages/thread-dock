import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { StrictMode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'
import type { Snapshot } from './types'

const snapshot = (revision: number, title: string): Snapshot => ({
  schemaVersion: 2,
  revision,
  observedAt: '2026-09-11T04:00:00Z',
  freshness: { state: 'fresh', syncStatus: 'synced' },
  state: 'running',
  syncStatus: 'synced',
  nextAction: 'review',
  evidenceRefs: [],
  source: 'github',
  notices: [],
  projects: [{
    projectId: 'repo:acme/app', name: 'acme/app', state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [],
    workItems: [{ workId: `issue:${revision}`, title, state: revision === 1 ? 'open' : 'closed', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], decisions: [], handoffs: [], links: [], github: { kind: 'issue', number: revision, state: revision === 1 ? 'OPEN' : 'CLOSED', checks: [], relatedIssueUrls: [], relatedPullRequestUrls: [], fields: {}, contentAvailable: true } }],
    notices: [], links: [], source: 'github',
  }],
})

describe('work history scope', () => {
  afterEach(() => { cleanup(); vi.useRealTimers(); delete (window as Window & { go?: unknown }).go })

  it('loads open work first and calls the full source only after 전체 보기', async () => {
    const openSource = vi.fn(async () => snapshot(1, 'Open work'))
    const allSource = vi.fn(async () => snapshot(2, 'Closed history'))
    render(<App snapshotSource={openSource} allSnapshotSource={allSource} pollIntervalMs={60_000} />)

    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(openSource).toHaveBeenCalledTimes(1)
    expect(allSource).not.toHaveBeenCalled()
    expect(screen.getByText('Open work')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '작업 현황' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '전체 보기' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(allSource).toHaveBeenCalledTimes(1)
    expect(screen.getByText('Closed history')).toBeInTheDocument()
  })

  it('describes repository health as 조회 상태 instead of a synthetic workflow stage', async () => {
    render(<App snapshotSource={vi.fn(async () => snapshot(1, 'Open work'))} />)

    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('columnheader', { name: '조회 상태' })).toBeInTheDocument()
    expect(screen.queryByRole('columnheader', { name: '현재 단계' })).not.toBeInTheDocument()
    expect(screen.queryByRole('columnheader', { name: '다음 행동' })).not.toBeInTheDocument()
  })

  it('does not keep polling closed history after the user opens 전체 보기', async () => {
    vi.useFakeTimers()
    const openSource = vi.fn(async () => snapshot(1, 'Open work'))
    const allSource = vi.fn(async () => snapshot(2, 'Closed history'))
    render(<App snapshotSource={openSource} allSnapshotSource={allSource} pollIntervalMs={4000} />)

    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '전체 보기' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(allSource).toHaveBeenCalledTimes(1)

    await act(async () => { await vi.advanceTimersByTimeAsync(12000) })
    expect(allSource).toHaveBeenCalledTimes(1)
  })

  it('loads the global toolbox once under StrictMode and derives project Todo count from that cache', async () => {
    const openSource = vi.fn(async () => snapshot(1, 'Open work'))
    const getGlobal = vi.fn(async () => ({ commands: [{ id: 'cmd', label: '상태', command: 'herdr status' }], todos: [{ id: 'todo', text: 'Project todo', done: false, projectKey: 'repo:acme/app' }] }))
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetGlobalToolbox: getGlobal } } }
    render(<StrictMode><App snapshotSource={openSource} pollIntervalMs={60_000} /></StrictMode>)
    await act(async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() })
    expect(getGlobal).toHaveBeenCalledTimes(1)
    expect(screen.getByText('할 일 1개')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('herdr status')).toBeInTheDocument()
  })

  it('does not let the first project row steal focus from an already open Drawer', async () => {
    let resolveSnapshot!: (value: Snapshot) => void
    const source = vi.fn(() => new Promise<Snapshot>((resolve) => { resolveSnapshot = resolve }))
    render(<App snapshotSource={source} pollIntervalMs={60_000} />)
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    const close = screen.getByRole('button', { name: '닫기' })
    expect(close).toHaveFocus()
    await act(async () => { resolveSnapshot(snapshot(1, 'Deferred work')); await Promise.resolve(); await Promise.resolve() })
    expect(close).toHaveFocus()
  })

  it('does not turn a failed global load into writable empty data and retries explicitly', async () => {
    const getGlobal = vi.fn().mockRejectedValueOnce(new Error('toolbox offline')).mockRejectedValueOnce(new Error('toolbox still offline')).mockResolvedValueOnce({ commands: [], todos: [] })
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetGlobalToolbox: getGlobal } } }
    render(<App snapshotSource={vi.fn(async () => snapshot(1, 'Open work'))} pollIntervalMs={60_000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('alert')).toHaveTextContent('toolbox still offline')
    expect(screen.getByRole('button', { name: '다시 불러오기' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '다시 불러오기' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(getGlobal).toHaveBeenCalledTimes(3)
  })

  it('retries a failed global load when the drawer is closed and reopened', async () => {
    const getGlobal = vi.fn().mockRejectedValueOnce(new Error('reopen offline')).mockRejectedValueOnce(new Error('reopen still offline')).mockResolvedValueOnce({ commands: [{ id: 'cmd', label: '재시도', command: 'echo ok' }], todos: [] })
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetGlobalToolbox: getGlobal } } }
    render(<App snapshotSource={vi.fn(async () => snapshot(1, 'Open work'))} pollIntervalMs={60_000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('alert')).toHaveTextContent('reopen still offline')
    fireEvent.click(screen.getByRole('button', { name: '닫기' }))
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(getGlobal).toHaveBeenCalledTimes(3)
    expect(screen.getByText('echo ok')).toBeInTheDocument()
  })

  it('keeps the committed global cache and draft input when a Drawer save fails', async () => {
    const getGlobal = vi.fn(async () => ({ commands: [{ id: 'existing', label: '기존 명령', command: 'echo old' }], todos: [] }))
    const saveGlobal = vi.fn(async () => { throw new Error('global save failed') })
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetGlobalToolbox: getGlobal, SaveGlobalToolbox: saveGlobal } } }
    render(<App snapshotSource={vi.fn(async () => snapshot(1, 'Open work'))} pollIntervalMs={60_000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    await act(async () => { await Promise.resolve() })
    fireEvent.change(screen.getByRole('textbox', { name: '명령어 이름' }), { target: { value: '보존할 초안' } })
    fireEvent.change(screen.getByRole('textbox', { name: '명령어 내용' }), { target: { value: 'echo new' } })
    fireEvent.click(screen.getByRole('button', { name: '명령어 추가' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(saveGlobal).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('alert')).toHaveTextContent('global save failed')
    expect(screen.getByText('echo old')).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: '명령어 이름' })).toHaveValue('보존할 초안')
  })

  it('opens the project Todo shortcut with the matching filter', async () => {
    const getGlobal = vi.fn(async () => ({ commands: [], todos: [{ id: 'todo', text: 'Project todo', done: false, projectKey: 'repo:acme/app' }] }))
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetGlobalToolbox: getGlobal } } }
    render(<App snapshotSource={vi.fn(async () => snapshot(1, 'Open work'))} pollIntervalMs={60_000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '할 일 보기' }))
    expect(screen.getByRole('tab', { name: '할 일' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('combobox', { name: '할 일 범위' })).toHaveValue('project:repo:acme/app')
  })

  it('updates the project count after a successful save and resets normal Toolbox entry', async () => {
    const getGlobal = vi.fn(async () => ({ commands: [], todos: [] }))
    const saveGlobal = vi.fn(async (next) => next)
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetGlobalToolbox: getGlobal, SaveGlobalToolbox: saveGlobal } } }
    render(<App snapshotSource={vi.fn(async () => snapshot(1, 'Open work'))} pollIntervalMs={60_000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    fireEvent.click(screen.getByRole('tab', { name: '할 일' }))
    fireEvent.change(screen.getByRole('combobox', { name: '새 할 일 프로젝트' }), { target: { value: 'project:repo:acme/app' } })
    fireEvent.change(screen.getByRole('textbox', { name: '할 일 내용' }), { target: { value: '새 프로젝트 일' } })
    fireEvent.click(screen.getByRole('button', { name: '할 일 추가' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(saveGlobal).toHaveBeenCalledWith(expect.objectContaining({ todos: [expect.objectContaining({ projectKey: 'repo:acme/app' })] }))
    fireEvent.click(screen.getByRole('button', { name: '닫기' }))
    expect(screen.getByText('할 일 1개')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    expect(screen.getByRole('tab', { name: '명령어' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByRole('combobox', { name: '할 일 범위' })).not.toBeInTheDocument()
  })

  it('coalesces repeated mutation while a global save is deferred and keeps the saved command', async () => {
    const getGlobal = vi.fn(async () => ({ commands: [], todos: [] }))
    let resolveSave!: (value: { commands: Array<{ id: string; label: string; command: string }>; todos: [] }) => void
    const saveGlobal = vi.fn(() => new Promise((resolve) => { resolveSave = resolve }))
    ;(window as Window & { go?: unknown }).go = { main: { App: { GetGlobalToolbox: getGlobal, SaveGlobalToolbox: saveGlobal } } }
    render(<App snapshotSource={vi.fn(async () => snapshot(1, 'Open work'))} pollIntervalMs={60_000} />)
    await act(async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    fireEvent.change(screen.getByRole('textbox', { name: '명령어 이름' }), { target: { value: '저장 명령' } })
    fireEvent.change(screen.getByRole('textbox', { name: '명령어 내용' }), { target: { value: 'echo saved' } })
    const add = screen.getByRole('button', { name: '명령어 추가' })
    fireEvent.click(add)
    fireEvent.click(add)
    expect(saveGlobal).toHaveBeenCalledTimes(1)
    expect(add).toBeDisabled()
    resolveSave({ commands: [{ id: 'saved', label: '저장 명령', command: 'echo saved' }], todos: [] })
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.click(screen.getByRole('button', { name: '닫기' }))
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    expect(screen.getByText('echo saved')).toBeInTheDocument()
  })
})
