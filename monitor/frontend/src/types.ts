export type Freshness = {
  state: string
  syncStatus: string
  observedAt?: string
  lastSyncedAt?: string
}

export type Link = { kind: string; label: string; url: string }
export type GithubCheck = { name: string; status?: string; conclusion?: string; url?: string }
export type GithubWork = {
  kind: 'issue' | 'pull_request' | 'project_item' | string
  number?: number
  url?: string
  state?: string
  reviewDecision?: string
  checks?: GithubCheck[]
  relatedIssueUrls?: string[]
  relatedPullRequestUrls?: string[]
  fields?: Record<string, string>
  contentAvailable?: boolean
  author?: string
  observedAt?: string
  stale?: boolean
}
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
  tasks?: TaskDetail[]
  blocker?: string
  publications?: PublicationDetail[]
  decisions?: DecisionDetail[]
  handoffs?: HandoffDetail[]
  links?: Link[]
  github?: GithubWork
}
export type Project = {
  projectId: string
  name: string
  state: string
  syncStatus: string
  nextAction: string
  evidenceRefs: string[]
  updatedAt?: string
  observedAt?: string
  workItems: WorkItem[]
  source?: 'github' | string
  notices?: string[]
  links?: Link[]
}
export type HerdrAgent = {
  name?: string
  agent_status?: string
  workspace_id?: string
  tab_id?: string
  pane_id?: string
  cwd?: string
  foreground_cwd?: string
}
export type HerdrSession = {
  session: string
  status: string
  observedAt?: string
  agents: HerdrAgent[]
  links?: Link[]
}
export type HerdrConnection = {
  issueUrl?: string
  projectUrl?: string
  repository?: string
  worktree?: string
  session: string
  workspaceId?: string
  tabId?: string
  paneId?: string
  agentName?: string
  role?: string
  status: string
  agentStatus?: string
  observedAt?: string
  nextAction: string
  handoff: string
  location?: { session?: string; workspaceId?: string; tabId?: string; paneId?: string; agentName?: string; cwd?: string }
}
export type HerdrSnapshot = {
  source: 'herdr' | string
  schemaVersion: number
  revision: number
  observedAt: string
  status: string
  state?: string
  syncStatus: string
  nextAction?: string
  freshness: Freshness
  notices: string[]
  sessions: HerdrSession[]
  connections: HerdrConnection[]
  unconnectedAgents: Array<HerdrAgent & { session: string }>
  links?: Link[]
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
  source?: 'github' | string
  notices?: string[]
  herdr?: HerdrSnapshot
}

export type SnapshotSource = () => Promise<Snapshot>
