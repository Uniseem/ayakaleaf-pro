'use client'

/**
 * One tracked change, from review-panel/components/review-panel-change.
 *
 * Accept is the owner's; reject is the owner's or the person who suggested it.
 * A deletion immediately followed by an insertion is shown as one "changed X
 * to Y" card rather than two, because that is what replacing a word does.
 */

import { memo, useCallback, useMemo, useState } from 'react'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import { useProject } from '@/features/ide/contexts/project-context'
import { useReview } from '@/features/ide/contexts/review-context'
import { useSite } from '@/features/ide/contexts/site-context'
import { isDeletion, isInsertion, type TrackedChange } from '@/lib/ranges'
import { hueFor, nameFor } from '../utils/change-users'
import { ReviewPanelEntry } from './review-panel-entry'
import { ExpandableContent } from './review-panel-expandable-content'
import { ChangeAction } from './review-panel-change-action'
import { AddIcon, DeleteIcon, EditIcon } from './review-panel-action-icons'

export const ReviewPanelChange = memo(function ReviewPanelChange({
  change,
  aggregate,
  top,
  docId,
  editable = true,
  hovered,
  handleEnter,
  handleLeave,
}: {
  change: TrackedChange
  aggregate?: TrackedChange
  top?: number
  editable?: boolean
  docId: string
  hovered?: boolean
  handleEnter?: (changeId: string) => void
  handleLeave?: () => void
}) {
  const { t } = useTranslation()
  const { accept, reject } = useReview()
  const { canWrite, canReview } = useProject()
  const { user } = useSite()

  const [accepting, setAccepting] = useState(false)

  const acceptHandler = useCallback(async () => {
    setAccepting(true)
    try {
      const ids = aggregate ? [change.id, aggregate.id] : [change.id]
      await accept(ids)
    } finally {
      setAccepting(false)
    }
  }, [accept, aggregate, change.id])

  const rejectHandler = useCallback(async () => {
    const ids = aggregate ? [change.id, aggregate.id] : [change.id]
    await reject(ids)
  }, [aggregate, change.id, reject])

  const { handleMouseEnter, handleMouseLeave } = useMemo(
    () => ({
      handleMouseEnter: handleEnter && (() => handleEnter(change.id)),
      handleMouseLeave: handleLeave && (() => handleLeave()),
    }),
    [change.id, handleEnter, handleLeave]
  )

  const isChangeAuthor = change.metadata?.user_id === user.id
  const aggregateChange = aggregate && isDeletion(aggregate) && /\S/.test(aggregate.op.d)

  return (
    <ReviewPanelEntry
      className={cx('review-panel-entry-change', {
        'review-panel-entry-insert': isInsertion(change),
        'review-panel-entry-delete': isDeletion(change),
        'review-panel-entry-hover': hovered,
      })}
      top={top}
      op={change.op}
      position={change.op.p}
      docId={docId}
      disabled={accepting}
      handleEnter={handleMouseEnter}
      handleLeave={handleMouseLeave}
      entryIndicator="edit"
    >
      <div
        className="review-panel-entry-content"
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
      >
        <div className="review-panel-entry-header">
          <div>
            <ChangeUser userId={change.metadata?.user_id} />
            {change.metadata?.ts && (
              <div className="review-panel-entry-time">{formatTime(change.metadata.ts)}</div>
            )}
          </div>
          {editable && (
            <div className="review-panel-entry-actions">
              {canWrite && (
                <ChangeAction
                  id={`accept-change-${change.id}`}
                  label={t('accept_change')}
                  type="check"
                  handleClick={acceptHandler}
                />
              )}

              {(canWrite || (canReview && isChangeAuthor)) && (
                <ChangeAction
                  id={`reject-change-${change.id}`}
                  label={t('reject_change')}
                  type="close"
                  handleClick={rejectHandler}
                />
              )}
            </div>
          )}
        </div>

        <div className="review-panel-change-body">
          {isInsertion(change) && (
            <>
              {aggregateChange ? <EditIcon /> : <AddIcon />}

              {aggregateChange && aggregate && isDeletion(aggregate) ? (
                <span>
                  {t('aggregate_changed')}:{' '}
                  <del className="review-panel-content-highlight">
                    <ExpandableContent inline content={aggregate.op.d} checkNewLines={false} />
                  </del>{' '}
                  {t('aggregate_to')}{' '}
                  <ExpandableContent inline content={change.op.i} checkNewLines={false} />
                </span>
              ) : (
                <span>
                  {t('tracked_change_added')}:&nbsp;
                  <ins className="review-panel-content-highlight">
                    <ExpandableContent content={change.op.i} checkNewLines={false} />
                  </ins>
                </span>
              )}
            </>
          )}

          {isDeletion(change) && (
            <>
              <DeleteIcon />
              <span>
                {t('tracked_change_deleted')}:&nbsp;
                <del className="review-panel-content-highlight">
                  <ExpandableContent content={change.op.d} checkNewLines={false} />
                </del>
              </span>
            </>
          )}
        </div>
      </div>
    </ReviewPanelEntry>
  )
})

export function ChangeUser({ userId }: { userId?: string }) {
  const { t } = useTranslation()
  const name = nameFor(userId) ?? t('unknown')
  const hue = hueFor(userId)

  return (
    <div className="review-panel-entry-user">
      <span className="review-panel-entry-user-colour" style={{ backgroundColor: `hsl(${hue}, 70%, 50%)` }} />
      <span className="review-panel-entry-user-name">{name}</span>
    </div>
  )
}

/** The original shows a time within this year and a date before it. */
function formatTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return ''
  }
  const sameYear = date.getFullYear() === new Date().getFullYear()
  return date.toLocaleString(undefined, {
    hour: 'numeric',
    minute: '2-digit',
    day: 'numeric',
    month: 'short',
    ...(sameYear ? {} : { year: 'numeric' }),
  })
}

export default ReviewPanelChange
