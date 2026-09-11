import type { WorkItem } from './types'
import './work-table.css'

const kindLabel = (item: WorkItem) => item.github?.kind === 'pull_request' ? 'PR' : item.github?.kind === 'issue' ? 'Issue' : 'Project'
const stateLabel = (item: WorkItem) => {
  const value = (item.github?.state || item.state || 'unknown').toLowerCase()
  if (value === 'open') return '열림'
  if (value === 'closed') return '닫힘'
  if (value === 'merged') return '병합됨'
  return value.replaceAll('_', ' ')
}
const updatedLabel = (value?: string) => value
  ? new Intl.DateTimeFormat('ko-KR', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value))
  : '—'

export function WorkTable({ items, selected, onSelect }: { items: WorkItem[]; selected: string | null; onSelect: (id: string) => void }) {
  if (items.length <= 1) return null
  return <div className="work-table" role="table" aria-label="Issue 및 PR 목록">
    <div className="work-table-head" role="row">
      <span role="columnheader">번호</span>
      <span role="columnheader">종류</span>
      <span role="columnheader">제목</span>
      <span role="columnheader">상태</span>
      <span role="columnheader">수정시각</span>
    </div>
    {items.map((item) => {
      const legacyRole = item.github ? undefined : 'tab'
      return <button
        type="button"
        role={legacyRole}
        key={item.workId}
        className={`work-table-row${selected === item.workId ? ' selected' : ''}`}
        aria-pressed={item.github ? selected === item.workId : undefined}
        aria-selected={!item.github ? selected === item.workId : undefined}
        onClick={() => onSelect(item.workId)}
      >
        <span className="work-number">{item.github?.number ? `#${item.github.number}` : '—'}</span>
        <span className="work-kind">{kindLabel(item)}</span>
        <span className="work-title" title={item.title}>{item.title}</span>
        <span className="work-state">{stateLabel(item)}</span>
        <span className="work-updated">{updatedLabel(item.updatedAt)}</span>
      </button>
    })}
  </div>
}
