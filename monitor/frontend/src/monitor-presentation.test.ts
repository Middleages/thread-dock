import { describe, expect, it } from 'vitest'
import { connectionsForWork, herdrGuidance, labelFor } from './monitor-presentation'
import type { HerdrSnapshot, Project, WorkItem } from './types'

const work = (url: string): WorkItem => ({ workId: 'w1', title: 'Work', state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], github: { kind: 'issue', url, state: 'OPEN' }, links: [] })
const project = (links: Project['links']): Project => ({ projectId: 'repo:acme/app', name: 'Project', state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], workItems: [work('https://github.com/acme/app/issues/1')], links })
const herdr = (connections: HerdrSnapshot['connections']): HerdrSnapshot => ({ source: 'herdr', schemaVersion: 1, revision: 1, observedAt: '2026-09-11T00:00:00Z', status: 'fresh', syncStatus: 'synced', freshness: { state: 'fresh', syncStatus: 'synced' }, notices: [], sessions: [], connections, unconnectedAgents: [] })
const connection = (session: string, extra: Partial<HerdrSnapshot['connections'][number]> = {}) => ({ session, repository: 'github.com/acme/app', role: 'coordinator', status: 'connected', nextAction: 'observe', handoff: session, ...extra })

describe('monitor presentation helpers', () => {
  it('keeps state labels and guidance human-readable', () => {
    expect(labelFor('needs_operator')).toBe('판단 필요')
    expect(herdrGuidance(connection('x', { agentStatus: 'working' }))).toBe('해당 세션에서 작업 중입니다.')
  })

  it('uses exclusive issue, project, then repository matching precedence', () => {
    const issueURL = 'https://github.com/acme/app/issues/1'
    const boardURL = 'https://github.com/orgs/acme/projects/7'
    const issue = connection('issue', { issueUrl: issueURL, role: 'feature' })
    const board = connection('project', { projectUrl: boardURL, role: 'feature' })
    const central = connection('central')
    expect(connectionsForWork(work(issueURL), project([{ kind: 'github', label: 'Project', url: boardURL }]), herdr([issue, board, central])).map((item) => item.session)).toEqual(['issue'])
    expect(connectionsForWork(work('https://github.com/acme/app/issues/2'), project([{ kind: 'github', label: 'Project', url: boardURL }]), herdr([board, central])).map((item) => item.session)).toEqual(['project'])
    expect(connectionsForWork(work('https://github.com/acme/app/issues/2'), project([{ kind: 'evidence', label: 'Project', url: boardURL }]), herdr([central])).map((item) => item.session)).toEqual(['central'])
  })
})
