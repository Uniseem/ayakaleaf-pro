'use client'

/**
 * Whether the review panel is showing, and how much of it.
 *
 * From review-panel/hooks/use-review-panel-layout. There are three states,
 * not two. Selecting the rail's review tab opens the full panel. With the tab
 * closed but something outstanding in the document, a narrow strip of marks
 * shows in the margin instead, because a suggestion nobody can see is a
 * suggestion nobody will act on. With neither, nothing is drawn.
 *
 * The panel itself is always inside the editor, never in the rail: the cards
 * are positioned against the lines they are about, which only the editor
 * knows. The rail's tab is a switch, which is why it has no component of its
 * own.
 */

import { useCallback } from 'react'
import { useRailContext } from '@/features/ide/contexts/rail-context'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useReview } from '@/features/ide/contexts/review-context'
import { useThreadsContext } from '../contexts/threads-context'
import { hasActiveRange } from '../utils/has-active-range'

export function useReviewPanelLayout(): {
  showPanel: boolean
  showHeader: boolean
  mini: boolean
  openReviewPanel: () => void
  closeReviewPanel: () => void
} {
  const { ranges } = useReview()
  const threads = useThreadsContext()
  const { selectedTab, isOpen: railIsOpen, openTab, setIsOpen } = useRailContext()
  const { focusMode } = useLayout()

  const reviewPanelOpen = selectedTab === 'review-panel' && railIsOpen

  const openReviewPanel = useCallback(() => {
    openTab('review-panel')
  }, [openTab])

  const closeReviewPanel = useCallback(() => {
    setIsOpen(false)
  }, [setIsOpen])

  const hasCommentOrChange = hasActiveRange(ranges, threads)
  const showPanel = focusMode ? false : reviewPanelOpen || Boolean(hasCommentOrChange)
  const mini = !reviewPanelOpen
  const showHeader = showPanel && !mini

  return { showPanel, showHeader, mini, openReviewPanel, closeReviewPanel }
}

export default useReviewPanelLayout
