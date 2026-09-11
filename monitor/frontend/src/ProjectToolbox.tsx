import { useEffect, useState } from 'react'
import {
  getProjectToolbox,
  openToolboxReference,
  saveProjectToolbox,
  type ProjectToolbox as ProjectToolboxValue,
  type ToolboxReferenceType,
} from './bindings'
import { openExternalURL } from './safe-url'
import './project-toolbox.css'

const emptyToolbox = (): ProjectToolboxValue => ({ references: [], commands: [], checklist: [] })

const newID = (prefix: string) => {
  const random = globalThis.crypto?.randomUUID?.()
  return `${prefix}-${random ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`}`
}

async function copyText(value: string) {
  const wails = window.runtime?.ClipboardSetText
  if (wails) {
    if (!(await wails(value))) throw new Error('clipboard rejected')
    return
  }
  if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable')
  await navigator.clipboard.writeText(value)
}

export function ProjectToolbox({ projectKey, projectName, onStatus }: { projectKey: string; projectName: string; onStatus?: (message: string) => void }) {
  const [toolbox, setToolbox] = useState<ProjectToolboxValue>(emptyToolbox)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const [referenceLabel, setReferenceLabel] = useState('')
  const [referenceType, setReferenceType] = useState<ToolboxReferenceType>('web')
  const [referenceTarget, setReferenceTarget] = useState('')
  const [commandLabel, setCommandLabel] = useState('')
  const [commandText, setCommandText] = useState('')
  const [commandNote, setCommandNote] = useState('')
  const [checkText, setCheckText] = useState('')

  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    void getProjectToolbox(projectKey).then((value) => {
      if (active) setToolbox({ references: value.references ?? [], commands: value.commands ?? [], checklist: value.checklist ?? [] })
    }).catch((cause) => {
      if (active) setError(cause instanceof Error ? cause.message : 'Toolbox를 불러오지 못했습니다.')
    }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [projectKey])

  const persist = async (next: ProjectToolboxValue, message: string) => {
    setSaving(true)
    setError('')
    try {
      const saved = await saveProjectToolbox(projectKey, next)
      setToolbox({ references: saved.references ?? [], commands: saved.commands ?? [], checklist: saved.checklist ?? [] })
      onStatus?.(message)
      return true
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Toolbox를 저장하지 못했습니다.')
      return false
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <section className="project-toolbox" aria-label={`${projectName} Toolbox`}><p className="muted">Toolbox를 불러오는 중입니다.</p></section>

  return <section className="project-toolbox" aria-labelledby="toolbox-title">
    <div className="toolbox-heading">
      <div><h3 id="toolbox-title">Project Toolbox</h3><p>{projectName}에서 자주 찾는 자료와 명령어, 개인 체크리스트를 로컬에 보관합니다.</p></div>
      <span>Agent에 자동 전달하지 않음</span>
    </div>
    {error && <p className="toolbox-error" role="alert">{error}</p>}

    <section className="toolbox-section" aria-labelledby="toolbox-references-title">
      <div className="toolbox-section-title"><h4 id="toolbox-references-title">자료</h4><span>{toolbox.references.length}개</span></div>
      <div className="toolbox-list">
        {toolbox.references.length === 0 && <p className="muted">등록한 자료가 없습니다.</p>}
        {toolbox.references.map((item) => <div className="toolbox-row" key={item.id}>
          <div><strong>{item.label}</strong><small>{item.type === 'web' ? 'Web' : item.type === 'wsl-file' ? 'WSL 파일' : '로컬 파일'} · {item.target}</small></div>
          <div className="toolbox-actions">
            <button type="button" disabled={saving} onClick={() => {
              if (item.type === 'web') {
                if (!openExternalURL(item.target)) setError('웹 링크를 열 수 없습니다.')
                return
              }
              void openToolboxReference(item.type, item.target).catch((cause) => setError(cause instanceof Error ? cause.message : '파일을 열 수 없습니다.'))
            }}>열기</button>
            <button type="button" onClick={() => void copyText(item.target).then(() => onStatus?.('자료 경로를 복사했습니다.')).catch(() => setError('자료 경로를 복사하지 못했습니다.'))}>복사</button>
            <button type="button" className="danger-lite" disabled={saving} onClick={() => void persist({ ...toolbox, references: toolbox.references.filter((value) => value.id !== item.id) }, '자료를 삭제했습니다.')}>삭제</button>
          </div>
        </div>)}
      </div>
      <div className="toolbox-form toolbox-reference-form">
        <input aria-label="자료 이름" placeholder="자료 이름" value={referenceLabel} onChange={(event) => setReferenceLabel(event.target.value)} />
        <select aria-label="자료 유형" value={referenceType} onChange={(event) => setReferenceType(event.target.value as ToolboxReferenceType)}>
          <option value="web">Web</option><option value="file">Windows 파일</option><option value="wsl-file">WSL 파일</option>
        </select>
        <input aria-label="자료 주소 또는 경로" placeholder={referenceType === 'web' ? 'https://...' : referenceType === 'wsl-file' ? '/home/...' : 'C:\\...'} value={referenceTarget} onChange={(event) => setReferenceTarget(event.target.value)} />
        <button type="button" disabled={saving || !referenceLabel.trim() || !referenceTarget.trim()} onClick={() => {
          const next = { ...toolbox, references: [...toolbox.references, { id: newID('ref'), label: referenceLabel.trim(), type: referenceType, target: referenceTarget.trim() }] }
          void persist(next, '자료를 추가했습니다.').then((ok) => { if (ok) { setReferenceLabel(''); setReferenceTarget('') } })
        }}>추가</button>
      </div>
    </section>

    <section className="toolbox-section" aria-labelledby="toolbox-commands-title">
      <div className="toolbox-section-title"><h4 id="toolbox-commands-title">명령어</h4><span>{toolbox.commands.length}개 · 복사 전용</span></div>
      <div className="toolbox-list">
        {toolbox.commands.length === 0 && <p className="muted">등록한 명령어가 없습니다.</p>}
        {toolbox.commands.map((item) => <div className="toolbox-command" key={item.id}>
          <div className="toolbox-command-heading"><div><strong>{item.label}</strong>{item.note && <small>{item.note}</small>}</div><div className="toolbox-actions"><button type="button" onClick={() => void copyText(item.command).then(() => onStatus?.('명령어를 복사했습니다.')).catch(() => setError('명령어를 복사하지 못했습니다.'))}>복사</button><button type="button" className="danger-lite" disabled={saving} onClick={() => void persist({ ...toolbox, commands: toolbox.commands.filter((value) => value.id !== item.id) }, '명령어를 삭제했습니다.')}>삭제</button></div></div>
          <pre><code>{item.command}</code></pre>
        </div>)}
      </div>
      <div className="toolbox-command-form">
        <input aria-label="명령어 이름" placeholder="예: PostgreSQL 접속" value={commandLabel} onChange={(event) => setCommandLabel(event.target.value)} />
        <input aria-label="명령어 메모" placeholder="메모 (선택)" value={commandNote} onChange={(event) => setCommandNote(event.target.value)} />
        <textarea aria-label="명령어 내용" rows={3} placeholder="psql -h localhost -U postgres -d app" value={commandText} onChange={(event) => setCommandText(event.target.value)} />
        <button type="button" disabled={saving || !commandLabel.trim() || !commandText.trim()} onClick={() => {
          const next = { ...toolbox, commands: [...toolbox.commands, { id: newID('cmd'), label: commandLabel.trim(), command: commandText.trim(), note: commandNote.trim() }] }
          void persist(next, '명령어를 추가했습니다.').then((ok) => { if (ok) { setCommandLabel(''); setCommandText(''); setCommandNote('') } })
        }}>추가</button>
      </div>
    </section>

    <section className="toolbox-section" aria-labelledby="toolbox-checklist-title">
      <div className="toolbox-section-title"><h4 id="toolbox-checklist-title">체크리스트</h4><span>{toolbox.checklist.filter((item) => item.done).length}/{toolbox.checklist.length}</span></div>
      <div className="toolbox-checklist">
        {toolbox.checklist.length === 0 && <p className="muted">등록한 체크리스트가 없습니다.</p>}
        {toolbox.checklist.map((item) => <label key={item.id} className={item.done ? 'checked' : ''}><input type="checkbox" checked={item.done} disabled={saving} onChange={(event) => void persist({ ...toolbox, checklist: toolbox.checklist.map((value) => value.id === item.id ? { ...value, done: event.target.checked } : value) }, '체크리스트를 저장했습니다.')} /><span>{item.text}</span><button type="button" className="danger-lite" disabled={saving} onClick={(event) => { event.preventDefault(); void persist({ ...toolbox, checklist: toolbox.checklist.filter((value) => value.id !== item.id) }, '체크리스트 항목을 삭제했습니다.') }}>삭제</button></label>)}
      </div>
      <div className="toolbox-form toolbox-check-form"><input aria-label="체크리스트 내용" placeholder="체크할 내용" value={checkText} onChange={(event) => setCheckText(event.target.value)} onKeyDown={(event) => {
        if (event.key !== 'Enter' || !checkText.trim() || saving) return
        event.preventDefault()
        const next = { ...toolbox, checklist: [...toolbox.checklist, { id: newID('check'), text: checkText.trim(), done: false }] }
        void persist(next, '체크리스트 항목을 추가했습니다.').then((ok) => { if (ok) setCheckText('') })
      }} /><button type="button" disabled={saving || !checkText.trim()} onClick={() => {
        const next = { ...toolbox, checklist: [...toolbox.checklist, { id: newID('check'), text: checkText.trim(), done: false }] }
        void persist(next, '체크리스트 항목을 추가했습니다.').then((ok) => { if (ok) setCheckText('') })
      }}>추가</button></div>
    </section>
  </section>
}
