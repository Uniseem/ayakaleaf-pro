/** Personal access tokens: what git asks for instead of a password. */

import { api } from './api'

export type AccessToken = {
  id: string
  /** The first few characters, which is all that is kept. */
  partial: string
  scope: string
  createdAt: string
  expiresAt: string
  lastUsedAt?: string
}

export function listTokens(headers?: Record<string, string>): Promise<AccessToken[]> {
  return api<{ tokens: AccessToken[] }>('/api/tokens', { headers }).then(
    answer => answer.tokens
  )
}

/**
 * Makes a token.
 *
 * The secret comes back once and is never stored, so whatever calls this has
 * to show it immediately: there is no second chance to read it.
 */
export function createToken(): Promise<{ token: AccessToken; accessToken: string }> {
  return api<{ token: AccessToken; accessToken: string }>('/api/tokens', {
    method: 'POST',
    body: {},
  })
}

export function revokeToken(id: string): Promise<void> {
  return api<void>(`/api/tokens/${id}`, { method: 'DELETE' })
}
