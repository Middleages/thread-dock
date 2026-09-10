import test from 'node:test'
import assert from 'node:assert/strict'
import http from 'node:http'
import { createGitHubMonitor, createGitHubMonitorMiddleware, parseMonitorConfig } from './github-monitor.mjs'

const issue = { number: 1, title: 'Fix retries', url: 'https://github.com/acme/app/issues/1', state: 'OPEN', body: 'Retry only once', updatedAt: '2026-09-09T10:00:00Z' }
const linkedPr = { number: 2, title: 'Fix retries implementation', url: 'https://github.com/acme/app/pull/2', state: 'OPEN', body: 'Implementation', updatedAt: '2026-09-09T11:00:00Z', reviewDecision: 'REVIEW_REQUIRED', statusCheckRollup: [{ conclusion: 'SUCCESS', status: 'COMPLETED', name: 'build' }], closingIssuesReferences: [{ url: issue.url }] }
const unlinkedPr = { number: 3, title: 'Docs', url: 'https://github.com/acme/app/pull/3', state: 'CLOSED', body: 'Docs', updatedAt: '2026-09-09T09:00:00Z', reviewDecision: 'APPROVED', statusCheckRollup: [], closingIssuesReferences: [] }

function runnerFor(fixtures, calls = []) {
  return async (args) => {
    calls.push(args)
    const joined = args.join(' ')
    if (joined.includes('issue list')) return JSON.stringify(fixtures.issues ?? [])
    if (joined.includes('pr list')) return JSON.stringify(fixtures.prs ?? [])
    if (joined.includes('project item-list')) return JSON.stringify(fixtures.project ?? { items: [], totalCount: 0 })
    throw new Error(`unexpected command ${joined}`)
  }
}

test('validates repository and project configuration with setup guidance', () => {
  assert.deepEqual(parseMonitorConfig({ THREADDOCK_REPOS: 'acme/app,octo/docs', THREADDOCK_PROJECTS: 'https://github.com/users/acme/projects/2' }), {
    repos: [{ owner: 'acme', repo: 'app' }, { owner: 'octo', repo: 'docs' }],
    projects: [{ owner: 'acme', number: 2, url: 'https://github.com/users/acme/projects/2' }],
  })
  assert.throws(() => parseMonitorConfig({ THREADDOCK_REPOS: 'not-a-repo' }), /THREADDOCK_REPOS/)
  assert.throws(() => parseMonitorConfig({ THREADDOCK_REPOS: 'acme/app', THREADDOCK_PROJECTS: 'https://github.com/acme/projects/2' }), /THREADDOCK_PROJECTS/)
})

test('maps GitHub issues, pull requests, links, and arbitrary project fields', async () => {
  const calls = []
  const monitor = createGitHubMonitor({
    env: { THREADDOCK_REPOS: 'acme/app', THREADDOCK_PROJECTS: 'https://github.com/users/acme/projects/2' },
    run: runnerFor({ issues: [issue], prs: [linkedPr, unlinkedPr], project: { totalCount: 1, items: [{ content: { number: 1, title: 'Fix retries', body: 'Board copy', url: issue.url, state: 'OPEN' }, fieldValues: [{ field: { name: 'Status' }, name: 'In progress' }, { field: { name: 'Priority' }, name: 'P1' }, { field: { name: 'Custom lane' }, value: 'Later' }] }] } }, calls),
    now: () => 1736500000000,
  })
  const snapshot = await monitor.getSnapshot()
  const repoProject = snapshot.projects.find((item) => item.projectId === 'repo:acme/app')
  assert.ok(repoProject)
  assert.deepEqual(repoProject.links.map((link) => link.label), ['Issues', 'PRs', 'Wiki'])
  assert.equal(repoProject.workItems.length, 3)
  const pr = repoProject.workItems.find((work) => work.github?.url === linkedPr.url)
  assert.equal(pr.github.kind, 'pull_request')
  assert.deepEqual(pr.github.relatedIssueUrls, [issue.url])
  assert.equal(pr.github.reviewDecision, 'REVIEW_REQUIRED')
  assert.equal(pr.github.checks[0].name, 'build')
  const board = snapshot.projects.find((item) => item.projectId === 'board:acme/2')
  assert.ok(board)
  assert.equal(board.workItems[0].github.fields['Status'], 'In progress')
  assert.equal(board.workItems[0].github.fields['Custom lane'], 'Later')
  assert.equal(calls.filter((args) => args.join(' ').includes('issue list')).length, 1)
})

