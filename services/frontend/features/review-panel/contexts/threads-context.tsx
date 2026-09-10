'use client'

/**
 * The comment threads, from review-panel/context/threads-context.
 *
 * A thread is the conversation; the range in the document that points at it
 * is a separate thing, kept with the tracked changes. They are loaded apart
 * because they change apart: typing moves the ranges and says nothing about
 * the messages, and replying adds a message without moving anything.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import {
  comment as postComment,
  newThreadId,
  deleteThread as deleteThreadRequest,
  listThreads,
  reopenThread as reopenThreadRequest,
  resolveThread as resolveThreadRequest,
  type Thread,
} from '@/lib/comments'
import { debugConsole } from '@/lib/debug'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useReview } from '@/features/ide/contexts/review-context'

export type Threads = Record<string, Thread>

const ThreadsContext = createContext<Threads | undefined>(undefined)

type ThreadsActions = {
  refresh: () => Promise<void>
  addMessage: (threadId: string, content: string) => Promise<void>
  /**
   * Starts a thread on a range of the open document.
   *
   * The message is posted first and the range second, because a range
   * pointing at a thread that does not exist would draw a comment marker with
   * nothing behind it; the other order leaves at worst an empty thread that
   * nothing points at.
   */
  addComment: (position: number, text: string, content: string) => Promise<void>
  resolveThread: (threadId: string) => Promise<void>
  reopenThread: (threadId: string) => Promise<void>
  deleteThread: (threadId: string) => Promise<void>
}

const ThreadsActionsContext = createContext<ThreadsActions | undefined>(undefined)

export function ThreadsProvider({ children }: { children: ReactNode }) {
  const { projectId } = useProject()
  const editor = useEditor()
  const review = useReview()
  const [threads, setThreads] = useState<Threads>({})

  const refresh = useCallback(async () => {
    try {
      setThreads(await listThreads(projectId))
    } catch (error) {
      debugConsole.error(error)
    }
  }, [projectId])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const addMessage = useCallback(
    async (threadId: string, content: string) => {
      await postComment(projectId, threadId, content)
      await refresh()
    },
    [projectId, refresh]
  )

  const addComment = useCallback(
    async (position: number, text: string, content: string) => {
      const threadId = newThreadId()
      await postComment(projectId, threadId, content)
      await editor.comment(position, text, threadId)
      await Promise.all([refresh(), review.refresh()])
    },
    [projectId, editor, review, refresh]
  )

  const resolveThread = useCallback(
    async (threadId: string) => {
      await resolveThreadRequest(projectId, threadId)
      await refresh()
    },
    [projectId, refresh]
  )

  const reopenThread = useCallback(
    async (threadId: string) => {
      await reopenThreadRequest(projectId, threadId)
      await refresh()
    },
    [projectId, refresh]
  )

  const deleteThread = useCallback(
    async (threadId: string) => {
      await deleteThreadRequest(projectId, threadId)
      await refresh()
    },
    [projectId, refresh]
  )

  const actions = useMemo<ThreadsActions>(
    () => ({ refresh, addMessage, addComment, resolveThread, reopenThread, deleteThread }),
    [refresh, addMessage, addComment, resolveThread, reopenThread, deleteThread]
  )

  return (
    <ThreadsContext.Provider value={threads}>
      <ThreadsActionsContext.Provider value={actions}>{children}</ThreadsActionsContext.Provider>
    </ThreadsContext.Provider>
  )
}

export function useThreadsContext(): Threads | undefined {
  return useContext(ThreadsContext)
}

export function useThreadsActionsContext(): ThreadsActions {
  const context = useContext(ThreadsActionsContext)
  if (!context) {
    throw new Error('useThreadsActionsContext is only available inside ThreadsProvider')
  }
  return context
}
