import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SettingsShell } from './SettingsShell'

const { getSettings, saveSettings } = vi.hoisted(() => ({
  getSettings: vi.fn(async () => ({ githubHost: 'github.com', repositories: 'acme/app', projects: '', wslDistribution: 'Ubuntu', sessionsFile: '/tmp/sessions.json' })),
  saveSettings: vi.fn(async (value: { githubHost: string; repositories: string; projects: string; wslDistribution: string; sessionsFile: string }) => value),
}))

vi.mock('./App', () => ({ App: ({ onOpenSettings }: { onOpenSettings: () => void }) => <button type="button" onClick={onOpenSettings}>mock app settings</button> }))
vi.mock('./bindings', async () => {
  const actual = await vi.importActual<typeof import('./bindings')>('./bindings')
  return { ...actual, getMonitorSettings: getSettings, saveMonitorSettings: saveSettings }
})

afterEach(() => { cleanup(); vi.clearAllMocks() })

describe('SettingsShell', () => {
  it('opens settings from App callback without a floating launcher', async () => {
    render(<SettingsShell />)
    expect(screen.queryByRole('button', { name: /설정$/ })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'mock app settings' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(screen.getByRole('dialog', { name: '연결 설정' })).toBeInTheDocument()
    expect(getSettings).toHaveBeenCalledTimes(1)
  })

  it('keeps the existing settings payload and save semantics', async () => {
    render(<SettingsShell />)
    fireEvent.click(screen.getByRole('button', { name: 'mock app settings' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    fireEvent.change(screen.getByLabelText('GitHub 저장소'), { target: { value: 'acme/app\nacme/other' } })
    fireEvent.click(screen.getByRole('button', { name: '저장' }))
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(saveSettings).toHaveBeenCalledWith(expect.objectContaining({ repositories: 'acme/app,acme/other', githubHost: 'github.com' }))
  })
})
