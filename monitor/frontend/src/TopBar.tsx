import './top-bar.css'

export type ConnectionStatus = 'healthy' | 'attention' | 'disabled' | 'checking'

const statusCopy: Record<ConnectionStatus, { label: string; tone: 'success' | 'attention' | 'progress' }> = {
  healthy: { label: '정상', tone: 'success' },
  attention: { label: '확인 필요', tone: 'attention' },
  disabled: { label: '미설정', tone: 'progress' },
  checking: { label: '확인 중', tone: 'progress' },
}

function ConnectionBadge({ name, status }: { name: string; status: ConnectionStatus }) {
  const current = statusCopy[status]
  return <div className="connection"><span className={`status-mark ${current.tone}`} aria-label={`${name} 상태`} />{name} {current.label}</div>
}

export function TopBar({ githubStatus, herdrStatus, onOpenToolbox, onOpenSettings }: { githubStatus: ConnectionStatus; herdrStatus: ConnectionStatus; onOpenToolbox: () => void; onOpenSettings: () => void }) {
  return <header className="top-bar">
    <div className="brand" aria-label="ThreadDock Monitor">Thread<span>Dock</span></div>
    <div className="top-bar-health" aria-label="연결 상태">
      <ConnectionBadge name="GitHub" status={githubStatus} />
      <ConnectionBadge name="Herdr" status={herdrStatus} />
    </div>
    <div className="top-bar-actions">
      <button type="button" onClick={onOpenToolbox}>Toolbox 열기</button>
      <button type="button" onClick={onOpenSettings}>설정 열기</button>
    </div>
  </header>
}
