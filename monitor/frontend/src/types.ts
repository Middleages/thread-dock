export type Freshness = {
  state: string
  syncStatus: string
  observedAt?: string
  lastSyncedAt?: string
}

export type Link = { kind: string; label: string; url: string }
export type TaskDetail = {
  taskId: string
  repoKey: string
  state: string
  verification?: string
  review?: string
  merge?: string
}
export type PublicationDetail = {
  intentId: string
  key: string
  generation: number
  kind: string
  status: string
  attempts: number
  url?: string
}
export type DecisionDetail = { decisionId: string; summary: string; status: string; observedAt?: string }
export type HandoffDetail = { summary: string; nextAction: string; evidenceRefs: string[]; observedAt?: string }
export type WorkItem = {
  workId: string
  title: string
  request?: string
  state: string
  syncStatus: string
  nextAction: string
  evidenceRefs: string[]
  updatedAt?: string
  tasks: TaskDetail[]
  blocker?: string
  publications: PublicationDetail[]
  decisions?: DecisionDetail[]
  handoffs?: HandoffDetail[]
  links?: Link[]
}
export type Project = {
  projectId: string
  name: string
  state: string
  syncStatus: string
  nextAction: string
  evidenceRefs: string[]
  updatedAt?: string
  workItems: WorkItem[]
}
export type Snapshot = {
  schemaVersion: number
  revision: number
  observedAt: string
  freshness: Freshness
  state: string
  syncStatus: string
  nextAction: string
  evidenceRefs: string[]
  projects: Project[]
}

export type SnapshotSource = () => Promise<Snapshot>
