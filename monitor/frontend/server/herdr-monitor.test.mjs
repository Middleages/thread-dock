import test from 'node:test'
import assert from 'node:assert/strict'
import { createHerdrMonitor, parseSessionsFile, normalizeAgentList } from './herdr-monitor.mjs'

test('parses version one bindings and validates session names', () => {
  const parsed = parseSessionsFile(JSON.stringify({ version: 1, bindings: [{ session: 'feature-1', issueUrl: 'https://github.com/acme/app/issues/4', repository: 'github.com/acme/app', worktree: '/repo', paneId: 'p1' }] }))
  assert.equal(parsed.bindings[0].session, 'feature-1')
  assert.throws(() => parseSessionsFile(JSON.stringify({ version: 2, bindings: [] })), /version/)
  assert.throws(() => parseSessionsFile(JSON.stringify({ version: 1, bindings: [{ session: '.' }] })), /session/)
})

test('normalizes v0.8.2 agent list and strips forbidden fields', () => {
  const result = normalizeAgentList({ result: { type: 'agent_list', agents: [{ name: 'Luna', agent_status: 'working', workspace_id: 'w', tab_id: 't', pane_id: 'p', cwd: '/repo/src', foreground_cwd: '/repo/src', terminal_id: 'secret', agent_session: 'secret', tokens: 10, transcript: 'secret' }] } })
  assert.deepEqual(result, [{ name: 'Luna', agent_status: 'working', workspace_id: 'w', tab_id: 't', pane_id: 'p', cwd: '/repo/src', foreground_cwd: '/repo/src', }])
})

test('reads configured sessions, keeps same pane ids isolated by session, and connects canonical subdirectories', async () => {
  const calls = []
  const monitor = createHerdrMonitor({ env: { THREADDOCK_SESSIONS_FILE: '/tmp/sessions.json', HERDR_ENV: '1' }, readFile: async () => JSON.stringify({ version: 1, bindings: [
    { issueUrl: 'https://github.com/acme/app/issues/4', repository: 'github.com/acme/app', worktree: '/repo', session: 'feature-a', workspaceId: 'w1', tabId: 't1', paneId: 'same', agentName: 'A', role: 'feature' },
    { issueUrl: 'https://github.com/acme/app/issues/4', repository: 'github.com/acme/app', worktree: '/repo', session: 'feature-b', workspaceId: 'w2', tabId: 't2', paneId: 'same', agentName: 'B', role: 'review' },
  ] }), realpath: async (path) => path, run: async (args) => { calls.push(args); return JSON.stringify({ result: { type: 'agent_list', agents: [{ name: args[1] === 'feature-a' ? 'A' : 'B', agent_status: 'working', workspace_id: args[1] === 'feature-a' ? 'w1' : 'w2', tab_id: args[1] === 'feature-a' ? 't1' : 't2', pane_id: 'same', cwd: '/repo/src' }] } }) }, now: () => 1000 })
  const snapshot = await monitor.getSnapshot()
  assert.equal(calls.length, 2)
  assert.deepEqual(calls[0], ['--session', 'feature-a', 'agent', 'list'])
  assert.equal(snapshot.connections.filter((connection) => connection.status === 'connected').length, 2)
  assert.equal(new Set(snapshot.sessions.flatMap((session) => session.agents.map((agent) => `${session.session}:${agent.pane_id}`))).size, 2)
  assert.equal(snapshot.connections[0].location.cwd, '/repo/src')
  assert.equal(snapshot.connections[0].nextAction, 'observe')
})

test('guards Herdr outside a Herdr pane and reports unconnected agents without discovering sessions', async () => {
  const calls = []
  const monitor = createHerdrMonitor({ env: { THREADDOCK_SESSIONS_FILE: '/tmp/sessions.json', HERDR_ENV: '0' }, readFile: async () => JSON.stringify({ version: 1, bindings: [{ session: 'feature', repository: 'github.com/acme/app', worktree: '/repo', agentName: 'A' }] }), run: async (args) => { calls.push(args); return '{}' } })
  const snapshot = await monitor.getSnapshot()
  assert.equal(calls.length, 0)
  assert.equal(snapshot.status, 'offline')
  assert.match(snapshot.notices.join(' '), /HERDR_ENV/)
})

test('distinguishes source failure while hiding stderr and preserving recheck guidance', async () => {
  const monitor = createHerdrMonitor({ env: { THREADDOCK_SESSIONS_FILE: '/tmp/sessions.json', HERDR_ENV: '1' }, readFile: async () => JSON.stringify({ version: 1, bindings: [
    { issueUrl: 'https://github.com/acme/app/issues/1', repository: 'github.com/acme/app', worktree: '/repo', session: 'missing', paneId: 'p1' },
    { issueUrl: 'https://github.com/acme/app/issues/2', repository: 'github.com/acme/app', worktree: '/repo', session: 'present', paneId: 'p1' },
  ] }), realpath: async (path) => path, run: async () => { throw new Error('raw secret stderr') }, now: () => 5000 })
  const snapshot = await monitor.getSnapshot()
  assert.equal(snapshot.connections[0].status, 'offline')
  assert.equal(snapshot.connections[1].status, 'offline')
  assert.doesNotMatch(JSON.stringify(snapshot), /raw secret stderr/)
  assert.ok(snapshot.connections.every((connection) => connection.nextAction === 'recheck'))
})

