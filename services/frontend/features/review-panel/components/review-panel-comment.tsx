'use client'

/**
 * One comment thread, from review-panel/components/review-panel-comment.
 *
 * The card shows the quoted text the comment is anchored to, then the
 * conversation, then a box to reply. Resolving hides the thread from the panel
 * without deleting anything, which is why resolved threads have their own list
 * rather than simply disappearing.
 */

import { memo, useCallback, useState } from 'react'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import { Button } from '@/components/ol/button'
import MaterialIcon from '@/components/ol/material-icon'
import { nameOf } from '@/lib/chat'
import type { CommentAnchor } from '@/lib/ranges'
import { useProject } from '@/features/ide/contexts/project-context'
import { useThreadsActionsContext, useThreadsContext } from '../contexts/threads-context'
import { ReviewPanelEntry } from './review-panel-entry'
import { ExpandableContent } from './review-panel-expandable-content'
import { ChangeAction } from './review-panel-change-action'

export const ReviewPanelComment = memo(function ReviewPanelComment({
  comment,
  docId,
  top,
  hovered,
  handleEnter,
  handleLeave,
}: {
  comment: CommentAnchor
  docId: string
  top?: number
  hovered?: boolean
  handleEnter?: (id: string) => void
  handleLeave?: (id: string) => void
}) {
  const { t } = useTranslation()
  const threads = useThreadsContext()
  const { addMessage, resolveThread } = useThreadsActionsContext()
  const { canWrite, canReview } = useProject()

  const threadId = comment.op.t ?? comment.op.tid
  const thread = threadId ? threads?.[threadId] : undefined

  const [replying, setReplying] = useState(false)
  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)

  const submit = useCallback(async () => {
    const content = draft.trim()
    if (!content || !threadId) {
      return
    }
    setSending(true)
    try {
      await addMessage(threadId, content)
      setDraft('')
      setReplying(false)
    } finally {
      setSending(false)
    }
  }, [draft, threadId, addMessage])

  // A thread that has been resolved belongs in the resolved list, not here.
  if (thread?.resolved) {
    return null
  }

  return (
    <ReviewPanelEntry
      className={cx('review-panel-entry-comment', { 'review-panel-entry-hover': hovered })}
      top={top}
      op={comment.op}
      position={comment.op.p}
      docId={docId}
      handleEnter={handleEnter && (() => handleEnter(comment.id))}
      handleLeave={handleLeave && (() => handleLeave(comment.id))}
      entryIndicator="comment"
    >
      <div className="review-panel-entry-content">
        <div className="review-panel-entry-header">
          <div className="review-panel-entry-quote">
            <MaterialIcon type="format_quote" className="review-panel-entry-icon" />
            <ExpandableContent inline content={comment.op.c} />
          </div>
          {(canWrite || canReview) && threadId && (
            <div className="review-panel-entry-actions">
              <ChangeAction
                id={`resolve-thread-${threadId}`}
                label={t('resolve_comment')}
                type="check"
                handleClick={() => void resolveThread(threadId)}
              />
            </div>
          )}
        </div>

        <div className="review-panel-comment-body">
          {thread?.messages.map(message => (
            <div key={message.id} className="review-panel-comment-message">
              <div className="review-panel-entry-user">
                <span className="review-panel-entry-user-name">{nameOf(message.user)}</span>
                <span className="review-panel-entry-time">{formatTime(message.timestamp)}</span>
              </div>
              <div className="review-panel-comment-message-content">{message.content}</div>
            </div>
          ))}
        </div>

        {(canWrite || canReview) &&
          (replying ? (
            <div className="review-panel-comment-reply">
              <textarea
                className="review-panel-comment-input"
                value={draft}
                autoFocus
                placeholder={t('reply')}
                onChange={event => setDraft(event.target.value)}
                onKeyDown={event => {
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault()
                    void submit()
                  }
                  if (event.key === 'Escape') {
                    setReplying(false)
                  }
                }}
              />
              <div className="review-panel-comment-reply-actions">
                <Button size="sm" variant="secondary" onClick={() => setReplying(false)} disabled={sending}>
                  {t('cancel')}
                </Button>
                <Button
                  size="sm"
                  variant="primary"
                  onClick={() => void submit()}
                  disabled={!draft.trim() || sending}
                  isLoading={sending}
                >
                  {t('reply')}
                </Button>
              </div>
            </div>
          ) : (
            <button type="button" className="review-panel-comment-reply-prompt" onClick={() => setReplying(true)}>
              {t('reply')}
            </button>
          ))}
      </div>
    </ReviewPanelEntry>
  )
})

function formatTime(value: string | number | undefined): string {
  if (value === undefined) {
    return ''
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return ''
  }
  return date.toLocaleString(undefined, { hour: 'numeric', minute: '2-digit', day: 'numeric', month: 'short' })
}

export default ReviewPanelComment
