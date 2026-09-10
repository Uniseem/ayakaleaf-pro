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
  deleteThread as deleteThreadRequest,
  listThreads,
  reopenThread as reopenThreadRequest,
  resolveThread as resolveThreadRequest,
  type Thread,
} from '@/lib/comments'
import { debugConsole } from '@/lib/debug'
import { useProject } from '@/features/ide/contexts/project-context'

export type Threads = Record<string, Thread>

const ThreadsContext = createContext<Threads | undefined>(undefined)

type ThreadsActions = {
  refresh: () => Promise<void>
  addMessage: (threadId: string, content: string) => Promise<void>
  resolveThread: (threadId: string) => Promise<void>
  reopenThread: (threadId: string) => Promise<void>
  deleteThread: (threadId: string) => Promise<void>
}

const ThreadsActionsContext = createContext<ThreadsActions | undefined>(undefined)

export function ThreadsProvider({ children }: { children: ReactNode }) {
  const { projectId } = useProject()
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
    () => ({ refresh, addMessage, resolveThread, reopenThread, deleteThread }),
    [refresh, addMessage, resolveThread, reopenThread, deleteThread]
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
