/** What the API says about projects. */

import { api } from './api'

export type Access = 'owner' | 'readAndWrite' | 'review' | 'readOnly' | ''

export type ProjectSummary = {
  id: string
  name: string
  ownerId: string
  access: Access
  lastUpdated: string
  lastUpdatedBy?: string
  archived: boolean
  trashed: boolean
}

export function listProjects(headers?: Record<string, string>): Promise<ProjectSummary[]> {
  return api<{ projects: ProjectSummary[] }>('/api/projects', { headers }).then(
    answer => answer.projects
  )
}

export function createProject(name: string): Promise<ProjectSummary> {
  return api<{ project: ProjectSummary }>('/api/projects', {
    method: 'POST',
    body: { name },
  }).then(answer => answer.project)
}

export function renameProject(id: string, name: string): Promise<void> {
  return api<void>(`/api/projects/${id}/rename`, { method: 'POST', body: { name } })
}

export function setArchived(id: string, on: boolean): Promise<void> {
  return api<void>(`/api/projects/${id}/archive`, { method: 'POST', body: { on } })
}

export function setTrashed(id: string, on: boolean): Promise<void> {
  return api<void>(`/api/projects/${id}/trash`, { method: 'POST', body: { on } })
}

export function deleteProject(id: string): Promise<void> {
  return api<void>(`/api/projects/${id}`, { method: 'DELETE' })
}

/** Whether this access lets somebody change the project. */
export function canWrite(access: Access): boolean {
  return access === 'owner' || access === 'readAndWrite'
}
