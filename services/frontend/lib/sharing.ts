/** Who else may open a project. */

import { api } from './api'

export type Privilege = 'readAndWrite' | 'readOnly' | 'review'

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
  expiresAt?: string
}

export type PublicAccessLevel = 'private' | 'tokenBased' | 'readAndWrite' | 'readOnly'

export type Sharing = {
  members: Member[]
  invites: Invite[]
  publicAccess: PublicAccessLevel
}

export function getSharing(projectId: string, headers?: Record<string, string>): Promise<Sharing> {
  return api<Sharing>(`/api/projects/${projectId}/sharing`, { headers })
}

export type InviteAnswer = { member?: Member; invite?: Invite; link?: string; sent?: boolean; error?: string }

/**
 * Offers somebody access.
 *
 * Answers with a member when the address already has an account, and with an
 * invitation when it does not.
 */
export function invite(projectId: string, email: string, privilege: Privilege): Promise<InviteAnswer> {
  return api<InviteAnswer>(`/api/projects/${projectId}/sharing/invites`, {
    method: 'POST',
    body: { email, privilege },
  })
}

/** Sends the invitation email again. */
export function resendInvite(projectId: string, inviteId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/sharing/invites/${inviteId}/resend`, { method: 'POST' })
}

export function revokeInvite(projectId: string, inviteId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/sharing/invites/${inviteId}`, { method: 'DELETE' })
}

export function setPrivilege(projectId: string, userId: string, privilege: Privilege): Promise<void> {
  return api<void>(`/api/projects/${projectId}/sharing/members/${userId}`, {
    method: 'POST',
    body: { privilege },
  })
}

export function removeMember(projectId: string, userId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/sharing/members/${userId}`, { method: 'DELETE' })
}

export function transferOwnership(projectId: string, userId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/sharing/owner`, {
    method: 'POST',
    body: { userId },
  })
}

export function setPublicAccess(projectId: string, level: 'private' | 'tokenBased'): Promise<void> {
  return api<void>(`/api/projects/${projectId}/sharing/public`, {
    method: 'POST',
    body: { level },
  })
}

/** The link-sharing tokens: one that lets an editor in, one for a viewer. */
export type Tokens = {
  readAndWrite: string
  readAndWriteHashPrefix: string
  readAndWritePrefix: string
  readOnly: string
  readOnlyHashPrefix: string
}

export function getTokens(projectId: string, signal?: AbortSignal): Promise<Tokens> {
  return api<Tokens>(`/api/projects/${projectId}/tokens`, { signal })
}

export function acceptInvite(token: string): Promise<{ projectId: string }> {
  return api<{ projectId: string }>(`/api/invites/${token}/accept`, { method: 'POST' })
}

/**
 * Following a sharing link.
 *
 * A POST rather than something a page does while rendering: this changes who
 * can see a project, and a GET that does that is a GET a link preview or a
 * mail scanner can fire on somebody's behalf.
 */
export function redeemLink(
  token: string
): Promise<{ projectId: string; privilege: string }> {
  return api<{ projectId: string; privilege: string }>(
    `/api/sharing/links/${encodeURIComponent(token)}`,
    { method: 'POST' }
  )
}