test('rejects relative session file paths before reading them', async () => {
  let reads = 0
  const snapshot = await createHerdrMonitor({ env: { THREADDOCK_SESSIONS_FILE: '.threaddock/sessions.json', HERDR_ENV: '1' }, readFile: async () => { reads += 1; return '{}' } }).getSnapshot()
  assert.equal(reads, 0)
  assert.equal(snapshot.status, 'unverified')
  assert.match(snapshot.notices.join(' '), /절대|absolute/i)
})

test('does not wildcard-select an Agent for an underspecified binding', async () => {
  const snapshot = await createHerdrMonitor({ env: { THREADDOCK_SESSIONS_FILE: '/tmp/sessions.json', HERDR_ENV: '1' }, readFile: async () => JSON.stringify({ version: 1, bindings: [{ repository: 'github.com/acme/app', session: 'feature', worktree: '/repo' }] }), realpath: async (path) => path, run: async () => JSON.stringify({ result: { type: 'agent_list', agents: [{ name: 'first', workspace_id: 'w1', tab_id: 't1', pane_id: 'p1', cwd: '/repo' }, { name: 'second', workspace_id: 'w2', tab_id: 't2', pane_id: 'p2', cwd: '/repo' }] } }) }).getSnapshot()
  assert.equal(snapshot.connections[0].status, 'unverified')
  assert.equal(snapshot.connections[0].location.agentName, undefined)
  assert.equal(snapshot.unconnectedAgents.length, 2)
})

test('keeps a partially specified identity unverified even with one matching Agent', async () => {
  const snapshot = await createHerdrMonitor({ env: { THREADDOCK_SESSIONS_FILE: '/tmp/sessions.json', HERDR_ENV: '1' }, readFile: async () => JSON.stringify({ version: 1, bindings: [{ repository: 'github.com/acme/app', session: 'feature', paneId: 'p1' }] }), run: async () => JSON.stringify({ result: { type: 'agent_list', agents: [{ name: 'Luna', workspace_id: 'w1', tab_id: 't1', pane_id: 'p1', cwd: '/repo' }] } }) }).getSnapshot()
  assert.equal(snapshot.connections[0].status, 'unverified')
  assert.equal(snapshot.connections[0].location.agentName, undefined)
})

test('confirms a coordinator by complete identity without requiring a worktree', async () => {
  const snapshot = await createHerdrMonitor({ env: { THREADDOCK_SESSIONS_FILE: '/tmp/sessions.json', HERDR_ENV: '1' }, readFile: async () => JSON.stringify({ version: 1, bindings: [{ repository: 'github.com/acme/app', session: 'central', workspaceId: 'w1', tabId: 't1', paneId: 'p1', agentName: 'Sol', role: 'coordinator' }] }), run: async () => JSON.stringify({ result: { type: 'agent_list', agents: [{ name: 'Sol', agent_status: 'idle', workspace_id: 'w1', tab_id: 't1', pane_id: 'p1', cwd: '/other' }] } }), now: () => 1000 }).getSnapshot()
  assert.equal(snapshot.connections[0].status, 'connected')
})

test('keeps an empty successful target cached after a failed refresh', async () => {
  let now = 1000
  let fail = false
  const snapshotSource = createHerdrMonitor({ env: { THREADDOCK_SESSIONS_FILE: '/tmp/sessions.json', HERDR_ENV: '1' }, readFile: async () => JSON.stringify({ version: 1, bindings: [{ repository: 'github.com/acme/app', session: 'feature', workspaceId: 'w1', tabId: 't1', paneId: 'p1', agentName: 'Luna' }] }), run: async () => { if (fail) throw new Error('offline'); return JSON.stringify({ result: { type: 'agent_list', agents: [] } }) }, now: () => now })
  const first = await snapshotSource.getSnapshot()
  assert.equal(first.connections[0].status, 'missing')
  now = 7001; fail = true
  const second = await snapshotSource.getSnapshot()
  assert.equal(second.status, 'cached')
  assert.equal(second.connections[0].status, 'cached')
  assert.match(second.connections[0].handoff, /관찰 시각/)
})

test('copies state-specific guidance and observation time into handoffs', async () => {
  for (const [agentStatus, nextAction, guidance] of [['working', 'observe', '돌아가 관찰'], ['blocked', 'inspect_question', '질문'], ['idle', 'check_github', 'GitHub'], ['done', 'check_github', 'GitHub'], ['unknown', 'recheck', '재확인']]) {
    const snapshot = await createHerdrMonitor({ env: { THREADDOCK_SESSIONS_FILE: '/tmp/sessions.json', HERDR_ENV: '1' }, readFile: async () => JSON.stringify({ version: 1, bindings: [{ repository: 'github.com/acme/app', session: 'feature', workspaceId: 'w1', tabId: 't1', paneId: 'p1', agentName: 'Luna', worktree: '/repo' }] }), realpath: async (path) => path, run: async () => JSON.stringify({ result: { type: 'agent_list', agents: [{ name: 'Luna', agent_status: agentStatus, workspace_id: 'w1', tab_id: 't1', pane_id: 'p1', cwd: '/repo' }] } }), now: () => 1000 }).getSnapshot()
    assert.equal(snapshot.connections[0].nextAction, nextAction)
    assert.match(snapshot.connections[0].handoff, /1970-01-01T00:00:01.000Z/)
    assert.match(snapshot.connections[0].handoff, new RegExp(guidance))
  }
})
