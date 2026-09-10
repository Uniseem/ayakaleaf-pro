'use client'

import { memo } from 'react'
import cx from '@/lib/cx'
import MaterialIcon from '@/components/ol/material-icon'
import { useTranslation } from '@/lib/i18n'
import {
  useReviewPanelViewActionsContext,
  useReviewPanelViewContext,
} from '../contexts/review-panel-view-context'

export const ReviewPanelTabs = memo(function ReviewPanelTabs() {
  const subView = useReviewPanelViewContext()
  const { setView: setSubView } = useReviewPanelViewActionsContext()

  const { t } = useTranslation()

  return (
    <>
      <button
        role="tab"
        aria-selected={subView === 'cur_file'}
        aria-controls="review-panel-current-file"
        id="review-panel-tab-button-current-file"
        className={cx('review-panel-tab', { 'review-panel-tab-active': subView === 'cur_file' })}
        onClick={() => setSubView('cur_file')}
      >
        <MaterialIcon type="description" />
        {t('current_file')}
      </button>
      <button
        role="tab"
        aria-selected={subView === 'overview'}
        aria-controls="review-panel-overview"
        id="review-panel-tab-button-overview"
        className={cx('review-panel-tab', { 'review-panel-tab-active': subView === 'overview' })}
        onClick={() => setSubView('overview')}
      >
        <MaterialIcon type="list" />
        {t('overview')}
      </button>
    </>
  )
})

export default ReviewPanelTabs
