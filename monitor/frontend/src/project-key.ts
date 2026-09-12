import type { Project } from './types'
import { isSafeExternalURL } from './safe-url'

export function toolboxKeyFor(project: Project): string {
  const projectURL = project.links?.find((link) => isSafeExternalURL(link.url))?.url
  const workURL = project.workItems.find((work) => isSafeExternalURL(work.github?.url ?? ''))?.github?.url
  const candidate = projectURL ?? workURL
  if (candidate) {
    try { return `${new URL(candidate).hostname}/${project.projectId}` } catch { /* use project id */ }
  }
  return project.projectId
}