test('deduplicates requests and keeps last-good source data when GitHub fails', async () => {
  let now = 1000
  let failing = false
  let calls = 0
  const monitor = createGitHubMonitor({
    env: { THREADDOCK_REPOS: 'acme/app' },
    now: () => now,
    run: async (args) => { calls += 1; if (failing) throw new Error('network details must stay private'); return args.join(' ').includes('issue list') ? JSON.stringify([issue]) : JSON.stringify([]) },
  })
  const [first, second] = await Promise.all([monitor.getSnapshot(), monitor.getSnapshot()])
  assert.deepEqual(first.projects, second.projects)
  assert.equal(calls, 2)
  now = 62001
  failing = true
  const stale = await monitor.getSnapshot()
  assert.equal(stale.freshness.state, 'stale')
  assert.match(stale.notices.join(' '), /acme\/app/)
  assert.doesNotMatch(stale.notices.join(' '), /network details/)
  assert.equal(stale.projects.some((item) => item.projectId === 'repo:acme/app'), true)
})

test('serves GET JSON and rejects browser mutation requests', async () => {
  const server = http.createServer(createGitHubMonitorMiddleware({ getSnapshot: async () => ({ source: 'github', notices: [], projects: [] }) }))
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const { port } = server.address()
  const response = await fetch(`http://127.0.0.1:${port}/api/github-monitor`)
  assert.equal(response.status, 200)
  assert.equal((await response.json()).source, 'github')
  const post = await fetch(`http://127.0.0.1:${port}/api/github-monitor`, { method: 'POST' })
  assert.equal(post.status, 405)
  await new Promise((resolve) => server.close(resolve))
})

test('keeps the loopback endpoint private and does not expose adapter errors', async () => {
  const server = http.createServer(createGitHubMonitorMiddleware({ getSnapshot: async () => { throw new Error('secret stderr token') } }))
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const { port } = server.address()
  const foreignOrigin = await fetch(`http://127.0.0.1:${port}/api/github-monitor`, { headers: { Origin: 'https://evil.example' } })
  assert.equal(foreignOrigin.status, 403)
  const foreignHost = await new Promise((resolve, reject) => { const request = http.get({ hostname: '127.0.0.1', port, path: '/api/github-monitor', headers: { Host: 'evil.example' } }, resolve); request.on('error', reject) })
  assert.equal(foreignHost.statusCode, 403)
  const failed = await fetch(`http://127.0.0.1:${port}/api/github-monitor`)
  assert.equal(failed.status, 500)
  assert.doesNotMatch(await failed.text(), /secret stderr token/)
  await new Promise((resolve) => server.close(resolve))
})

test('preserves inaccessible Project items and renders flattened custom fields without guessing state', async () => {
  const monitor = createGitHubMonitor({
    env: { THREADDOCK_PROJECTS: 'https://github.com/orgs/acme/projects/7' },
    run: async () => JSON.stringify({ totalCount: 1, items: [{ id: 'draft-1', content: null, status: 'Waiting', priority: { name: 'P2' } }] }),
  })
  const snapshot = await monitor.getSnapshot()
  const item = snapshot.projects[0].workItems[0]
  assert.equal(item.state, 'unknown')
  assert.equal(item.github.contentAvailable, false)
  assert.equal(item.github.fields.status, 'Waiting')
  assert.equal(item.github.fields.priority, 'P2')
  assert.match(snapshot.notices.join(' '), /내용을 확인할 수 없습니다/)
})

test('does not replace a valid cache with malformed JSON shape or retry outages every UI poll', async () => {
  let now = 0
  let calls = 0
  const monitor = createGitHubMonitor({
    env: { THREADDOCK_REPOS: 'acme/app' },
    now: () => now,
    run: async (args) => { calls += 1; if (now === 0) return args.join(' ').includes('issue list') ? JSON.stringify([issue]) : JSON.stringify([]); return JSON.stringify({ unexpected: true }) },
  })
  await monitor.getSnapshot()
  now = 60_001
  const stale = await monitor.getSnapshot()
  assert.equal(stale.projects[0].workItems.length, 1)
  assert.equal(stale.freshness.state, 'stale')
  const afterPoll = await monitor.getSnapshot()
  assert.equal(afterPoll.projects[0].workItems.length, 1)
  assert.equal(calls, 4)
})
