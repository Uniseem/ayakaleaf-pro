/** What the API says about signing up, in and out. */

import { api } from './api'

export type PublicUser = {
  id: string
  email: string
  firstName?: string
  lastName?: string
  isAdmin: boolean
}

export type Provider = {
  id: string
  name: string
  path: string
}

export type AuthStatus = {
  open: boolean
  firstUser: boolean
  allowedDomains: string[]
  providers: Provider[]
  minPasswordLength: number
}

export type SessionResponse = {
  user: PublicUser
  redirect: string
}

/** What the sign-up page needs before it offers the form. */
export function authStatus(headers?: Record<string, string>): Promise<AuthStatus> {
  return api<AuthStatus>('/api/auth/status', { headers })
}

/** Who the request is from, or null. */
export async function currentUser(headers?: Record<string, string>): Promise<PublicUser | null> {
  const answer = await api<{ user: PublicUser | null }>('/api/auth/me', { headers })
  return answer.user
}

export function register(input: {
  email: string
  password: string
  firstName?: string
  lastName?: string
}): Promise<SessionResponse> {
  return api<SessionResponse>('/api/auth/register', { method: 'POST', body: input })
}

export function login(input: { email: string; password: string }): Promise<SessionResponse> {
  return api<SessionResponse>('/api/auth/login', { method: 'POST', body: input })
}

export function logout(): Promise<{ redirect: string }> {
  return api<{ redirect: string }>('/api/auth/logout', { method: 'POST' })
}

/** The display name for somebody, falling back to their address. */
export function displayName(user: PublicUser): string {
  const name = [user.firstName, user.lastName].filter(Boolean).join(' ').trim()
  return name || user.email
}

/**
 * Asks for a password reset link.
 *
 * Answers the same way whether or not the address has an account: telling
 * those apart would make this a way to ask whether somebody is a user here.
 */
export function requestPasswordReset(email: string): Promise<{ message: string }> {
  return api<{ message: string }>('/api/auth/password/reset', {
    method: 'POST',
    body: { email },
  })
}

/** Finishes a reset, with the token from the emailed link. */
export function setPasswordFromToken(input: {
  token: string
  password: string
}): Promise<void> {
  return api<void>('/api/auth/password/set', { method: 'POST', body: input })
}
