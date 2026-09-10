'use client'

/**
 * The resolved comments, from
 * review-panel/components/review-panel-resolved-threads-button and its menu.
 *
 * Resolving takes a thread out of the panel but does not delete it, so there
 * has to be somewhere to read one back and reopen it. Behind a button rather
 * than in the list, because the list is meant to be what is still outstanding.
 */

import { useRef, useState } from 'react'
import { useTranslation } from '@/lib/i18n'
import { Tooltip } from '@/components/ol/tooltip'
import MaterialIcon from '@/components/ol/material-icon'
import { Button } from '@/components/ol/button'
import { Overlay, Popover } from '@/components/ol/overlay'
import { nameOf } from '@/lib/chat'
import { useThreadsActionsContext, useThreadsContext } from '../contexts/threads-context'

export function ReviewPanelResolvedThreadsButton() {
  const [expanded, setExpanded] = useState(false)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const { t } = useTranslation()

  return (
    <>
      <Tooltip
        id="resolved-comments"
        overlayProps={{ placement: 'bottom' }}
        description={t('resolved_comments')}
      >
        <button
          type="button"
          className="review-panel-resolved-comments-toggle"
          ref={buttonRef}
          onClick={() => setExpanded(value => !value)}
          aria-label={t('resolved_comments')}
        >
          <MaterialIcon type="inbox" />
        </button>
      </Tooltip>
      {expanded && (
        <Overlay
          show
          onHide={() => setExpanded(false)}
          containerPadding={0}
          placement="bottom"
          rootClose
          target={buttonRef.current}
        >
          <Popover id="popover-resolved-threads" className="review-panel-resolved-comments">
            <ReviewPanelResolvedThreadsMenu />
          </Popover>
        </Overlay>
      )}
    </>
  )
}

function ReviewPanelResolvedThreadsMenu() {
  const { t } = useTranslation()
  const threads = useThreadsContext()
  const { reopenThread, deleteThread } = useThreadsActionsContext()

  const resolved = Object.entries(threads ?? {}).filter(([, thread]) => thread.resolved)

  if (!resolved.length) {
    return <div className="review-panel-resolved-comments-empty">{t('no_resolved_comments')}</div>
  }

  return (
    <div className="review-panel-resolved-comments-list">
      {resolved.map(([id, thread]) => (
        <div key={id} className="review-panel-resolved-comment">
          <div className="review-panel-resolved-comment-messages">
            {thread.messages.map(message => (
              <div key={message.id} className="review-panel-comment-message">
                <div className="review-panel-entry-user">
                  <span className="review-panel-entry-user-name">{nameOf(message.user)}</span>
                </div>
                <div className="review-panel-comment-message-content">{message.content}</div>
              </div>
            ))}
          </div>
          <div className="review-panel-resolved-comment-actions">
            <Button size="sm" variant="link" onClick={() => void reopenThread(id)}>
              {t('reopen')}
            </Button>
            <Button size="sm" variant="link" onClick={() => void deleteThread(id)}>
              {t('delete')}
            </Button>
          </div>
        </div>
      ))}
    </div>
  )
}

export default ReviewPanelResolvedThreadsButton
