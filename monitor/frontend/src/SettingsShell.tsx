import { useCallback, useEffect, useState } from 'react'
import { App } from './App'
import { getMonitorSettings, saveMonitorSettings, type MonitorSettings } from './bindings'
import './settings.css'

const emptySettings: MonitorSettings = {
  githubHost: 'github.com',
  repositories: '',
  projects: '',
  wslDistribution: '',
  sessionsFile: '',
}

const listForEditor = (value: string) => value.split(',').map((item) => item.trim()).filter(Boolean).join('\n')
const listForStore = (value: string) => value.split(/[\n,]+/).map((item) => item.trim()).filter(Boolean).join(',')

export function SettingsShell() {
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState<MonitorSettings>(emptySettings)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  const close = useCallback(() => {
    if (saving) return
    setOpen(false)
    setError('')
    setMessage('')
  }, [saving])

  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, close])

  const showSettings = async () => {
    setOpen(true)
    setLoading(true)
    setError('')
    setMessage('')
    try {
      const settings = await getMonitorSettings()
      setForm({
        ...settings,
        githubHost: settings.githubHost || 'github.com',
        repositories: listForEditor(settings.repositories),
        projects: listForEditor(settings.projects),
      })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '설정을 불러오지 못했습니다.')
    } finally {
      setLoading(false)
    }
  }

  const update = (key: keyof MonitorSettings, value: string) => setForm((current) => ({ ...current, [key]: value }))

  const save = async (event: React.FormEvent) => {
    event.preventDefault()
    setSaving(true)
    setError('')
    setMessage('')
    const next: MonitorSettings = {
      githubHost: form.githubHost.trim(),
      repositories: listForStore(form.repositories),
      projects: listForStore(form.projects),
      wslDistribution: form.wslDistribution.trim(),
      sessionsFile: form.sessionsFile.trim(),
    }
    try {
      const saved = await saveMonitorSettings(next)
      setForm({
        ...saved,
        githubHost: saved.githubHost || 'github.com',
        repositories: listForEditor(saved.repositories),
        projects: listForEditor(saved.projects),
      })
      setMessage('저장했습니다. Monitor가 새 설정으로 다시 연결됩니다.')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '설정을 저장하지 못했습니다.')
    } finally {
      setSaving(false)
    }
  }

  return <>
    <App onOpenSettings={() => void showSettings()} />
    {open && <div className="settings-backdrop" onMouseDown={(event) => { if (event.currentTarget === event.target) close() }}>
      <section className="settings-dialog" role="dialog" aria-modal="true" aria-labelledby="settings-title">
        <header className="settings-header">
          <div><p className="settings-kicker">ThreadDock Monitor</p><h1 id="settings-title">연결 설정</h1><p>GitHub 업무와 WSL의 Herdr 관찰 대상을 지정합니다.</p></div>
          <button type="button" className="settings-close" aria-label="설정 닫기" onClick={close}>×</button>
        </header>
        {loading ? <div className="settings-loading" role="status">설정을 불러오는 중입니다.</div> : <form className="settings-form" onSubmit={(event) => void save(event)}>
          <label>
            <span>GitHub Host</span>
            <small><code>https://</code> 없이 호스트 이름만 입력합니다. GitHub.com은 <code>github.com</code>입니다.</small>
            <input aria-label="GitHub Host" value={form.githubHost} onChange={(event) => update('githubHost', event.target.value)} placeholder="github.samsungds.net" autoFocus />
          </label>
          <label>
            <span>GitHub 저장소</span>
            <small><code>OWNER/REPO</code> 형식으로 한 줄에 하나씩 입력합니다.</small>
            <textarea aria-label="GitHub 저장소" rows={3} value={form.repositories} onChange={(event) => update('repositories', event.target.value)} placeholder={'FDYPhotoDX/thread-dock\nFDYPhotoDX/jmj'} />
          </label>
          <label>
            <span>GitHub Projects</span>
            <small>위 GitHub Host의 <code>/views/2</code>가 아닌 Project 루트 URL을 입력합니다.</small>
            <textarea aria-label="GitHub Projects" rows={2} value={form.projects} onChange={(event) => update('projects', event.target.value)} placeholder="https://github.samsungds.net/orgs/FDYPhotoDX/projects/4" />
          </label>
          <div className="settings-grid">
            <label>
              <span>WSL 배포판</span>
              <small><code>wsl -l</code>에 표시되는 이름입니다.</small>
              <input aria-label="WSL 배포판" value={form.wslDistribution} onChange={(event) => update('wslDistribution', event.target.value)} placeholder="Ubuntu" />
            </label>
            <label>
              <span>Herdr 연결 파일</span>
              <small>WSL 내부의 절대 경로입니다. Herdr를 쓰지 않으면 비워둘 수 있습니다.</small>
              <input aria-label="Herdr 연결 파일" value={form.sessionsFile} onChange={(event) => update('sessionsFile', event.target.value)} placeholder="/home/appuser/.threaddock/sessions.json" />
            </label>
          </div>
          {error && <div className="settings-error" role="alert">{error}</div>}
          {message && <div className="settings-success" role="status">{message}</div>}
          <footer className="settings-actions">
            <button type="button" className="settings-cancel" onClick={close} disabled={saving}>취소</button>
            <button type="submit" className="settings-save" disabled={saving}>{saving ? '저장 중…' : '저장'}</button>
          </footer>
        </form>}
      </section>
    </div>}
  </>
}
