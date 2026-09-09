/** A project's past. */

import { api } from './api'

/** One sitting of work, as the history service groups it. */
export type Update = {
  fromV: number
  toV: number
  meta: {
    users?: Array<{ id?: string; email?: string; first_name?: string; last_name?: string }>
    start_ts?: number
    end_ts?: number
  }
  labels?: Label[]
  pathnames?: string[]
  project_ops?: Array<Record<string, unknown>>
}

export type Label = {
  id: string
  comment: string
  version: number
  user_id?: string
  created_at?: string
}

export function listUpdates(
  projectId: string,
  before?: number
): Promise<{ updates: Update[]; nextBefore: number }> {
  const query = before ? `?before=${before}` : ''
  return api<{ updates: Update[]; nextBefore: number }>(
    `/api/projects/${projectId}/history/updates${query}`
  )
}

/** One run of text, and whether it was added, removed, or left alone. */
export type DiffChunk = {
  u?: string
  i?: string
  d?: string
  meta?: { users?: unknown[]; start_ts?: number; end_ts?: number }
}

export function getDiff(
  projectId: string,
  path: string,
  from: number,
  to: number
): Promise<{ diff: DiffChunk[] }> {
  const query = new URLSearchParams({
    path,
    from: String(from),
    to: String(to),
  })
  return api<{ diff: DiffChunk[] }>(
    `/api/projects/${projectId}/history/diff?${query}`
  )
}

/** Which files changed between two versions. */
export type FileChange = {
  pathname: string
  operation?: 'added' | 'removed' | 'edited' | 'renamed'
  newPathname?: string
}

export function getChanges(
  projectId: string,
  from: number,
  to: number
): Promise<{ diff: FileChange[] }> {
  const query = new URLSearchParams({ from: String(from), to: String(to) })
  return api<{ diff: FileChange[] }>(
    `/api/projects/${projectId}/history/changes?${query}`
  )
}

export function listLabels(projectId: string): Promise<{ labels: Label[] }> {
  return api<{ labels: Label[] }>(`/api/projects/${projectId}/history/labels`)
}

export function createLabel(
  projectId: string,
  comment: string,
  version: number
): Promise<void> {
  return api<void>(`/api/projects/${projectId}/history/labels`, {
    method: 'POST',
    body: { comment, version },
  })
}

/** Puts a file back as it was, as a new edit rather than by rewinding. */
export function restoreFile(
  projectId: string,
  path: string,
  version: number
): Promise<void> {
  return api<void>(`/api/projects/${projectId}/history/restore`, {
    method: 'POST',
    body: { path, version },
  })
}

/** Who made a change, as a name to show. */
export function authorsOf(update: Update): string {
  const users = update.meta.users ?? []
  const names = users
    .map(user =>
      [user.first_name, user.last_name].filter(Boolean).join(' ') || user.email
    )
    .filter(Boolean)
  if (names.length === 0) {
    return 'Unknown'
  }
  return [...new Set(names)].join(', ')
}
