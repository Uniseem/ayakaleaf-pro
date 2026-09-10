'use client'

/**
 * The bubble that appears over a selection, from
 * review-panel/components/review-tooltip-menu.
 *
 * It is rendered into the tooltip CodeMirror positioned for it rather than
 * beside it, so it follows the selection without a second layout pass. Its
 * appearance is delayed a moment: a selection made by dragging passes through
 * every intermediate range, and a bubble that appeared instantly would flicker
 * across the line as the mouse moved.
 */

import {
  memo,
  useCallback,
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
  type FC,
} from 'react'
import ReactDOM from 'react-dom'
import { EditorSelection } from '@codemirror/state'
import { EditorView, getTooltip } from '@codemirror/view'
import classNames from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import { Tooltip as OLTooltip } from '@/components/ol/tooltip'
import { useEventListener } from '@/lib/hooks'
import {
  useCodeMirrorStateContext,
  useCodeMirrorViewContext,
} from '@/features/source-editor/components/codemirror-context'
import {
  buildAddNewCommentRangeEffect,
  reviewTooltipStateField,
} from '@/features/source-editor/extensions/review-tooltip'
import { selectHighlightedOrNearestToken } from '@/features/source-editor/utils/select-highlighted-or-nearest-token'
import { isCursorNearViewportEdge } from '@/features/source-editor/utils/is-cursor-near-edge'
import { useProject } from '@/features/ide/contexts/project-context'
import { useReview } from '@/features/ide/contexts/review-context'
import { useRailContext } from '@/features/ide/contexts/rail-context'
import { useReviewPanelViewActionsContext } from '../contexts/review-panel-view-context'
import { isInsertion } from '@/lib/ranges'

const EDIT_MODE_SWITCH_WIDGET_HEIGHT = 40
const CM_LINE_RIGHT_PADDING = 8
const TOOLTIP_SHOW_DELAY = 120

const ReviewTooltipMenu: FC = () => {
  const state = useCodeMirrorStateContext()
  const view = useCodeMirrorViewContext()
  const { canReview } = useProject()
  const [show, setShow] = useState(true)
  const { setView } = useReviewPanelViewActionsContext()
  const { openTab } = useRailContext()
  const tooltipState = state.field(reviewTooltipStateField, false)?.tooltip
  const [previousTooltipState, setPreviousTooltipState] = useState(tooltipState)

  useEffect(() => {
    if (tooltipState !== previousTooltipState) {
      if (tooltipState != null && previousTooltipState == null) {
        setShow(true)
      }
      setPreviousTooltipState(tooltipState)
    }
  }, [tooltipState, previousTooltipState])

  useEffect(() => {
    if (!show || !tooltipState || !canReview) {
      return
    }
    const handleMouseDown = (event: MouseEvent) => {
      const target = event.target as Element | null
      if (
        !view.contentDOM.contains(target) &&
        !target?.closest?.('.review-tooltip-menu-container') &&
        !target?.closest?.('.modal') &&
        !target?.closest?.('.modal-backdrop')
      ) {
        setShow(false)
      }
    }
    document.addEventListener('mousedown', handleMouseDown)
    return () => {
      document.removeEventListener('mousedown', handleMouseDown)
    }
  }, [show, tooltipState, canReview, view])

  const addComment = useCallback(() => {
    if (!canReview) {
      return
    }

    let { main } = view.state.selection

    if (main.empty) {
      const tokenRange = selectHighlightedOrNearestToken(view.state)
      if (!tokenRange) {
        return
      }
      main = EditorSelection.range(tokenRange.from, tokenRange.to)
    }

    openTab('review-panel')
    setView('cur_file')

    const effects = isCursorNearViewportEdge(view, main.anchor)
      ? [
          buildAddNewCommentRangeEffect(main),
          EditorView.scrollIntoView(main.anchor, { y: 'center' }),
        ]
      : [buildAddNewCommentRangeEffect(main)]

    view.dispatch({
      selection: { anchor: main.anchor, head: main.head },
      effects,
    })
    setShow(false)
  }, [view, canReview, openTab, setView])

  useEventListener('add-new-review-comment', addComment)

  if (!canReview || !show || !tooltipState) {
    return null
  }

  const tooltipView = getTooltip(view, tooltipState)

  if (!tooltipView) {
    return null
  }

  return ReactDOM.createPortal(
    <ReviewTooltipMenuContent onAddComment={addComment} />,
    tooltipView.dom
  )
}

