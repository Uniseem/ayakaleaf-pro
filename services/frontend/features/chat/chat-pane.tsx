'use client'

/**
 * The chat panel, from chat/components/chat-pane.
 *
 * The messages load when the panel is first opened rather than when the
 * project does: most sessions never open it, and the history can be long.
 */

import { useEffect } from 'react'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import { Button } from '@/components/ol/button'
import { FullSizeLoadingSpinner } from '@/components/ol/spinner'
import { withErrorBoundary } from '@/components/ol/error-boundary'
import { useChatContext } from './contexts/chat-context'
import InfiniteScroll from './components/infinite-scroll'
import MessageList from './components/message-list'
import MessageInput from './components/message-input'
import { RailPanelHeader } from '@/features/ide/components/rail/rail-parts'

const Loading = () => <FullSizeLoadingSpinner delay={500} className="pt-4" />

function ChatPaneBody() {
  const { t } = useTranslation()
  const {
    status,
    messages,
    initialMessagesLoaded,
    atEnd,
    loadInitialMessages,
    loadMoreMessages,
    reload,
    reset,
    sendMessage,
    markMessagesAsRead,
    error,
  } = useChatContext()

  // Every time the panel is opened, not only the first. New messages are
  // pushed while the session is connected, but a message sent during a
  // reconnect was pushed to nobody, and opening the panel is when that would
  // be noticed.
  useEffect(() => {
    if (initialMessagesLoaded) {
      reload()
    } else {
      loadInitialMessages()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const shouldDisplayPlaceholder = status !== 'pending' && messages.length === 0

  if (error) {
    return <ChatFallbackError reconnect={reset} />
  }

  return (
    <div className="chat-panel">
      <RailPanelHeader title={t('collaborator_chat')} />
      <div className="chat-wrapper">
        <aside className="chat" aria-label={t('chat')}>
          <InfiniteScroll
            atEnd={atEnd}
            className="messages"
            fetchData={loadMoreMessages}
            isLoading={status === 'pending'}
            itemCount={messages.length}
          >
            <div className={cx({ 'h-100': shouldDisplayPlaceholder })}>
              <h2 className="visually-hidden">{t('chat')}</h2>
              {status === 'pending' && <Loading />}
              {shouldDisplayPlaceholder && <Placeholder />}
              <MessageList messages={messages} resetUnreadMessages={markMessagesAsRead} />
            </div>
          </InfiniteScroll>
          <MessageInput resetUnreadMessages={markMessagesAsRead} sendMessage={sendMessage} />
        </aside>
      </div>
    </div>
  )
}

function Placeholder() {
  const { t } = useTranslation()
  return (
    <div className="chat-empty-state-placeholder">
      <div>
        <span className="chat-empty-state-icon">
          <MaterialIcon type="forum" />
        </span>
      </div>
      <div>
        <div className="chat-empty-state-title">{t('no_messages_yet')}</div>
        <div className="chat-empty-state-body">
          {t('start_the_conversation_by_saying_hello_or_sharing_an_update')}
        </div>
      </div>
    </div>
  )
}

function ChatFallbackError({ reconnect }: { reconnect?: () => void }) {
  const { t } = useTranslation()

  return (
    <aside className="chat chat-error" aria-label={t('chat')}>
      <div className="chat-error-message">
        <p>{t('generic_something_went_wrong')}</p>
        <p>{t('try_again')}</p>
        {reconnect && (
          <Button variant="secondary" size="sm" onClick={reconnect}>
            {t('reconnect')}
          </Button>
        )}
      </div>
    </aside>
  )
}

const ChatPaneWithBoundary = withErrorBoundary(ChatPaneBody, () => <ChatFallbackError />)

export function ChatPane() {
  return <ChatPaneWithBoundary />
}

/** The count on the rail's chat tab. */
export function ChatIndicator() {
  const { unreadMessageCount } = useChatContext()

  if (unreadMessageCount === 0) {
    return null
  }

  return <span className="chat-indicator">{unreadMessageCount}</span>
}

export default ChatPane
