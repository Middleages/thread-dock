import { describe, expect, it } from 'vitest'
import { toolboxKeyFor } from './project-key'
import type { Project } from './types'

const project = (projectId: string, links: Project['links'] = []): Project => ({
  projectId,
  name: projectId,
  state: 'running',
  syncStatus: 'synced',
  nextAction: 'review',
  evidenceRefs: [],
  workItems: [],
  links,
})

describe('toolboxKeyFor', () => {
  it('uses a valid GitHub hostname with the project id', () => {
    expect(toolboxKeyFor(project('repo:acme/app', [{ kind: 'github', label: 'Repository', url: 'https://github.com/repo/acme/app' }]))).toBe('github.com/repo:acme/app')
  })

  it('preserves a GitHub Enterprise hostname', () => {
    expect(toolboxKeyFor(project('repo:FDYPhotoDX/jmj', [{ kind: 'github', label: 'Repository', url: 'https://github.samsungds.net/repo/FDYPhotoDX/jmj' }]))).toBe('github.samsungds.net/repo:FDYPhotoDX/jmj')
  })

  it('falls back to the project id without a valid URL', () => {
    expect(toolboxKeyFor(project('repo:acme/app'))).toBe('repo:acme/app')
  })
})
