import { useEffect, useLayoutEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import type { GlobalToolbox, ToolboxCommand, ToolboxTodo } from './bindings'
import { toolboxKeyFor } from './project-key'
import type { Project } from './types'
import './global-toolbox-drawer.css'

type TodoFilter = { kind: 'all' } | { kind: 'common' } | { kind: 'project'; key: string }

const allFilter = 'all'
const commonFilter = 'common'
const projectFilterPrefix = 'project:'

const newID = (prefix: string) => {
  const random = globalThis.crypto?.randomUUID?.()
  return `${prefix}-${random ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`}`
}

const trimOptional = (value: string) => value.trim() || undefined

const filterToken = (filter: TodoFilter) => filter.kind === 'project' ? `${projectFilterPrefix}${filter.key}` : filter.kind

const parseFilter = (value: string): TodoFilter => {
  if (value === commonFilter) return { kind: 'common' }
  if (value.startsWith(projectFilterPrefix)) return { kind: 'project', key: value.slice(projectFilterPrefix.length) }
  return { kind: 'all' }
}

const projectLabel = (projects: Array<{ key: string; name: string }>, key: string) => {
  const project = projects.find((item) => item.key === key)
  return project?.name ?? `알 수 없는 프로젝트 (${key})`
}

async function copyText(value: string) {
  const runtime = (window as Window & { runtime?: { ClipboardSetText?: (text: string) => Promise<boolean> } }).runtime
  if (runtime?.ClipboardSetText) {
    if (!(await runtime.ClipboardSetText(value))) throw new Error('clipboard rejected')
    return
  }
  if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable')
  await navigator.clipboard.writeText(value)
}

export function GlobalToolboxDrawer({
  open,
  projects,
  initialProjectKey,
  toolbox,
  loadError,
  loading,
  saving,
  onReload,
  onSave,
  onClose,
  onStatus,
}: {
  open: boolean
  projects: Project[]
  initialProjectKey?: string
  toolbox: GlobalToolbox | null
  loadError?: string
  loading: boolean
  saving: boolean
  onReload: () => void
  onSave: (next: GlobalToolbox) => Promise<GlobalToolbox>
  onClose: () => void
  onStatus?: (message: string) => void
}) {
  const panelRef = useRef<HTMLDivElement>(null)
  const openerRef = useRef<HTMLElement | null>(null)
  const onCloseRef = useRef(onClose)
  const wasOpenRef = useRef(false)
  const restoreFocus = () => {
    const opener = openerRef.current
    openerRef.current = null
    if (opener && opener.isConnected) opener.focus()
  }
  const [tab, setTab] = useState<'commands' | 'todos'>(() => initialProjectKey ? 'todos' : 'commands')
  const [todoFilter, setTodoFilter] = useState<TodoFilter>(() => initialProjectKey ? { kind: 'project', key: initialProjectKey } : { kind: 'all' })
  const [includeCompleted, setIncludeCompleted] = useState(false)
  const [commandLabel, setCommandLabel] = useState('')
  const [commandText, setCommandText] = useState('')
  const [commandNote, setCommandNote] = useState('')
  const [todoText, setTodoText] = useState('')
  const [todoProjectKey, setTodoProjectKey] = useState(initialProjectKey ?? '')
  const [error, setError] = useState('')
  const [pendingSave, setPendingSave] = useState(false)

  const projectChoices = useMemo(() => {
    const seen = new Set<string>()
    return projects.flatMap((project) => {
      const key = toolboxKeyFor(project)
      if (seen.has(key)) return []
      seen.add(key)
      return [{ key, name: project.name }]
    })
  }, [projects])

  const blocked = loading || saving || pendingSave || toolbox === null || Boolean(loadError)
  const committed = toolbox ?? { commands: [], todos: [] }
  const commands = committed.commands ?? []
  const todos = committed.todos ?? []
  const selectedProject = todoFilter.kind === 'project' ? todoFilter.key : undefined
  const visibleTodos = todos.filter((todo) => {
    if (!includeCompleted && todo.done) return false
    if (todoFilter.kind === 'common') return !todo.projectKey
    if (todoFilter.kind === 'project') return todo.projectKey === todoFilter.key
    return true
  })

  useLayoutEffect(() => {
    if (!open) return
    openerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
  }, [open])

  useEffect(() => {
    onCloseRef.current = onClose
  }, [onClose])

  useEffect(() => {
    if (!open) {
      restoreFocus()
      return
    }
    return () => restoreFocus()
  }, [open])

  useEffect(() => {
    if (!open) {
      wasOpenRef.current = false
      return
    }
    if (!wasOpenRef.current || initialProjectKey !== undefined) {
      if (initialProjectKey) {
        setTab('todos')
        setTodoFilter({ kind: 'project', key: initialProjectKey })
      } else if (!wasOpenRef.current) {
        setTab('commands')
        setTodoFilter({ kind: 'all' })
      }
      wasOpenRef.current = true
    }
  }, [open, initialProjectKey])

  useEffect(() => {
    setTodoProjectKey(selectedProject ?? '')
  }, [selectedProject])

  useEffect(() => {
    if (!open) return
    const panel = panelRef.current
    if (!panel) return
    const focusables = () => Array.from(panel.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [href]'))
    const closeButton = panel.querySelector<HTMLElement>('[data-drawer-close]')
    closeButton?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        requestClose()
        return
      }
      if (event.key !== 'Tab') return
      const items = focusables()
      if (items.length === 0) return
      const first = items[0]
      const last = items[items.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  // requestClose and focusables are intentionally local to this effect's listener.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const requestClose = () => {
    onCloseRef.current()
  }

  const save = async (next: GlobalToolbox, successMessage: string) => {
    if (blocked) return false
    setPendingSave(true)
    setError('')
    try {
      await onSave({ commands: next.commands, todos: next.todos })
      onStatus?.(successMessage)
      return true
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Toolbox를 저장하지 못했습니다.')
      return false
    } finally {
      setPendingSave(false)
    }
  }

  const addCommand = async (event: FormEvent) => {
    event.preventDefault()
    if (!commandLabel.trim() || !commandText.trim()) return
    const next: GlobalToolbox = {
      ...committed,
      commands: [...commands, { id: newID('cmd'), label: commandLabel.trim(), command: commandText.trim(), note: trimOptional(commandNote) }],
    }
    if (await save(next, '명령어를 추가했습니다.')) {
      setCommandLabel('')
      setCommandText('')
      setCommandNote('')
    }
  }

  const addTodo = async (event: FormEvent) => {
    event.preventDefault()
    if (!todoText.trim()) return
    const todo: ToolboxTodo = { id: newID('todo'), text: todoText.trim(), done: false }
    if (todoProjectKey) todo.projectKey = todoProjectKey
    if (await save({ ...committed, todos: [...todos, todo] }, '할 일을 추가했습니다.')) setTodoText('')
  }

  const updateTodo = async (todo: ToolboxTodo, update: Partial<ToolboxTodo>, message: string) => {
    const nextTodos = todos.map((item) => {
      if (item.id !== todo.id) return item
      const updated = { ...item, ...update }
      if (Object.prototype.hasOwnProperty.call(update, 'projectKey') && update.projectKey === undefined) delete updated.projectKey
      return updated
    })
    await save({ ...committed, todos: nextTodos }, message)
  }

  if (!open) return null

  const reportedError = error || loadError
  const knownKeys = new Set(projectChoices.map((item) => item.key))
  const filterOptions = [
    <option key={allFilter} value={allFilter}>전체</option>,
    <option key={commonFilter} value={commonFilter}>공통</option>,
    ...projectChoices.map((project) => <option key={project.key} value={`${projectFilterPrefix}${project.key}`}>{project.name}</option>),
  ]

  return <div className="toolbox-drawer-backdrop" data-testid="toolbox-backdrop" onClick={(event) => { if (event.target === event.currentTarget) requestClose() }}>
    <section className="toolbox-drawer" ref={panelRef} role="dialog" aria-modal="true" aria-labelledby="global-toolbox-title">
      <header className="toolbox-drawer-header">
        <div><h2 id="global-toolbox-title">빠른 작업</h2></div>
        <button type="button" data-drawer-close aria-label="닫기" onClick={requestClose}><svg aria-hidden="true" viewBox="0 0 20 20" focusable="false"><path d="M4 4l12 12M16 4L4 16" /></svg></button>
      </header>
      {reportedError && <div className="toolbox-drawer-error" role="alert"><span>{reportedError}</span>{loadError && !error && <button type="button" onClick={onReload}>다시 불러오기</button>}</div>}
      {loading && <p className="toolbox-drawer-state">Toolbox를 불러오는 중입니다.</p>}
      <div className="toolbox-drawer-tabs" role="tablist" aria-label="Toolbox 종류">
        <button type="button" role="tab" aria-selected={tab === 'commands'} onClick={() => setTab('commands')}>명령어</button>
        <button type="button" role="tab" aria-selected={tab === 'todos'} onClick={() => setTab('todos')}>할 일</button>
      </div>

      {tab === 'commands' ? <section className="toolbox-drawer-section" role="tabpanel" aria-labelledby="global-toolbox-title">
        <div className="toolbox-drawer-section-heading"><div><h3>명령어</h3><p>실행하지 않고 필요한 명령만 복사합니다.</p></div><span>{commands.length}개</span></div>
        <div className="toolbox-drawer-list">
          {commands.length === 0 && <p className="muted">등록한 명령어가 없습니다.</p>}
          {commands.map((command: ToolboxCommand) => <article className="toolbox-drawer-command" key={command.id}>
            <div className="toolbox-drawer-row-heading"><div><strong>{command.label}</strong>{command.note && <small>{command.note}</small>}</div><div className="toolbox-drawer-actions"><button type="button" aria-label={`명령어 복사: ${command.label}`} onClick={() => void copyText(command.command).then(() => onStatus?.('명령어를 복사했습니다.')).catch(() => setError('명령어를 복사하지 못했습니다.'))}>복사</button><button type="button" aria-label={`명령어 삭제: ${command.label}`} className="danger-lite" disabled={blocked} onClick={() => void save({ ...committed, commands: commands.filter((item) => item.id !== command.id) }, '명령어를 삭제했습니다.')}>삭제</button></div></div>
            <pre><code>{command.command}</code></pre>
          </article>)}
        </div>
        <form className="toolbox-drawer-form" onSubmit={addCommand}>
          <input aria-label="명령어 이름" placeholder="명령어 이름" value={commandLabel} disabled={blocked} onChange={(event) => setCommandLabel(event.target.value)} />
          <input aria-label="명령어 메모" placeholder="메모 (선택)" value={commandNote} disabled={blocked} onChange={(event) => setCommandNote(event.target.value)} />
          <textarea aria-label="명령어 내용" rows={3} placeholder="herdr status" value={commandText} disabled={blocked} onChange={(event) => setCommandText(event.target.value)} />
          <button type="submit" disabled={blocked || !commandLabel.trim() || !commandText.trim()}>명령어 추가</button>
        </form>
      </section> : <section className="toolbox-drawer-section" role="tabpanel" aria-labelledby="global-toolbox-title">
        <div className="toolbox-drawer-section-heading"><div><h3>할 일</h3><p>공통 또는 한 프로젝트에 연결한 개인 메모입니다.</p></div><span>{visibleTodos.length}/{todos.length}개</span></div>
        <div className="toolbox-drawer-todo-controls">
          <label>범위<select aria-label="할 일 범위" value={filterToken(todoFilter)} onChange={(event) => setTodoFilter(parseFilter(event.target.value))}>{filterOptions}</select></label>
          <label className="toolbox-drawer-toggle"><input type="checkbox" aria-label="완료 항목 포함" checked={includeCompleted} onChange={(event) => setIncludeCompleted(event.target.checked)} /> 완료 포함</label>
        </div>
        <div className="toolbox-drawer-list">
          {visibleTodos.length === 0 && <p className="muted">표시할 할 일이 없습니다.</p>}
          {visibleTodos.map((todo) => {
            const unknown = todo.projectKey && !knownKeys.has(todo.projectKey)
            const choices = todo.projectKey && unknown ? [{ key: todo.projectKey, name: projectLabel(projectChoices, todo.projectKey) }, ...projectChoices] : projectChoices
            return <article className={`toolbox-drawer-todo${todo.done ? ' done' : ''}`} key={todo.id}>
              <label className="toolbox-drawer-todo-main"><input type="checkbox" aria-label={todo.text} checked={todo.done} disabled={blocked} onChange={(event) => void updateTodo(todo, { done: event.target.checked }, '할 일을 저장했습니다.')} /><span>{todo.text}</span></label>
              <small className="toolbox-drawer-todo-project">{todo.projectKey ? projectLabel(projectChoices, todo.projectKey) : '공통'}</small>
              <select aria-label={`${todo.text} 프로젝트`} value={todo.projectKey ? `${projectFilterPrefix}${todo.projectKey}` : commonFilter} disabled={blocked} onChange={(event) => {
                const value = event.target.value === commonFilter ? undefined : event.target.value.slice(projectFilterPrefix.length)
                void updateTodo(todo, value ? { projectKey: value } : { projectKey: undefined }, '할 일 범위를 저장했습니다.')
              }}>
                <option value={commonFilter}>공통</option>
                {choices.map((choice) => <option key={choice.key} value={`${projectFilterPrefix}${choice.key}`}>{choice.name}</option>)}
              </select>
              <button type="button" className="danger-lite" disabled={blocked} onClick={() => void save({ ...committed, todos: todos.filter((item) => item.id !== todo.id) }, '할 일을 삭제했습니다.')}>삭제</button>
            </article>
          })}
        </div>
        <form className="toolbox-drawer-form toolbox-drawer-todo-form" onSubmit={addTodo}>
          <input aria-label="할 일 내용" placeholder="할 일 추가" value={todoText} disabled={blocked} onChange={(event) => setTodoText(event.target.value)} />
          <select aria-label="새 할 일 프로젝트" value={todoProjectKey ? `${projectFilterPrefix}${todoProjectKey}` : commonFilter} disabled={blocked} onChange={(event) => setTodoProjectKey(event.target.value === commonFilter ? '' : event.target.value.slice(projectFilterPrefix.length))}>
            <option value={commonFilter}>공통</option>
            {projectChoices.map((choice) => <option key={choice.key} value={`${projectFilterPrefix}${choice.key}`}>{choice.name}</option>)}
          </select>
          <button type="submit" disabled={blocked || !todoText.trim()}>할 일 추가</button>
        </form>
      </section>}
    </section>
  </div>
}
