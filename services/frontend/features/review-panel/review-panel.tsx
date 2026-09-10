'use client'

/**
 * The review panel, from review-panel/components/review-panel and its
 * container.
 *
 * It is drawn into CodeMirror's own scrolling element, and only there. The
 * cards are positioned against the lines they are about, so they have to
 * scroll with the text rather than be aimed at it from outside; that is also
 * why the rail's review tab has no component of its own and only decides
 * whether this shows in full or as a strip of marks in the margin.
 */

import { memo } from 'react'
import { createPortal } from 'react-dom'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useCodeMirrorViewContext } from '@/features/source-editor/components/codemirror-context'
import { useReviewPanelViewContext } from './contexts/review-panel-view-context'
import ReviewPanelTabs from './components/review-panel-tabs'
import ReviewPanelCurrentFile from './components/review-panel-current-file'
import ReviewPanelOverview from './components/review-panel-overview'
import ReviewModeSwitcher from './components/review-mode-switcher'
import { ReviewPanelResolvedThreadsButton } from './components/review-panel-resolved-threads'
import useReviewPanelLayout from './hooks/use-review-panel-layout'

function ReviewPanelBody({ mini, showHeader }: { mini: boolean; showHeader: boolean }) {
  const { t } = useTranslation()
  const chosenSubView = useReviewPanelViewContext()
  // The narrow form has no room for the overview, and no tabs to switch with.
  const activeSubView = mini ? 'cur_file' : chosenSubView

  const className = cx('review-panel-container', {
    'review-panel-mini': mini,
    'review-panel-subview-overview': activeSubView === 'overview',
  })

  return (
    <div className={className} data-testid="review-panel">
      <div id="review-panel-inner" className="review-panel-inner">
        {showHeader && (
          <div className="review-panel-header">
            <div className="rail-panel-header">
              <h4 className="rail-panel-title">{t('review')}</h4>
              <div className="rail-panel-header-actions">
                <ReviewPanelResolvedThreadsButton />
              </div>
            </div>
          </div>
        )}

        {activeSubView === 'cur_file' && <ReviewPanelCurrentFile />}
        {activeSubView === 'overview' && <ReviewPanelOverview />}

        {!mini && (
          <div className="review-panel-footer" id="review-panel-tabs" role="tablist">
            <ReviewPanelTabs />
          </div>
        )}
      </div>
    </div>
  )
}

/**
 * The mode switcher, and the panel when there is anything to show.
 *
 * Rendered by the editor so that both are inside the CodeMirror context they
 * depend on.
 */
export const ReviewPanelContainer = memo(function ReviewPanelContainer() {
  const view = useCodeMirrorViewContext()
  const { focusMode } = useLayout()
  const { showPanel, showHeader, mini } = useReviewPanelLayout()

  if (!view) {
    return null
  }

  return createPortal(
    <>
      {!focusMode && <ReviewModeSwitcher />}
      {showPanel && <ReviewPanelBody mini={mini} showHeader={showHeader} />}
    </>,
    view.scrollDOM
  )
})

export default ReviewPanelContainer
