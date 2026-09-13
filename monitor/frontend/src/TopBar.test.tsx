import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { TopBar } from './TopBar'

describe('TopBar', () => {
  afterEach(cleanup)
  it('shows product identity, connection state, and toolbox/settings actions', () => {
    const onOpenToolbox = vi.fn()
    const onOpenSettings = vi.fn()
    render(<TopBar connectionDegraded={false} onOpenToolbox={onOpenToolbox} onOpenSettings={onOpenSettings} />)
    expect(screen.getByLabelText('ThreadDock Monitor')).toBeInTheDocument()
    expect(screen.getByText('로컬 연결 정상')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Toolbox 열기' }))
    fireEvent.click(screen.getByRole('button', { name: '설정 열기' }))
    expect(onOpenToolbox).toHaveBeenCalledTimes(1)
    expect(onOpenSettings).toHaveBeenCalledTimes(1)
  })

  it('announces degraded local connection with text and attention mark', () => {
    render(<TopBar connectionDegraded onOpenToolbox={vi.fn()} onOpenSettings={vi.fn()} />)
    expect(screen.getByText('로컬 연결 확인 필요')).toBeInTheDocument()
    expect(screen.getByLabelText('로컬 연결 상태')).toHaveClass('attention')
  })
})