const ReviewTooltipMenuContent = memo<{ onAddComment: () => void }>(
  function ReviewTooltipMenuContent({ onAddComment }) {
    const { t } = useTranslation()
    const view = useCodeMirrorViewContext()
    const state = useCodeMirrorStateContext()
    const { isOpen: railIsOpen } = useRailContext()
    const { ranges, accept, reject, suggesting } = useReview()
    const [tooltipStyle, setTooltipStyle] = useState<CSSProperties | undefined>()
    const [visible, setVisible] = useState(false)

    const changesInSelection = useMemo(() => {
      return ranges.changes.filter(change => {
        const opFrom = change.op.p
        const opLength = isInsertion(change) ? change.op.i.length : 0
        const opTo = opFrom + opLength
        const selection = state.selection.main
        return opFrom >= selection.from && opTo <= selection.to
      })
    }, [ranges, state.selection.main])

    const acceptChangesHandler = useCallback(() => {
      void accept(changesInSelection.map(change => change.id))
    }, [accept, changesInSelection])

    const rejectChangesHandler = useCallback(() => {
      void reject(changesInSelection.map(change => change.id))
    }, [reject, changesInSelection])

    const showChangesButtons = changesInSelection.length > 0

    // A selection that has scrolled out of sight leaves the tooltip with
    // nothing to point at, so it is pinned to the top of the viewport instead
    // of being drawn off-screen.
    useEffect(() => {
      view.requestMeasure({
        key: 'review-tooltip-outside-viewport',
        read(view) {
          const cursorCoords = view.coordsAtPos(view.state.selection.main.head)

          if (!cursorCoords) {
            return
          }

          const scrollDomRect = view.scrollDOM.getBoundingClientRect()
          const contentDomRect = view.contentDOM.getBoundingClientRect()
          const editorRightPos = contentDomRect.right - CM_LINE_RIGHT_PADDING

          if (
            cursorCoords.top > scrollDomRect.top &&
            cursorCoords.top < scrollDomRect.bottom
          ) {
            return
          }

          return {
            position: 'fixed' as const,
            top: scrollDomRect.top + EDIT_MODE_SWITCH_WIDGET_HEIGHT,
            right: window.innerWidth - editorRightPos,
          }
        },
        write(res) {
          setTooltipStyle(res as CSSProperties | undefined)
        },
      })
    }, [view, railIsOpen, suggesting])

    useEffect(() => {
      setVisible(false)
      const timeout = setTimeout(() => {
        setVisible(true)
      }, TOOLTIP_SHOW_DELAY)

      return () => {
        clearTimeout(timeout)
      }
    }, [])

    return (
      <div
        className={classNames('review-tooltip-menu', {
          'review-tooltip-menu-visible': visible,
        })}
        style={tooltipStyle}
      >
        <button
          className="review-tooltip-menu-button review-tooltip-add-comment-button"
          onClick={onAddComment}
          aria-label={t('add_comment')}
        >
          <MaterialIcon type="chat" />
          {t('add_comment')}
        </button>
        {showChangesButtons && (
          <>
            <div className="review-tooltip-menu-divider" />
            <OLTooltip
              id="accept-all-changes"
              description={t('accept_selected_changes')}
            >
              <button
                className="review-tooltip-menu-button"
                onClick={acceptChangesHandler}
                aria-label={t('accept_selected_changes')}
              >
                <MaterialIcon type="check" />
              </button>
            </OLTooltip>

            <OLTooltip
              id="reject-all-changes"
              description={t('reject_selected_changes')}
            >
              <button
                className="review-tooltip-menu-button"
                onClick={rejectChangesHandler}
                aria-label={t('reject_selected_changes')}
              >
                <MaterialIcon type="clear" />
              </button>
            </OLTooltip>
          </>
        )}
      </div>
    )
  }
)

export default ReviewTooltipMenu
