/** Who else may open a project. */

import { api } from './api'

export type Privilege = 'readAndWrite' | 'readOnly'

export type Member = {
  id: string
  email: string
  name: string
  privilege: Privilege
  owner: boolean
}

export type Invite = {
  id: string
  email: string
  privilege: Privilege
  createdAt: string
}

export type Sharing = {
  members: Member[]
  invites: Invite[]
  /** 'private' or 'tokenBased'. */
  publicAccess: string
}

export function getSharing(
  projectId: string,
  headers?: Record<string, string>
): Promise<Sharing> {
  return api<Sharing>(`/api/projects/${projectId}/sharing`, { headers })
}

/**
 * Offers somebody access.
 *
 * Answers with a member when the address already has an account, and with an
 * invitation and its link when it does not. The link comes back exactly once:
 * it is the whole credential, so there is no endpoint that reads it again.
 */
export function invite(
  projectId: string,
  email: string,
  privilege: Privilege
): Promise<{ member?: Member; invite?: Invite; link?: string }> {
  return api(`/api/projects/${projectId}/sharing/invites`, {
    method: 'POST',
    body: { email, privilege },
  })
}

export function revokeInvite(projectId: string, inviteId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/sharing/invites/${inviteId}`, {
    method: 'DELETE',
  })
}

export function setPrivilege(
  projectId: string,
  userId: string,
  privilege: Privilege
): Promise<void> {
  return api<void>(`/api/projects/${projectId}/sharing/members/${userId}`, {
    method: 'POST',
    body: { privilege },
  })
}

export function removeMember(projectId: string, userId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/sharing/members/${userId}`, {
    method: 'DELETE',
  })
}

export function setPublicAccess(
  projectId: string,
  level: 'private' | 'tokenBased'
): Promise<void> {
  return api<void>(`/api/projects/${projectId}/sharing/public`, {
    method: 'POST',
    body: { level },
  })
}

export function acceptInvite(token: string): Promise<{ projectId: string }> {
  return api<{ projectId: string }>(`/api/invites/${token}/accept`, {
    method: 'POST',
  })
}
