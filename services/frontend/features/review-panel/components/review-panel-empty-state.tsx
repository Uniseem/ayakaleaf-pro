'use client'

import MaterialIcon from '@/components/ol/material-icon'
import { useTranslation } from '@/lib/i18n'

export function ReviewPanelEmptyState() {
  const { t } = useTranslation()

  return (
    <div className="review-panel-empty-state">
      <div className="review-panel-empty-state-inner">
        <div className="review-panel-empty-state-comment-icon">
          <MaterialIcon type="question_answer" />
        </div>
        <p>
          <strong>{t('no_comments_or_suggestions')}</strong>
        </p>
        <p>{t('no_one_has_commented_or_left_any_suggestions_yet')}</p>
      </div>
    </div>
  )
}

export default ReviewPanelEmptyState
