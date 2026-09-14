import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { TopBar } from './TopBar'

describe('TopBar', () => {
  afterEach(cleanup)

  it('shows GitHub and Herdr health separately with toolbox/settings actions', () => {
    const onOpenToolbox = vi.fn()
    const onOpenSettings = vi.fn()
    render(<TopBar githubStatus="healthy" herdrStatus="healthy" onOpenToolbox={onOpenToolbox} onOpenSettings={onOpenSettings} />)
    expect(screen.getByLabelText('ThreadDock Monitor')).toBeInTheDocument()
    expect(screen.getByText('GitHub 정상')).toBeInTheDocument()
    expect(screen.getByText('Herdr 정상')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    fireEvent.click(screen.getByRole('button', { name: '설정 열기' }))
    expect(onOpenToolbox).toHaveBeenCalledTimes(1)
    expect(onOpenSettings).toHaveBeenCalledTimes(1)
  })

  it('does not label a GitHub-only failure as a local or Herdr failure', () => {
    render(<TopBar githubStatus="attention" herdrStatus="healthy" onOpenToolbox={vi.fn()} onOpenSettings={vi.fn()} />)
    expect(screen.getByText('GitHub 확인 필요')).toBeInTheDocument()
    expect(screen.getByLabelText('GitHub 상태')).toHaveClass('attention')
    expect(screen.getByText('Herdr 정상')).toBeInTheDocument()
    expect(screen.getByLabelText('Herdr 상태')).toHaveClass('success')
    expect(screen.queryByText(/로컬 연결/)).not.toBeInTheDocument()
  })

  it('shows an intentionally disabled Herdr connection as 미설정 instead of failure', () => {
    render(<TopBar githubStatus="healthy" herdrStatus="disabled" onOpenToolbox={vi.fn()} onOpenSettings={vi.fn()} />)
    expect(screen.getByText('GitHub 정상')).toBeInTheDocument()
    expect(screen.getByText('Herdr 미설정')).toBeInTheDocument()
    expect(screen.getByLabelText('Herdr 상태')).toHaveClass('progress')
  })
})
