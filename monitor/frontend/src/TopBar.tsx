import './top-bar.css'

export function TopBar({ connectionDegraded, onOpenToolbox, onOpenSettings }: { connectionDegraded: boolean; onOpenToolbox: () => void; onOpenSettings: () => void }) {
  return <header className="top-bar">
    <div className="brand" aria-label="ThreadDock Monitor">Thread<span>Dock</span></div>
    <div className={`connection${connectionDegraded ? ' degraded' : ''}`}><span className={`status-mark ${connectionDegraded ? 'attention' : 'success'}`} aria-label="로컬 연결 상태" />로컬 연결 {connectionDegraded ? '확인 필요' : '정상'}</div>
    <div className="top-bar-actions">
      <button type="button" onClick={onOpenToolbox}>Toolbox 열기</button>
      <button type="button" onClick={onOpenSettings}>설정 열기</button>
    </div>
  </header>
}
