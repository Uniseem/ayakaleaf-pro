'use client'

/**
 * The project chat, from chat/context/chat-context.
 *
 * Messages are read newest first and prepended, because that is the order the
 * reader wants and the order the API can answer without counting. A message
 * this person just sent is shown before the server has confirmed it, marked
 * pending, so the conversation does not stutter on a slow connection.
 *
 * Unread is counted here rather than by the panel: the count has to keep
 * rising while the panel is closed, which is exactly when the panel is not
 * mounted to do the counting.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { listMessages, sendMessage as sendMessageRequest, type Message } from '@/lib/chat'
import { messageFor } from '@/lib/api'
import { useProject } from '@/features/ide/contexts/project-context'
import { useConnection } from '@/features/ide/contexts/connection-context'
import { useSite } from '@/features/ide/contexts/site-context'

const PAGE_SIZE = 50

export type ChatMessage = Message & { pending?: boolean }

type ChatValue = {
  status: 'idle' | 'pending' | 'error'
  messages: ChatMessage[]
  initialMessagesLoaded: boolean
  atEnd: boolean
  unreadMessageCount: number
  error: string | null
  loadInitialMessages: () => void
  loadMoreMessages: () => void
  sendMessage: (content: string) => void
  markMessagesAsRead: () => void
  reset: () => void
}

const ChatContext = createContext<ChatValue | undefined>(undefined)

export function ChatProvider({ children }: { children: ReactNode }) {
  const { projectId } = useProject()
  const { socket } = useConnection()
  const { user } = useSite()

  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [status, setStatus] = useState<ChatValue['status']>('idle')
  const [error, setError] = useState<string | null>(null)
  const [initialMessagesLoaded, setInitialMessagesLoaded] = useState(false)
  const [atEnd, setAtEnd] = useState(false)
  const [unreadMessageCount, setUnreadMessageCount] = useState(0)

  const loadingRef = useRef(false)

  const load = useCallback(
    async (before?: number) => {
      if (loadingRef.current) {
        return
      }
      loadingRef.current = true
      setStatus('pending')
      setError(null)
      try {
        const page = await listMessages(projectId, { limit: PAGE_SIZE, before })
        // Answered newest first; shown oldest first.
        const ordered = [...page].sort((a, b) => a.timestamp - b.timestamp)
        setMessages(current => (before ? [...ordered, ...current] : ordered))
        setAtEnd(page.length < PAGE_SIZE)
        setStatus('idle')
      } catch (thrown) {
        setError(messageFor(thrown))
        setStatus('error')
      } finally {
        loadingRef.current = false
      }
    },
    [projectId]
  )

  const loadInitialMessages = useCallback(() => {
    if (initialMessagesLoaded) {
      return
    }
    setInitialMessagesLoaded(true)
    void load()
  }, [initialMessagesLoaded, load])

  const loadMoreMessages = useCallback(() => {
    if (atEnd || loadingRef.current) {
      return
    }
    const oldest = messages[0]
    void load(oldest?.timestamp)
  }, [atEnd, messages, load])

  const sendMessage = useCallback(
    (content: string) => {
      const trimmed = content.trim()
      if (!trimmed) {
        return
      }

      const pendingId = `pending-${Date.now()}`
      const pending: ChatMessage = {
        id: pendingId,
        content: trimmed,
        timestamp: Date.now(),
        user: { id: user.id, email: user.email, firstName: user.firstName, lastName: user.lastName },
        pending: true,
      }
      setMessages(current => [...current, pending])

      sendMessageRequest(projectId, trimmed)
        .then(sent => {
          setMessages(current => current.map(message => (message.id === pendingId ? sent : message)))
        })
        .catch(thrown => {
          // The message did not go. Taking it back is better than leaving it
          // looking sent.
          setMessages(current => current.filter(message => message.id !== pendingId))
          setError(messageFor(thrown))
        })
    },
    [projectId, user]
  )

  // Somebody else's message, pushed while this session is open.
  useEffect(() => {
    if (!socket) {
      return
    }
    return socket.on('new-chat-message', (...args: unknown[]) => {
      const message = args[0] as Message | undefined
      if (!message) {
        return
      }
      setMessages(current =>
        current.some(existing => existing.id === message.id) ? current : [...current, message]
      )
      if (message.user?.id !== user.id) {
        setUnreadMessageCount(count => count + 1)
      }
    })
  }, [socket, user.id])

  const markMessagesAsRead = useCallback(() => setUnreadMessageCount(0), [])

  const reset = useCallback(() => {
    setMessages([])
    setInitialMessagesLoaded(false)
    setAtEnd(false)
    setError(null)
    setStatus('idle')
  }, [])

  const value = useMemo<ChatValue>(
    () => ({
      status,
      messages,
      initialMessagesLoaded,
      atEnd,
      unreadMessageCount,
      error,
      loadInitialMessages,
      loadMoreMessages,
      sendMessage,
      markMessagesAsRead,
      reset,
    }),
    [
      status,
      messages,
      initialMessagesLoaded,
      atEnd,
      unreadMessageCount,
      error,
      loadInitialMessages,
      loadMoreMessages,
      sendMessage,
      markMessagesAsRead,
      reset,
    ]
  )

  return <ChatContext.Provider value={value}>{children}</ChatContext.Provider>
}

export function useChatContext(): ChatValue {
  const context = useContext(ChatContext)
  if (!context) {
    throw new Error('useChatContext is only available inside ChatProvider')
  }
  return context
}
