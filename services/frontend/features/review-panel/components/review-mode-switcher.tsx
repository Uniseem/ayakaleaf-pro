'use client'

/**
 * Editing, Reviewing or Viewing, from
 * review-panel/components/review-mode-switcher.
 *
 * It floats over the top right of the editor rather than living in a toolbar,
 * because it is the answer to "what will happen when I type", and that has to
 * be visible from wherever the cursor is.
 *
 * The first time somebody sees it the label is shown next to the icon and then
 * collapses, so the control introduces itself once instead of costing that
 * width forever.
 */

import { forwardRef, memo, type MouseEventHandler } from 'react'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import { usePersistedState } from '@/lib/hooks'
import { Dropdown, DropdownMenu, DropdownToggle, OLDropdownMenuItem } from '@/components/ol/dropdown'
import MaterialIcon from '@/components/ol/material-icon'
import { useProject } from '@/features/ide/contexts/project-context'
import { useReview, type ReviewMode } from '@/features/ide/contexts/review-context'
import { useCodeMirrorViewContext } from '@/features/source-editor/components/codemirror-context'

function ReviewModeSwitcher() {
  const { t } = useTranslation()
  const { mode, setMode } = useReview()
  const { canWrite, canReview, features } = useProject()
  const view = useCodeMirrorViewContext()

  // Only somebody who cannot write at all is offered "viewing": for everybody
  // else it is not a mode, it is just not typing.
  const showViewOption = !canWrite && !canReview

  const choose = (next: ReviewMode) => {
    if (mode !== next) {
      setMode(next)
    }
    view?.focus()
  }

  return (
    <div className="review-mode-switcher-container">
      <Dropdown className="review-mode-switcher" align="end">
        <DropdownToggle as={ModeSwitcherToggleButton} id="review-mode-switcher" />
        <DropdownMenu flip={false}>
          <OLDropdownMenuItem
            as="button"
            disabled={!canWrite}
            onClick={() => choose('editing')}
            description={t('edit_content_directly')}
            leadingIcon="edit"
            active={canWrite && mode === 'editing'}
          >
            {t('editing')}
          </OLDropdownMenuItem>
          <OLDropdownMenuItem
            as="button"
            disabled={!features.trackChanges || (!canWrite && !canReview)}
            onClick={() => choose('suggesting')}
            description={canReview && !canWrite ? t('comment_only') : t('edits_become_suggestions')}
            leadingIcon="rate_review"
            active={mode === 'suggesting'}
          >
            {t('reviewing')}
          </OLDropdownMenuItem>
          {showViewOption && (
            <OLDropdownMenuItem
              as="button"
              description={t('can_view_content')}
              leadingIcon="visibility"
              active={mode === 'viewing'}
            >
              {t('viewing')}
            </OLDropdownMenuItem>
          )}
        </DropdownMenu>
      </Dropdown>
    </div>
  )
}

const ModeSwitcherToggleButton = forwardRef<
  HTMLButtonElement,
  { onClick?: MouseEventHandler<HTMLButtonElement>; 'aria-expanded'?: boolean }
>(function ModeSwitcherToggleButton({ onClick, 'aria-expanded': ariaExpanded }, ref) {
  const { t } = useTranslation()
  const { mode } = useReview()

  const look =
    mode === 'editing'
      ? { className: 'editing', iconType: 'edit', label: t('editing') }
      : mode === 'suggesting'
        ? { className: 'reviewing', iconType: 'rate_review', label: t('reviewing') }
        : { className: 'viewing', iconType: 'visibility', label: t('viewing') }

  return (
    <ModeSwitcherToggleButtonContent
      ref={ref}
      onClick={onClick}
      ariaExpanded={Boolean(ariaExpanded)}
      {...look}
    />
  )
})

const ModeSwitcherToggleButtonContent = forwardRef<
  HTMLButtonElement,
  {
    onClick?: MouseEventHandler<HTMLButtonElement>
    className: string
    iconType: string
    label: string
    ariaExpanded: boolean
  }
>(function ModeSwitcherToggleButtonContent({ onClick, className, iconType, label, ariaExpanded }, ref) {
  const [isFirstTimeUsed, setIsFirstTimeUsed] = usePersistedState('modeSwitcherFirstTimeUsed', true)

  return (
    <button
      type="button"
      className={cx('review-mode-switcher-toggle-button', className, {
        'review-mode-switcher-toggle-button-expanded': isFirstTimeUsed,
      })}
      ref={ref}
      onClick={event => {
        setIsFirstTimeUsed(false)
        onClick?.(event)
      }}
      aria-expanded={ariaExpanded}
      aria-label={label}
    >
      <MaterialIcon type={iconType} />
      <div className="review-mode-switcher-toggle-label">{label}</div>
      <MaterialIcon type="keyboard_arrow_down" />
    </button>
  )
})

export default memo(ReviewModeSwitcher)
