import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ProjectReferences } from './ProjectReferences'
import { getProjectReferences, openToolboxReference, saveProjectReferences } from './bindings'
import { openExternalURL } from './safe-url'

vi.mock('./bindings', async () => {
  const actual = await vi.importActual<typeof import('./bindings')>('./bindings')
  return {
    ...actual,
    getProjectReferences: vi.fn(),
    saveProjectReferences: vi.fn(),
    openToolboxReference: vi.fn(),
  }
})

vi.mock('./safe-url', async () => {
  const actual = await vi.importActual<typeof import('./safe-url')>('./safe-url')
  return { ...actual, openExternalURL: vi.fn() }
})

const mockedGet = vi.mocked(getProjectReferences)
const mockedSave = vi.mocked(saveProjectReferences)
const mockedOpenPath = vi.mocked(openToolboxReference)
const mockedOpenURL = vi.mocked(openExternalURL)

const flush = async () => {
  await act(async () => { await Promise.resolve(); await Promise.resolve() })
}

describe('ProjectReferences', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    Reflect.deleteProperty(navigator, 'clipboard')
    Reflect.deleteProperty(window, 'runtime')
  })

  it('loads and displays only project references', async () => {
    mockedGet.mockResolvedValue({ references: [{ id: 'r1', label: '문서', type: 'web', target: 'https://example.com/docs' }] })

    render(<ProjectReferences projectKey="github.example/repo:acme/app" projectName="acme/app" />)
    await flush()

    expect(mockedGet).toHaveBeenCalledWith('github.example/repo:acme/app')
    expect(screen.getByText('문서')).toBeInTheDocument()
    expect(screen.getByText('문서').parentElement).toHaveTextContent('https://example.com/docs')
    expect(screen.queryByText('명령어')).not.toBeInTheDocument()
    expect(screen.queryByText('체크리스트')).not.toBeInTheDocument()
  })

  it('opens web links safely and routes files through the native binding', async () => {
    mockedGet.mockResolvedValue({ references: [
      { id: 'web', label: 'Web', type: 'web', target: 'https://example.com' },
      { id: 'file', label: 'Windows', type: 'file', target: 'C:\\work\\README.md' },
      { id: 'wsl', label: 'WSL', type: 'wsl-file', target: '/home/app/README.md' },
      { id: 'bad', label: 'Unsafe', type: 'web', target: 'javascript:alert(1)' },
    ] })
    mockedOpenURL.mockImplementation((value) => value.startsWith('http'))

    render(<ProjectReferences projectKey="project" projectName="Project" />)
    await flush()
    const openButtons = screen.getAllByRole('button', { name: /열기$/ })

    fireEvent.click(openButtons[0])
    fireEvent.click(openButtons[1])
    fireEvent.click(openButtons[2])
    fireEvent.click(openButtons[3])

    expect(mockedOpenURL).toHaveBeenCalledWith('https://example.com')
    expect(mockedOpenPath).toHaveBeenNthCalledWith(1, 'file', 'C:\\work\\README.md')
    expect(mockedOpenPath).toHaveBeenNthCalledWith(2, 'wsl-file', '/home/app/README.md')
    expect(screen.getByRole('alert')).toHaveTextContent('웹 링크를 열 수 없습니다.')
  })

  it('copies a reference target using the native clipboard first', async () => {
    mockedGet.mockResolvedValue({ references: [{ id: 'r1', label: '자료', type: 'file', target: 'C:\\work\\README.md' }] })
    const clipboardSetText = vi.fn(async () => true)
    Object.defineProperty(window, 'runtime', { configurable: true, value: { ClipboardSetText: clipboardSetText } })

    render(<ProjectReferences projectKey="project" projectName="Project" />)
    await flush()
    fireEvent.click(screen.getByRole('button', { name: /복사$/ }))
    await flush()

    expect(clipboardSetText).toHaveBeenCalledWith('C:\\work\\README.md')
  })

  it.each([
    ['returns false', async () => false],
    ['rejects', async () => { throw new Error('native clipboard failed') }],
  ])('does not fall back to browser clipboard when native clipboard %s', async (_caseName, nativeClipboard) => {
    mockedGet.mockResolvedValue({ references: [{ id: 'r1', label: '자료', type: 'file', target: 'C:\\work\\README.md' }] })
    const browserClipboard = vi.fn(async () => undefined)
    const clipboardSetText = vi.fn(nativeClipboard)
    Object.defineProperty(window, 'runtime', { configurable: true, value: { ClipboardSetText: clipboardSetText } })
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: browserClipboard } })

    render(<ProjectReferences projectKey="project" projectName="Project" />)
    await flush()
    fireEvent.click(screen.getByRole('button', { name: /복사$/ }))
    await flush()

    expect(clipboardSetText).toHaveBeenCalledWith('C:\\work\\README.md')
    expect(browserClipboard).not.toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent('자료 경로를 복사하지 못했습니다.')
  })

  it('adds and deletes references through the project save binding', async () => {
    mockedGet.mockResolvedValue({ references: [] })
    mockedSave.mockImplementation(async (_projectKey, value) => value)

    render(<ProjectReferences projectKey="project" projectName="Project" />)
    await flush()
    fireEvent.change(screen.getByLabelText('자료 이름'), { target: { value: 'Runbook' } })
    fireEvent.change(screen.getByLabelText('자료 주소 또는 경로'), { target: { value: 'https://example.com/runbook' } })
    fireEvent.click(screen.getByRole('button', { name: '자료 추가' }))
    await flush()

    expect(mockedSave).toHaveBeenCalledWith('project', { references: [expect.objectContaining({ label: 'Runbook', type: 'web', target: 'https://example.com/runbook' })] })
    expect(screen.getByText('Runbook')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Runbook 삭제' }))
    await flush()
    expect(mockedSave).toHaveBeenLastCalledWith('project', { references: [] })
  })

  it('persists exact Windows and WSL reference types and targets', async () => {
    mockedGet.mockResolvedValue({ references: [] })
    mockedSave.mockImplementation(async (_projectKey, value) => value)

    render(<ProjectReferences projectKey="project" projectName="Project" />)
    await flush()
    fireEvent.change(screen.getByLabelText('자료 이름'), { target: { value: 'Windows 파일' } })
    fireEvent.change(screen.getByLabelText('자료 유형'), { target: { value: 'file' } })
    fireEvent.change(screen.getByLabelText('자료 주소 또는 경로'), { target: { value: 'C:\\work\\README.md' } })
    fireEvent.click(screen.getByRole('button', { name: '자료 추가' }))
    await flush()
    expect(mockedSave).toHaveBeenLastCalledWith('project', { references: [expect.objectContaining({ label: 'Windows 파일', type: 'file', target: 'C:\\work\\README.md' })] })

    fireEvent.change(screen.getByLabelText('자료 이름'), { target: { value: 'WSL 파일' } })
    fireEvent.change(screen.getByLabelText('자료 유형'), { target: { value: 'wsl-file' } })
    fireEvent.change(screen.getByLabelText('자료 주소 또는 경로'), { target: { value: '/home/app/README.md' } })
    fireEvent.click(screen.getByRole('button', { name: '자료 추가' }))
    await flush()
    expect(mockedSave).toHaveBeenLastCalledWith('project', { references: expect.arrayContaining([
      expect.objectContaining({ label: 'Windows 파일', type: 'file', target: 'C:\\work\\README.md' }),
      expect.objectContaining({ label: 'WSL 파일', type: 'wsl-file', target: '/home/app/README.md' }),
    ]) })
  })

  it('does not save after a failed load and offers an explicit retry', async () => {
    mockedGet.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ references: [] })

    render(<ProjectReferences projectKey="project" projectName="Project" />)
    await flush()
    expect(screen.getByRole('alert')).toHaveTextContent('offline')
    expect(screen.getByRole('button', { name: '다시 시도' })).toBeInTheDocument()
    expect(mockedSave).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '다시 시도' }))
    await flush()
    expect(mockedGet).toHaveBeenCalledTimes(2)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('preserves the input and confirmed list when save fails', async () => {
    mockedGet.mockResolvedValue({ references: [{ id: 'r1', label: '기존', type: 'web', target: 'https://example.com' }] })
    mockedSave.mockRejectedValue(new Error('disk full'))

    render(<ProjectReferences projectKey="project" projectName="Project" />)
    await flush()
    fireEvent.change(screen.getByLabelText('자료 이름'), { target: { value: '새 자료' } })
    fireEvent.change(screen.getByLabelText('자료 주소 또는 경로'), { target: { value: 'https://example.com/new' } })
    fireEvent.click(screen.getByRole('button', { name: '자료 추가' }))
    await flush()

    expect(screen.getByRole('alert')).toHaveTextContent('disk full')
    expect(screen.getByText('기존')).toBeInTheDocument()
    expect(screen.getByLabelText('자료 이름')).toHaveValue('새 자료')
    expect(screen.getByLabelText('자료 주소 또는 경로')).toHaveValue('https://example.com/new')
  })

  it('ignores an old load response after switching projects', async () => {
    let resolveOld!: (value: { references: [] }) => void
    const oldLoad = new Promise<{ references: [] }>((resolve) => { resolveOld = resolve })
    mockedGet.mockReturnValueOnce(oldLoad).mockResolvedValueOnce({ references: [{ id: 'new', label: '새 프로젝트 자료', type: 'web', target: 'https://new.example' }] })

    const { rerender } = render(<ProjectReferences projectKey="old" projectName="Old" />)
    rerender(<ProjectReferences projectKey="new" projectName="New" />)
    await flush()
    resolveOld({ references: [] })
    await flush()

    expect(screen.getByText('새 프로젝트 자료')).toBeInTheDocument()
    expect(screen.queryByText('Old')).not.toBeInTheDocument()
  })

  it('does not let a pending save response overwrite a switched project', async () => {
    mockedGet.mockResolvedValue({ references: [] })
    let resolveSave!: (value: { references: Array<{ id: string; label: string; type: 'web'; target: string }> }) => void
    mockedSave.mockReturnValueOnce(new Promise((resolve) => { resolveSave = resolve }))

    const { rerender } = render(<ProjectReferences projectKey="old" projectName="Old" />)
    await flush()
    fireEvent.change(screen.getByLabelText('자료 이름'), { target: { value: 'Old item' } })
    fireEvent.change(screen.getByLabelText('자료 주소 또는 경로'), { target: { value: 'https://old.example' } })
    fireEvent.click(screen.getByRole('button', { name: '자료 추가' }))
    rerender(<ProjectReferences projectKey="new" projectName="New" />)
    resolveSave({ references: [{ id: 'old', label: 'Old item', type: 'web', target: 'https://old.example' }] })
    await flush()

    expect(screen.queryByText('Old item')).not.toBeInTheDocument()
    expect(screen.getByText('등록한 자료가 없습니다.')).toBeInTheDocument()
  })
})
