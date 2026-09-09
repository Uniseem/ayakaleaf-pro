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

/** A label somebody has put on some of their projects. */
export type Tag = {
  id: string
  name: string
  color?: string
  projectIds: string[]
}

export function listTags(headers?: Record<string, string>): Promise<Tag[]> {
  return api<{ tags: Tag[] }>('/api/tags', { headers }).then(answer => answer.tags)
}

export function createTag(name: string, color?: string): Promise<Tag> {
  return api<{ tag: Tag }>('/api/tags', {
    method: 'POST',
    body: { name, color },
  }).then(answer => answer.tag)
}

export function renameTag(id: string, name: string, color?: string): Promise<void> {
  return api<void>(`/api/tags/${id}`, { method: 'POST', body: { name, color } })
}

export function deleteTag(id: string): Promise<void> {
  return api<void>(`/api/tags/${id}`, { method: 'DELETE' })
}

export function tagProject(tagId: string, projectId: string): Promise<void> {
  return api<void>(`/api/tags/${tagId}/projects/${projectId}`, { method: 'POST' })
}

export function untagProject(tagId: string, projectId: string): Promise<void> {
  return api<void>(`/api/tags/${tagId}/projects/${projectId}`, { method: 'DELETE' })
}

/** Copies a project, contents and all. */
export function cloneProject(id: string, name: string): Promise<ProjectSummary> {
  return api<{ project: ProjectSummary }>(`/api/projects/${id}/clone`, {
    method: 'POST',
    body: { name },
  }).then(answer => answer.project)
}

/** Which of the lists on the projects page a project belongs in. */
export type Filter = 'all' | 'owned' | 'shared' | 'archived' | 'trashed'

export function matchesFilter(
  project: ProjectSummary,
  filter: Filter,
  userId: string
): boolean {
  // Archived and trashed are places, not attributes: a project in either is
  // out of the other lists entirely, which is the whole point of putting it
  // there.
  if (filter === 'archived') {
    return project.archived
  }
  if (filter === 'trashed') {
    return project.trashed
  }
  if (project.archived || project.trashed) {
    return false
  }
  switch (filter) {
    case 'owned':
      return project.ownerId === userId
    case 'shared':
      return project.ownerId !== userId
    default:
      return true
  }
}

export type SortKey = 'lastUpdated' | 'name' | 'owner'

export function compareProjects(
  a: ProjectSummary,
  b: ProjectSummary,
  key: SortKey,
  ascending: boolean
): number {
  let result: number
  switch (key) {
    case 'name':
      result = a.name.localeCompare(b.name, undefined, { numeric: true })
      break
    case 'owner':
      result = (a.lastUpdatedBy ?? '').localeCompare(b.lastUpdatedBy ?? '')
      break
    default:
      result = Date.parse(a.lastUpdated) - Date.parse(b.lastUpdated)
  }
  return ascending ? result : -result
}
