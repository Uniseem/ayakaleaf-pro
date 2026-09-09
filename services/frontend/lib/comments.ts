/** Comment threads on a project. */

import { api } from './api'
import type { Message } from './chat'

export type Thread = {
  id: string
  messages: Message[]
  resolved: boolean
  resolvedBy?: Message['user']
  resolvedAt?: string
}

export function listThreads(projectId: string): Promise<Record<string, Thread>> {
  return api<{ threads: Record<string, Thread> }>(
    `/api/projects/${projectId}/threads`
  ).then(answer => answer.threads)
}

export function comment(
  projectId: string,
  threadId: string,
  content: string
): Promise<Message> {
  return api<{ message: Message }>(
    `/api/projects/${projectId}/threads/${threadId}/messages`,
    { method: 'POST', body: { content } }
  ).then(answer => answer.message)
}

export function resolveThread(projectId: string, threadId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/threads/${threadId}/resolve`, {
    method: 'POST',
  })
}

export function reopenThread(projectId: string, threadId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/threads/${threadId}/reopen`, {
    method: 'POST',
  })
}

export function deleteThread(projectId: string, threadId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/threads/${threadId}`, {
    method: 'DELETE',
  })
}

/**
 * Makes an id for a new thread.
 *
 * Made in the browser rather than asked for: the id has to exist before the
 * comment does, because it is what anchors the comment to a range in the
 * document, and that anchor is written into the text as part of the same edit.
 * An id from the server would need a round trip in the middle of that.
 */
export function newThreadId(): string {
  const bytes = new Uint8Array(12)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')
}
