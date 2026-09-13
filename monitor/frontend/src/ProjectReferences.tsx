import { useCallback, useEffect, useRef, useState } from 'react'
import {
  getProjectReferences,
  openToolboxReference,
  saveProjectReferences,
  type ProjectReferences as ProjectReferencesValue,
  type ToolboxReference,
  type ToolboxReferenceType,
} from './bindings'
import { openExternalURL } from './safe-url'
import './project-references.css'

const newID = () => {
  const random = globalThis.crypto?.randomUUID?.()
  return `ref-${random ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`}`
}

const normalizeReferences = (value: ProjectReferencesValue): ToolboxReference[] => (
  Array.isArray(value.references) ? value.references : []
)

async function copyText(value: string) {
  const wails = window.runtime?.ClipboardSetText
  if (wails) {
    if (!(await wails(value))) throw new Error('clipboard rejected')
    return
  }
  if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable')
  await navigator.clipboard.writeText(value)
}

export function ProjectReferences({ projectKey, projectName, onStatus }: { projectKey: string; projectName: string; onStatus?: (message: string) => void }) {
  const [references, setReferences] = useState<ToolboxReference[]>([])
  const [loading, setLoading] = useState(true)
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const generationRef = useRef(0)

  const [referenceLabel, setReferenceLabel] = useState('')
  const [referenceType, setReferenceType] = useState<ToolboxReferenceType>('web')
  const [referenceTarget, setReferenceTarget] = useState('')

  const load = useCallback(() => {
    const generation = ++generationRef.current
    setLoading(true)
    setLoaded(false)
    setSaving(false)
    setError('')
    setReferences([])
    setReferenceLabel('')
    setReferenceTarget('')

    void getProjectReferences(projectKey).then((value) => {
      if (generation !== generationRef.current) return
      setReferences(normalizeReferences(value))
      setLoaded(true)
    }).catch((cause) => {
      if (generation !== generationRef.current) return
      setError(cause instanceof Error ? cause.message : '자료를 불러오지 못했습니다.')
    }).finally(() => {
      if (generation === generationRef.current) setLoading(false)
    })
  }, [projectKey])

  useEffect(() => {
    load()
  }, [load])

  const persist = async (next: ToolboxReference[], message: string) => {
    const generation = generationRef.current
    setSaving(true)
    setError('')
    try {
      const saved = await saveProjectReferences(projectKey, { references: next })
      if (generation !== generationRef.current) return false
      setReferences(normalizeReferences(saved))
      onStatus?.(message)
      return true
    } catch (cause) {
      if (generation === generationRef.current) setError(cause instanceof Error ? cause.message : '자료를 저장하지 못했습니다.')
      return false
    } finally {
      if (generation === generationRef.current) setSaving(false)
    }
  }

  const openReference = (reference: ToolboxReference) => {
    if (reference.type === 'web') {
      if (!openExternalURL(reference.target)) setError('웹 링크를 열 수 없습니다.')
      return
    }
    void Promise.resolve(openToolboxReference(reference.type, reference.target)).catch((cause) => {
      setError(cause instanceof Error ? cause.message : '파일을 열 수 없습니다.')
    })
  }

  const addReference = () => {
    const label = referenceLabel.trim()
    const target = referenceTarget.trim()
    if (!loaded || saving || !label || !target) return
    void persist([...references, { id: newID(), label, type: referenceType, target }], '자료를 추가했습니다.').then((ok) => {
      if (ok) {
        setReferenceLabel('')
        setReferenceTarget('')
      }
    })
  }

  if (loading) {
    return <section className="project-references" aria-label={`${projectName} 자료`}><p className="muted">자료를 불러오는 중입니다.</p></section>
  }

  if (!loaded) {
    return <section className="project-references" aria-label={`${projectName} 자료`}>
      <p className="references-error" role="alert">{error || '자료를 불러오지 못했습니다.'}</p>
      <button type="button" className="references-retry" onClick={load}>다시 시도</button>
    </section>
  }

  return <section className="project-references" aria-labelledby="project-references-title">
    <div className="references-heading">
      <div>
        <h3 id="project-references-title">자료</h3>
        <p>{projectName}에서 자주 찾는 링크와 경로를 로컬에 보관합니다.</p>
      </div>
      <span>{references.length}개</span>
    </div>
    {error && <p className="references-error" role="alert">{error}</p>}
    <div className="references-list">
      {references.length === 0 && <p className="muted references-empty">등록한 자료가 없습니다.</p>}
      {references.map((reference) => <div className="reference-row" key={reference.id}>
        <div className="reference-copy">
          <strong>{reference.label}</strong>
          <small>{reference.type === 'web' ? 'Web' : reference.type === 'wsl-file' ? 'WSL 파일' : 'Windows 파일'} · {reference.target}</small>
        </div>
        <div className="reference-actions">
          <button type="button" aria-label={`${reference.label} 열기`} onClick={() => openReference(reference)}>열기</button>
          <button type="button" aria-label={`${reference.label} 복사`} onClick={() => void copyText(reference.target).then(() => onStatus?.('자료 경로를 복사했습니다.')).catch(() => setError('자료 경로를 복사하지 못했습니다.'))}>복사</button>
          <button type="button" className="danger-lite" disabled={saving} aria-label={`${reference.label} 삭제`} onClick={() => void persist(references.filter((value) => value.id !== reference.id), '자료를 삭제했습니다.')}>삭제</button>
        </div>
      </div>)}
    </div>
    <div className="references-form">
      <label>
        <span>자료 이름</span>
        <input aria-label="자료 이름" placeholder="자료 이름" value={referenceLabel} onChange={(event) => setReferenceLabel(event.target.value)} disabled={saving} />
      </label>
      <label>
        <span>자료 유형</span>
        <select aria-label="자료 유형" value={referenceType} onChange={(event) => setReferenceType(event.target.value as ToolboxReferenceType)} disabled={saving}>
          <option value="web">Web</option>
          <option value="file">Windows 파일</option>
          <option value="wsl-file">WSL 파일</option>
        </select>
      </label>
      <label className="reference-target-field">
        <span>자료 주소 또는 경로</span>
        <input aria-label="자료 주소 또는 경로" placeholder={referenceType === 'web' ? 'https://...' : referenceType === 'wsl-file' ? '/home/...' : 'C:\\...'} value={referenceTarget} onChange={(event) => setReferenceTarget(event.target.value)} disabled={saving} />
      </label>
      <button type="button" className="references-add" disabled={saving || !referenceLabel.trim() || !referenceTarget.trim()} onClick={addReference}>자료 추가</button>
    </div>
  </section>
}
