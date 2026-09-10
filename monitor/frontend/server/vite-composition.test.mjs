import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { resolve } from 'node:path'

test('keeps Vite document and module routes available alongside the GitHub API', async () => {
  const server = await createServer({ root: resolve('.'), configFile: resolve('vite.config.ts'), server: { host: '127.0.0.1', port: 0 } })
  await server.listen()
  const address = server.httpServer.address()
  const base = `http://127.0.0.1:${address.port}`
  try {
    assert.equal((await fetch(`${base}/`)).status, 200)
    assert.equal((await fetch(`${base}/src/main.tsx`)).status, 200)
    assert.equal((await fetch(`${base}/api/github-monitor`)).status, 200)
  } finally {
    server.httpServer.closeAllConnections?.()
    await new Promise((resolveClose) => server.httpServer.close(resolveClose))
    await server.watcher?.close()
  }
})
