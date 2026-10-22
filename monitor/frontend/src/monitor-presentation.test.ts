import { describe, expect, it } from 'vitest'
import { connectionsForWork, herdrAgentDisplayName, herdrGuidance, labelFor } from './monitor-presentation'
import type { HerdrAgent, HerdrSnapshot, Project, WorkItem } from './types'

const work = (url: string): WorkItem => ({ workId: 'w1', title: 'Work', state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], github: { kind: 'issue', url, state: 'OPEN' }, links: [] })
const project = (links: Project['links']): Project => ({ projectId: 'repo:acme/app', name: 'Project', state: 'running', syncStatus: 'synced', nextAction: 'review', evidenceRefs: [], workItems: [work('https://github.com/acme/app/issues/1')], links })
const herdr = (connections: HerdrSnapshot['connections']): HerdrSnapshot => ({ source: 'herdr', schemaVersion: 1, revision: 1, observedAt: '2026-09-11T00:00:00Z', status: 'fresh', syncStatus: 'synced', freshness: { state: 'fresh', syncStatus: 'synced' }, notices: [], sessions: [], connections, unconnectedAgents: [] })
const connection = (session: string, extra: Partial<HerdrSnapshot['connections'][number]> = {}) => ({ session, repository: 'github.com/acme/app', role: 'coordinator', status: 'connected', nextAction: 'observe', handoff: session, ...extra })

describe('monitor presentation helpers', () => {
  it('chooses explicit name, workspace label, cwd basename, then unnamed fallback', () => {
    const displayName = (agent: HerdrAgent) => herdrAgentDisplayName(agent)
    expect(displayName({ name: 'Luna', workspace_label: 'JMJ T16', cwd: '/repo/src/' })).toBe('Luna')
    expect(displayName({ workspace_label: 'JMJ T16', cwd: '/repo/src/' })).toBe('JMJ T16')
    expect(displayName({ cwd: '/repo/src///', foreground_cwd: '/fallback' })).toBe('src')
    expect(displayName({ foreground_cwd: '/repo/tools/' })).toBe('tools')
    expect(displayName({ cwd: '/', foreground_cwd: '/' })).toBe('이름 없음')
    expect(displayName({})).toBe('이름 없음')
  })

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

  it('returns every distinct related Issue binding before project fallback', () => {
    const issueA = 'https://GHE.example:8443/Acme/App/issues/1/'
    const issueB = 'https://ghe.example:8443/acme/app/issues/2'
    const workItem: WorkItem = { ...work('https://ghe.example:8443/acme/app/pull/7'), github: { kind: 'pull_request', url: 'https://ghe.example:8443/acme/app/pull/7', relatedIssueUrls: [issueA, issueB, issueA] } }
    const shared = connection('shared', { issueUrl: issueA, repository: 'ghe.example:8443/acme/app', role: 'feature', workspaceId: 'w1', paneId: 'p1' })
    const duplicate = connection('shared', { issueUrl: issueA, repository: 'ghe.example:8443/acme/app', role: 'feature', workspaceId: 'w1', paneId: 'p1' })
    const distinctLocation = connection('shared', { issueUrl: issueA, repository: 'ghe.example:8443/acme/app', role: 'feature', workspaceId: 'w2', paneId: 'p2' })
    const second = connection('second', { issueUrl: issueB, repository: 'ghe.example:8443/acme/app', role: 'feature' })
    const projectBinding = connection('project', { projectUrl: 'https://ghe.example:8443/orgs/acme/projects/1', role: 'feature' })
    expect(connectionsForWork(workItem, project([{ kind: 'github', label: 'Project', url: 'https://ghe.example:8443/orgs/acme/projects/1' }]), herdr([shared, duplicate, distinctLocation, second, projectBinding])).map((item) => item.session + ':' + (item.paneId ?? '')).sort()).toEqual(['second:', 'shared:p1', 'shared:p2'])
  })

  it('keeps configured host port and rejects invalid locator schemes', () => {
    const pr = { ...work('https://ghe.example:8443/acme/app/pull/7'), github: { kind: 'pull_request' as const, url: 'https://ghe.example:8443/acme/app/pull/7', relatedIssueUrls: [] } }
    const central = connection('central', { repository: 'ghe.example:8443/acme/app' })
    const wrongHost = connection('wrong', { issueUrl: 'https://github.com/acme/app/issues/7', repository: 'ghe.example:8443/acme/app', role: 'feature' })
    expect(connectionsForWork(pr, undefined, herdr([wrongHost, central])).map((item) => item.session)).toEqual(['central'])
    const invalid = connection('invalid', { issueUrl: 'javascript:alert(1)', repository: 'ghe.example:8443/acme/app', role: 'feature' })
    expect(connectionsForWork({ ...pr, github: { ...pr.github, relatedIssueUrls: ['javascript:alert(1)'] } }, undefined, herdr([invalid, central])).map((item) => item.session)).toEqual(['central'])
    const invalidPath = connection('invalid-path', { issueUrl: 'https://ghe.example:8443/acme/app/not-an-issue/7', repository: 'ghe.example:8443/acme/app', role: 'feature' })
    expect(connectionsForWork({ ...pr, github: { ...pr.github, relatedIssueUrls: ['https://ghe.example:8443/acme/app/not-an-issue/7'] } }, undefined, herdr([invalidPath, central])).map((item) => item.session)).toEqual(['central'])
  })
})
