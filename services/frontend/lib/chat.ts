/** The messages people leave each other in a project. */

import { api } from './api'

export type Message = {
  id: string
  content: string
  timestamp: number
  user: {
    id: string
    email?: string
    firstName?: string
    lastName?: string
  }
}

/**
 * Reads messages, newest first.
 *
 * `before` is a timestamp rather than a page number: messages arrive while
 * somebody is reading back through them, and a page number would shift under
 * them every time one did.
 */
export function listMessages(
  projectId: string,
  options: { limit?: number; before?: number } = {}
): Promise<Message[]> {
  const query = new URLSearchParams()
  if (options.limit) {
    query.set('limit', String(options.limit))
  }
  if (options.before) {
    query.set('before', String(options.before))
  }
  const suffix = query.toString() ? `?${query}` : ''
  return api<{ messages: Message[] }>(
    `/api/projects/${projectId}/messages${suffix}`
  ).then(answer => answer.messages)
}

export function sendMessage(
  projectId: string,
  content: string
): Promise<Message> {
  return api<{ message: Message }>(`/api/projects/${projectId}/messages`, {
    method: 'POST',
    body: { content },
  }).then(answer => answer.message)
}

/** A name to show for whoever wrote a message. */
export function nameOf(user: Message['user']): string {
  const full = [user.firstName, user.lastName].filter(Boolean).join(' ')
  return full || user.email || 'Someone'
}
