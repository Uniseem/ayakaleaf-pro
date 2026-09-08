/** The account page: which providers are linked, and changing a password. */

import { api } from './api'

export type LinkedProvider = {
  id: string
  name: string
  path: string
  linked: boolean
}

export function linkedProviders(
  headers?: Record<string, string>
): Promise<LinkedProvider[]> {
  return api<{ providers: LinkedProvider[] }>('/api/auth/providers', { headers }).then(
    answer => answer.providers
  )
}

export function unlinkProvider(id: string): Promise<void> {
  return api<void>(`/api/auth/${id}/link`, { method: 'DELETE' })
}
