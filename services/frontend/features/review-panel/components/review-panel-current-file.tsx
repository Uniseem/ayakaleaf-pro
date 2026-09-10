'use client'

/**
 * The cards for the open document, from
 * review-panel/components/review-panel-current-file.
 *
 * Each card is measured against the line it belongs to and then laid out by
 * positionItems, which is done after render because it needs the heights the
 * browser actually gave the cards. Until that pass runs the cards are hidden
 * rather than at the wrong place: a visible jump is worse than a blank frame.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useCodeMirrorStateContext, useCodeMirrorViewContext } from '@/features/source-editor/components/codemirror-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useReview } from '@/features/ide/contexts/review-context'
import { isDeletion, isInsertion, type TrackedChange } from '@/lib/ranges'
import { positionItems } from '../utils/position-items'
import { ReviewPanelChange } from './review-panel-change'
import { ReviewPanelComment } from './review-panel-comment'
import { ReviewPanelEmptyState } from './review-panel-empty-state'
import { useThreadsContext } from '../contexts/threads-context'
import { ReviewPanelAddComment } from './review-panel-add-comment'
import { reviewTooltipStateField } from '@/features/source-editor/extensions/review-tooltip'

/**
 * A deletion immediately followed by an insertion is one edit.
 *
 * Somebody replacing a word produces both, and showing them as two cards asks
 * the reader to accept "deleted cat" and "added dog" separately, which they
 * cannot sensibly do one at a time.
 */
function canAggregate(deletion: TrackedChange, insertion: TrackedChange): boolean {
  if (!isDeletion(deletion) || !isInsertion(insertion)) {
    return false
  }
  if (deletion.metadata?.user_id !== insertion.metadata?.user_id) {
    return false
  }
  return deletion.op.p === insertion.op.p
}

export function ReviewPanelCurrentFile() {
  const view = useCodeMirrorViewContext()
  const state = useCodeMirrorStateContext()
  const editor = useEditor()
  const { ranges } = useReview()
  const threads = useThreadsContext()
  const [hoveredEntry, setHoveredEntry] = useState<string | null>(null)

  const docId = editor.current?.id ?? ''

  const hoverTimeout = useRef<number>(0)
  const handleEntryEnter = useCallback((id: string) => {
    window.clearTimeout(hoverTimeout.current)
    setHoveredEntry(id)
  }, [])

  const handleEntryLeave = useCallback(() => {
    window.clearTimeout(hoverTimeout.current)
    hoverTimeout.current = window.setTimeout(() => setHoveredEntry(null), 100)
  }, [])

  const aggregated = useMemo(() => {
    const changes: TrackedChange[] = []
    const aggregates = new Map<string, TrackedChange>()

    const sorted = [...ranges.changes].sort((a, b) => a.op.p - b.op.p)

    for (let i = 0; i < sorted.length; i++) {
      const change = sorted[i]
      if (!change) continue
      const next = sorted[i + 1]
      if (next && canAggregate(change, next)) {
        aggregates.set(next.id, change)
        changes.push(next)
        i++
        continue
      }
      changes.push(change)
    }

    return { changes, aggregates }
  }, [ranges.changes])

  // Only the threads that are still open have a card here.
  const comments = useMemo(
    () =>
      ranges.comments.filter(comment => {
        const threadId = comment.op.t ?? comment.op.tid
        const thread = threadId ? threads?.[threadId] : undefined
        return !thread?.resolved
      }),
    [ranges.comments, threads]
  )

  // The ranges somebody has started a comment on but not yet written one for.
  // They live in the editor rather than here, because they move with the text.
  const addCommentRanges = state.field(reviewTooltipStateField, false)?.addCommentRanges

  const addCommentEntries = useMemo(() => {
    const entries: { id: string; from: number; to: number }[] = []
    if (!addCommentRanges) {
      return entries
    }
    const cursor = addCommentRanges.iter()
    while (cursor.value !== null) {
      entries.push({ id: cursor.value.spec.id, from: cursor.from, to: cursor.to })
      cursor.next()
    }
    return entries
  }, [addCommentRanges])

  const containerRef = useRef<HTMLDivElement>(null)
  const previousFocusedItemIndexRef = useRef<number>(0)

  /** Where in the panel a document offset lands, in pixels. */
  const topFor = useCallback(
    (position: number): number | undefined => {
      try {
        const coords = view.coordsAtPos(Math.min(position, state.doc.length))
        if (!coords) {
          return undefined
        }
        const scrollRect = view.scrollDOM.getBoundingClientRect()
        return coords.top - scrollRect.top + view.scrollDOM.scrollTop
      } catch {
        return undefined
      }
    },
    [view, state.doc.length]
  )

  // Laid out after every render, because the heights are only known once the
  // browser has drawn the cards.
  useEffect(() => {
    const container = containerRef.current
    if (!container) {
      return
    }
    positionItems(container, previousFocusedItemIndexRef, docId)
  })

  // And again whenever the document scrolls, because every card's anchor moved.
  useEffect(() => {
    const container = containerRef.current
    if (!container) {
      return
    }
    const onScroll = () => {
      positionItems(container, previousFocusedItemIndexRef, docId)
    }
    view.scrollDOM.addEventListener('scroll', onScroll)
    return () => view.scrollDOM.removeEventListener('scroll', onScroll)
  }, [view, docId])

  const nothingToShow =
    aggregated.changes.length === 0 &&
    comments.length === 0 &&
    addCommentEntries.length === 0

  return (
    <div className="review-panel-current-file" id="review-panel-current-file" ref={containerRef}>
      {nothingToShow ? (
        <ReviewPanelEmptyState />
      ) : (
        <>
          {addCommentEntries.map(entry => (
            <ReviewPanelAddComment
              key={entry.id}
              docId={docId}
              from={entry.from}
              to={entry.to}
              threadId={entry.id}
              top={topFor(entry.from)}
            />
          ))}
          {aggregated.changes.map(change => (
            <ReviewPanelChange
              key={change.id}
              change={change}
              aggregate={aggregated.aggregates.get(change.id)}
              top={topFor(change.op.p)}
              docId={docId}
              hovered={hoveredEntry === change.id}
              handleEnter={handleEntryEnter}
              handleLeave={handleEntryLeave}
            />
          ))}
          {comments.map(comment => (
            <ReviewPanelComment
              key={comment.id}
              comment={comment}
              top={topFor(comment.op.p)}
              docId={docId}
              hovered={hoveredEntry === comment.id}
              handleEnter={handleEntryEnter}
              handleLeave={handleEntryLeave}
            />
          ))}
        </>
      )}
    </div>
  )
}

export default ReviewPanelCurrentFile
