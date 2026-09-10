'use client'

/**
 * Everything outstanding, by file, from
 * review-panel/components/review-panel-overview.
 *
 * The current-file view can only speak about the document that is open. This
 * one is how somebody finds the three suggestions left in a chapter they are
 * not looking at, so the cards here are in document order and not positioned
 * against anything.
 */

import { useMemo, useState } from 'react'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { useReview } from '@/features/ide/contexts/review-context'
import { ReviewPanelChange } from './review-panel-change'
import { ReviewPanelComment } from './review-panel-comment'
import { ReviewPanelEmptyState } from './review-panel-empty-state'
import { useThreadsContext } from '../contexts/threads-context'

export function ReviewPanelOverview() {
  const { t } = useTranslation()
  const editor = useEditor()
  const { entryById } = useProject()
  const { ranges } = useReview()
  const threads = useThreadsContext()

  const docId = editor.current?.id ?? ''
  const docName = entryById(docId)?.path ?? editor.current?.name ?? ''

  const comments = useMemo(
    () =>
      ranges.comments.filter(comment => {
        const threadId = comment.op.t ?? comment.op.tid
        const thread = threadId ? threads?.[threadId] : undefined
        return !thread?.resolved
      }),
    [ranges.comments, threads]
  )

  const total = ranges.changes.length + comments.length

  const [collapsed, setCollapsed] = useState(false)

  if (total === 0) {
    return (
      <div className="review-panel-overview" id="review-panel-overview">
        <ReviewPanelEmptyState />
      </div>
    )
  }

  return (
    <div className="review-panel-overview" id="review-panel-overview">
      <div className="review-panel-overview-file">
        <button
          type="button"
          className="review-panel-overview-file-header"
          onClick={() => setCollapsed(value => !value)}
          aria-expanded={!collapsed}
        >
          <MaterialIcon type={collapsed ? 'chevron_right' : 'expand_more'} />
          <span className="review-panel-overview-file-name">{docName || t('current_file')}</span>
          <span className="review-panel-overview-file-count">{total}</span>
        </button>

        <div className={cx('review-panel-overview-file-entries', { hidden: collapsed })}>
          {ranges.changes.map(change => (
            <ReviewPanelChange key={change.id} change={change} docId={docId} />
          ))}
          {comments.map(comment => (
            <ReviewPanelComment key={comment.id} comment={comment} docId={docId} />
          ))}
        </div>
      </div>
    </div>
  )
}

export default ReviewPanelOverview
