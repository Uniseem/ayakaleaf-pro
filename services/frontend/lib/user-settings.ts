/**
 * A person's own settings, as the API holds them.
 *
 * Separate from lib/settings, which is the instance's configuration and only
 * an administrator may touch. These follow the person between machines.
 */

import { api } from './api'

/** Everything the server stores about how somebody likes the editor. */
export type StoredUserSettings = Record<string, unknown>

export function getUserSettings(
  headers?: Record<string, string>
): Promise<StoredUserSettings> {
  return api<StoredUserSettings>('/api/settings', { headers })
}

/**
 * Writes the settings that changed.
 *
 * A partial update rather than the whole object: two tabs open on the same
 * account would otherwise overwrite each other's unrelated changes with
 * whichever one saved last.
 */
export function saveUserSettings(
  values: StoredUserSettings
): Promise<StoredUserSettings> {
  return api<StoredUserSettings>('/api/settings', {
    method: 'POST',
    body: values,
  })
}

/** Changing a password, which is not a setting and is answered differently. */
export function changePassword(input: {
  currentPassword: string
  newPassword: string
}): Promise<void> {
  return api<void>('/api/settings/password', { method: 'POST', body: input })
}

/** The sessions this account has open elsewhere. */
export type Session = {
  ipAddress: string
  sessionCreatedAt: string
  isCurrent: boolean
}

export function listSessions(
  headers?: Record<string, string>
): Promise<Session[]> {
  return api<{ sessions: Session[] }>('/api/settings/sessions', {
    headers,
  }).then(answer => answer.sessions)
}

/** Ends every session but this one. */
export function clearSessions(): Promise<void> {
  return api<void>('/api/settings/sessions', { method: 'DELETE' })
}

export function deleteAccount(password: string): Promise<void> {
  return api<void>('/api/settings/account', {
    method: 'DELETE',
    body: { password },
  })
}
