'use client'

/**
 * The review panel, from review-panel/components/review-panel and its
 * container.
 *
 * Two places, one component. In the rail it is a normal panel with a header.
 * Beside the editor it is drawn into CodeMirror's own scrolling element, so
 * that the cards scroll with the text they are about rather than being
 * positioned against a moving target from outside.
 */

import { memo, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useCodeMirrorViewContext } from '@/features/source-editor/components/codemirror-context'
import { ReviewPanelViewProvider, useReviewPanelViewContext } from './contexts/review-panel-view-context'
import { ThreadsProvider } from './contexts/threads-context'
import ReviewPanelTabs from './components/review-panel-tabs'
import ReviewPanelCurrentFile from './components/review-panel-current-file'
import ReviewPanelOverview from './components/review-panel-overview'
import ReviewModeSwitcher from './components/review-mode-switcher'
import { ReviewPanelResolvedThreadsButton } from './components/review-panel-resolved-threads'

function ReviewPanelBody({ mini = false }: { mini?: boolean }) {
  const chosenSubView = useReviewPanelViewContext()
  const activeSubView = mini ? 'cur_file' : chosenSubView

  const className = cx('review-panel-container', {
    'review-panel-mini': mini,
    'review-panel-subview-overview': activeSubView === 'overview',
  })

  return (
    <div className={className} data-testid="review-panel">
      <div id="review-panel-inner" className="review-panel-inner">
        {activeSubView === 'cur_file' && <ReviewPanelCurrentFile />}
        {activeSubView === 'overview' && <ReviewPanelOverview />}

        <div className="review-panel-footer" id="review-panel-tabs" role="tablist">
          <ReviewPanelTabs />
        </div>
      </div>
    </div>
  )
}

/** The rail's version: a header, then the panel. */
export const ReviewPanel = memo(function ReviewPanel() {
  const { t } = useTranslation()

  return (
    <ThreadsProvider>
      <ReviewPanelViewProvider>
        <div className="review-panel-rail">
          <div className="review-panel-header">
            <RailPanelHeader title={t('review')} actions={<ReviewPanelResolvedThreadsButton />} />
          </div>
          <ReviewPanelBody />
        </div>
      </ReviewPanelViewProvider>
    </ThreadsProvider>
  )
})

function RailPanelHeader({ title, actions }: { title: string; actions?: ReactNode }) {
  return (
    <div className="rail-panel-header">
      <h4 className="rail-panel-title">{title}</h4>
      {actions ? <div className="rail-panel-header-actions">{actions}</div> : null}
    </div>
  )
}

/**
 * The version that lives inside the editor: the mode switcher, and the cards
 * when the panel is open.
 */
export const ReviewPanelContainer = memo(function ReviewPanelContainer() {
  const view = useCodeMirrorViewContext()
  const { reviewPanelOpen, focusMode } = useLayout()

  if (!view) {
    return null
  }

  return createPortal(
    <ThreadsProvider>
      <ReviewPanelViewProvider>
        {!focusMode && <ReviewModeSwitcher />}
        {reviewPanelOpen && <ReviewPanelBody mini />}
      </ReviewPanelViewProvider>
    </ThreadsProvider>,
    view.scrollDOM
  )
})

export default ReviewPanel
