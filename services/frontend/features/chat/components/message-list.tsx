'use client'

/**
 * The messages, grouped, from chat/components/message-list and message-group.
 *
 * Consecutive messages from the same person within five minutes are one group
 * with one name on it, because repeating the name on every line turns a
 * conversation into a list of records.
 */

import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import { nameOf } from '@/lib/chat'
import { useSite } from '@/features/ide/contexts/site-context'
import type { ChatMessage } from '../contexts/chat-context'

const FIVE_MINUTES = 5 * 60 * 1000

type MessageGroupData = {
  messages: ChatMessage[]
  id: string
  user?: ChatMessage['user']
}

function groupMessages(messages: ChatMessage[]): MessageGroupData[] {
  const groups: MessageGroupData[] = []
  let currentGroup: MessageGroupData | null = null
  let previousMessage: ChatMessage | null = null

  for (const message of messages) {
    if (
      currentGroup &&
      previousMessage &&
      !message.pending &&
      message.user?.id &&
      message.user.id === previousMessage.user?.id &&
      message.timestamp - previousMessage.timestamp < FIVE_MINUTES
    ) {
      currentGroup.messages.push(message)
    } else {
      currentGroup = { messages: [message], id: String(message.id), user: message.user }
      groups.push(currentGroup)
    }
    previousMessage = message
  }

  return groups
}

function formatTimestamp(value: number | undefined): string {
  if (!value) {
    return 'N/A'
  }
  const date = new Date(value)
  return date.toLocaleString(undefined, {
    hour: 'numeric',
    minute: '2-digit',
    day: 'numeric',
    month: 'short',
  })
}

export function MessageList({
  messages,
  resetUnreadMessages,
}: {
  messages: ChatMessage[]
  resetUnreadMessages: () => void
}) {
  const { user } = useSite()

  const messageGroups = groupMessages(messages)

  function shouldRenderDate(index: number) {
    if (index === 0) {
      return true
    }
    const group = messageGroups[index]
    const previous = messageGroups[index - 1]
    const at = group?.messages[0]?.timestamp
    const before = previous?.messages[previous.messages.length - 1]?.timestamp
    return Boolean(at && before && at - before > FIVE_MINUTES)
  }

  return (
    <ul className="list-unstyled" onClick={resetUnreadMessages} onKeyDown={resetUnreadMessages}>
      {messageGroups.map((group, index) => (
        <li key={group.id} className="message">
          {shouldRenderDate(index) && (
            <div className="date">
              <time
                dateTime={
                  group.messages[0]?.timestamp
                    ? new Date(group.messages[0].timestamp).toISOString()
                    : undefined
                }
              >
                {formatTimestamp(group.messages[0]?.timestamp)}
              </time>
            </div>
          )}
          <MessageGroup
            messages={group.messages}
            user={group.user}
            fromSelf={group.user?.id === user.id}
          />
        </li>
      ))}
    </ul>
  )
}

function MessageGroup({
  messages,
  user,
  fromSelf,
}: {
  messages: ChatMessage[]
  user?: ChatMessage['user']
  fromSelf: boolean
}) {
  const { t } = useTranslation()

  return (
    <div className="chat-message">
      <div>
        <div className="message-row">
          <div className="message-avatar-placeholder" />
          {!fromSelf && (
            <div className="message-author">
              <span>{user?.id ? nameOf(user) : t('deleted_user')}</span>
            </div>
          )}
        </div>
      </div>
      {messages.map(message => (
        <div
          key={message.id}
          className={cx('message-row', { 'message-row-self': fromSelf })}
        >
          <div className={cx('message-container', { 'message-pending': message.pending })}>
            <div className="message-content">{message.content}</div>
          </div>
        </div>
      ))}
    </div>
  )
}

export default MessageList
