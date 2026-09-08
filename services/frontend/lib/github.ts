/** Keeping a project in step with a GitHub repository. */

import { api } from './api'

export type GitHubStatus = {
  /** Whether this site offers GitHub sync at all. */
  enabled: boolean
  /** Whether this person has connected an account. */
  connected: boolean
  login?: string
}

export type GitHubRepo = {
  name: string
  fullName: string
  defaultBranchName: string
  private: boolean
}

export type SyncState = {
  repoFullName: string
  defaultBranchName?: string
  /** clean, conflict or diverged. */
  mergeStatus: string
  lastSyncCommit?: string
  lastSyncVersion?: number
  unmergedBranchName?: string
}

export type ProjectGitHub = {
  enabled: boolean
  linked: boolean
  state?: SyncState
  canPush?: boolean
}

export type GitHubCommit = {
  sha: string
  message: string
  author: { name: string; email: string; date: string }
}

export type MergeOverview = {
  mergeStatus: string
  repoFullName: string
  projectChanged: boolean
  unmergedBranch?: string
  repoRewritten: boolean
  commits: GitHubCommit[]
}

export function githubStatus(headers?: Record<string, string>): Promise<GitHubStatus> {
  return api<GitHubStatus>('/api/github/status', { headers })
}

export function unlinkGitHub(): Promise<void> {
  return api<void>('/api/github/link', { method: 'DELETE' })
}

export function listRepos(): Promise<GitHubRepo[]> {
  return api<{ repos: GitHubRepo[] }>('/api/github/repos').then(answer => answer.repos)
}

export function repoOwners(): Promise<{ login: string; orgs: string[] }> {
  return api<{ login: string; orgs: string[] }>('/api/github/owners')
}

export function importRepo(input: {
  name?: string
  fullName: string
  defaultBranchName: string
}): Promise<{ projectId: string }> {
  return api<{ projectId: string }>('/api/github/import', {
    method: 'POST',
    body: input,
  })
}

export function projectGitHub(
  projectId: string,
  headers?: Record<string, string>
): Promise<ProjectGitHub> {
  return api<ProjectGitHub>(`/api/projects/${projectId}/github`, { headers })
}

export function mergeOverview(projectId: string): Promise<MergeOverview> {
  return api<MergeOverview>(`/api/projects/${projectId}/github/overview`)
}

export function syncProject(
  projectId: string,
  input: { message?: string; resolved?: boolean } = {}
): Promise<{ mergeStatus: string; repoFullName: string; unmergedBranchName?: string }> {
  return api(`/api/projects/${projectId}/github/sync`, {
    method: 'POST',
    body: input,
  })
}

export function exportProject(
  projectId: string,
  input: { name?: string; description?: string; isPublic?: boolean; org?: string }
): Promise<{ state: SyncState }> {
  return api<{ state: SyncState }>(`/api/projects/${projectId}/github/export`, {
    method: 'POST',
    body: input,
  })
}

export function unlinkRepo(projectId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/github`, { method: 'DELETE' })
}
