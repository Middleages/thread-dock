import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
// The adapter is intentionally JavaScript so it can be exercised directly with node --test.
// @ts-expect-error no declaration is needed for Vite's runtime-only server adapter.
import { createGitHubMonitor, createGitHubMonitorMiddleware } from './server/github-monitor.mjs'

export default defineConfig({
  plugins: [react(), {
    name: 'threaddock-github-monitor',
    configureServer(server) {
      const monitor = createGitHubMonitor()
      server.middlewares.use(createGitHubMonitorMiddleware(monitor))
    },
  }],
  test: {
    environment: 'jsdom',
    setupFiles: './src/test-setup.ts',
    css: true,
    exclude: ['**/node_modules/**', 'server/**/*.test.mjs'],
  },
})
