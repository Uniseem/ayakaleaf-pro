'use client'

/**
 * The project's chat.
 *
 * Messages are grouped by author, so a run of five from one person is one
 * block with one name on it rather than five headed rows. Grouping breaks on
 * a different author or a gap of more than five minutes, which is what makes
 * a conversation from an hour ago read as a separate conversation.
 */

import { Avatar, Button, ScrollShadow, Spinner, Textarea } from '@heroui/react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { listMessages, nameOf, sendMessage, type Message } from '@/lib/chat'
import { messageFor } from '@/lib/api'
import { useProject } from '@/features/ide/contexts/project-context'

/** Messages closer together than this from one person are one block. */
const GROUP_WINDOW = 5 * 60 * 1000

type Group = { user: Message['user']; messages: Message[] }

function group(messages: Message[]): Group[] {
  const groups: Group[] = []
  for (const message of messages) {
    const last = groups[groups.length - 1]
    const previous = last?.messages[last.messages.length - 1]
    if (
      last &&
      previous &&
      last.user.id === message.user.id &&
      message.timestamp - previous.timestamp < GROUP_WINDOW
    ) {
      last.messages.push(message)
    } else {
      groups.push({ user: message.user, messages: [message] })
    }
  }
  return groups
}

export function ChatPane() {
  const { projectId } = useProject()
  const [messages, setMessages] = useState<Message[]>([])
  const [draft, setDraft] = useState('')
  const [loading, setLoading] = useState(true)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const bottom = useRef<HTMLDivElement>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const found = await listMessages(projectId, { limit: 100 })
      // The API answers newest first; reading order is oldest first.
      setMessages([...found].reverse())
    } catch (thrown) {
      setError(messageFor(thrown))
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    bottom.current?.scrollIntoView({ block: 'end' })
  }, [messages])

  const send = useCallback(async () => {
    const content = draft.trim()
    if (!content || sending) {
      return
    }
    setSending(true)
    setError(null)
    try {
      const sent = await sendMessage(projectId, content)
      setMessages(previous => [...previous, sent])
      setDraft('')
    } catch (thrown) {
      setError(messageFor(thrown))
    } finally {
      setSending(false)
    }
  }, [draft, sending, projectId])

  const groups = useMemo(() => group(messages), [messages])

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex items-center justify-between border-b border-divider px-3 py-2">
        <span className="text-xs font-semibold uppercase tracking-wide text-default-500">
          Chat
        </span>
        <Button
          size="sm"
          variant="light"
          className="h-6 min-w-0 px-2 text-xs"
          onPress={() => void load()}
          isDisabled={loading}
        >
          Refresh
        </Button>
      </header>

      <ScrollShadow className="min-h-0 flex-1 px-3 py-2">
        {loading ? (
          <div className="flex justify-center py-6">
            <Spinner size="sm" />
          </div>
        ) : error ? (
          <p className="py-4 text-xs text-danger">{error}</p>
        ) : groups.length === 0 ? (
          <p className="py-6 text-xs text-default-400">
            Nothing here yet. Messages stay with the project, so this is the
            place for something the next person to open it should know.
          </p>
        ) : (
          <ul className="flex flex-col gap-3">
            {groups.map((entry, index) => (
              <MessageGroup key={`${entry.user.id}-${index}`} group={entry} />
            ))}
          </ul>
        )}
        <div ref={bottom} />
      </ScrollShadow>

      <div className="border-t border-divider p-2">
        <Textarea
          value={draft}
          onValueChange={setDraft}
          placeholder="Write a message"
          minRows={1}
          maxRows={5}
          size="sm"
          onKeyDown={event => {
            // Enter sends, shift-enter makes a line. A chat box that needs a
            // button is a chat box nobody uses.
            if (event.key === 'Enter' && !event.shiftKey) {
              event.preventDefault()
              void send()
            }
          }}
        />
        <div className="mt-1 flex items-center justify-between">
          <span className="text-[10px] text-default-400">
            Enter sends, Shift+Enter makes a line
          </span>
          <Button
            size="sm"
            color="primary"
            className="h-7"
            isLoading={sending}
            isDisabled={!draft.trim()}
            onPress={() => void send()}
          >
            Send
          </Button>
        </div>
      </div>
    </div>
  )
}

function MessageGroup({ group: entry }: { group: Group }) {
  const name = nameOf(entry.user)
  const first = entry.messages[0]
  return (
    <li className="flex gap-2">
      <Avatar name={name} size="sm" className="h-6 w-6 shrink-0 text-[10px]" />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="truncate text-xs font-medium">{name}</span>
          {first ? (
            <time className="text-[10px] text-default-400">
              {new Date(first.timestamp).toLocaleString()}
            </time>
          ) : null}
        </div>
        {entry.messages.map(message => (
          <p
            key={message.id}
            className="whitespace-pre-wrap break-words text-xs text-default-700"
          >
            {message.content}
          </p>
        ))}
      </div>
    </li>
  )
}
