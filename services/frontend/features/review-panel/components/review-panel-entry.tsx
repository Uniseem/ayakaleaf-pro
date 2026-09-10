'use client'

/**
 * The card around one change or comment, from
 * review-panel/components/review-panel-entry.
 *
 * Clicking a card puts the cursor at the thing it is about. That is the whole
 * point of the panel, and it is also why the card cannot simply be selected on
 * focus: moving the cursor scrolls the document, which repositions every card,
 * which would pull the one being clicked out from under the pointer. So a
 * press is remembered and the selection waits for the release.
 */

import { useCallback, useRef, useState, type ReactNode } from 'react'
import cx from '@/lib/cx'
import { EditorSelection } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import {
  useCodeMirrorStateContext,
  useCodeMirrorViewContext,
} from '@/features/source-editor/components/codemirror-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { useOpenDocAtLine } from '@/features/ide/hooks/use-open-doc-at-line'
import type { CommentOp, DeleteOp, InsertOp } from '@/lib/ranges'
import { OFFSET_FOR_ENTRIES_ABOVE } from '../utils/position-items'
import { EntryIndicator } from './review-panel-entry-indicator'

type AnyOperation = InsertOp | DeleteOp | CommentOp

/** Whether the cursor is inside the range this card is about. */
function isSelectionWithinOp(op: AnyOperation, from: number, to: number): boolean {
  const start = op.p
  const length = 'i' in op ? op.i.length : 'c' in op ? op.c.length : 0
  const end = start + length
  return from <= end && to >= start
}

export function ReviewPanelEntry({
  children,
  position,
  top,
  op,
  className,
  selectLineOnFocus = true,
  docId,
  disabled,
  handleEnter,
  handleLeave,
  entryIndicator,
}: {
  children: ReactNode
  position: number
  op: AnyOperation
  docId: string
  top?: number
  className?: string
  selectLineOnFocus?: boolean
  disabled?: boolean
  handleEnter?: () => void
  handleLeave?: () => void
  entryIndicator?: 'comment' | 'edit'
}) {
  const state = useCodeMirrorStateContext()
  const view = useCodeMirrorViewContext()
  const editor = useEditor()
  const { entryById } = useProject()
  const { openDocWithId } = useOpenDocAtLine()
  const [selected, setSelected] = useState(false)
  const [focused, setFocused] = useState(false)
  const [textareaFocused, setTextareaFocused] = useState(false)
  const highlighted = isSelectionWithinOp(op, state.selection.main.from, state.selection.main.to)
  const entryRef = useRef<HTMLDivElement>(null)
  const mousePressedRef = useRef(false)

  const selectEntry = useCallback(
    (event: React.FocusEvent | React.MouseEvent) => {
      setFocused(true)

      if (event.target instanceof HTMLTextAreaElement) {
        const entryBottom = (entryRef.current?.offsetTop || 0) + (entryRef.current?.offsetHeight || 0)

        // A reply box that is already on screen should not drag the card
        // around while somebody types into it.
        if (entryBottom > OFFSET_FOR_ENTRIES_ABOVE) {
          setTextareaFocused(true)
          return
        }
      }

      if (mousePressedRef.current) {
        return
      }

      setSelected(true)

      if (!selectLineOnFocus) {
        return
      }

      if (editor.current?.id !== docId) {
        const entry = entryById(docId)
        if (entry) {
          openDocWithId(docId, { keepCurrentView: true })
        }
        return
      }

      setTimeout(() => {
        const selection = EditorSelection.cursor(position)

        // Outside the viewport the line's position is an estimate, so let
        // CodeMirror do the scrolling; inside it the measurement is real and
        // a smooth scroll to the middle reads better.
        if (position < view.viewport.from || view.viewport.to < position) {
          view.dispatch({
            selection,
            effects: EditorView.scrollIntoView(selection, { y: 'center' }),
          })
          return
        }

        view.dispatch({ selection })

        const blockInfo = view.lineBlockAt(position)
        const coordsAtPos = view.coordsAtPos(position)
        const coordsAtLineStart = view.coordsAtPos(blockInfo.from)
        let wrappedLineOffset = 0
        if (coordsAtPos !== null && coordsAtLineStart !== null) {
          wrappedLineOffset = coordsAtPos.top - coordsAtLineStart.top
        }

        const editorHeight = view.scrollDOM.getBoundingClientRect().height
        view.scrollDOM.scrollTo({
          top: blockInfo.top - editorHeight / 2 + view.defaultLineHeight + wrappedLineOffset,
          behavior: 'smooth',
        })
      })
    },
    [editor, docId, selectLineOnFocus, view, position, openDocWithId, entryById]
  )

  return (
    <div
      ref={entryRef}
      onMouseDown={() => {
        mousePressedRef.current = true
      }}
      onMouseUp={event => {
        mousePressedRef.current = false
        const isTextSelected = Boolean(window.getSelection()?.toString())
        if (!isTextSelected && !selected) {
          selectEntry(event)
        }
      }}
      onFocus={selectEntry}
      onBlur={() => {
        mousePressedRef.current = false
        setSelected(false)
        setFocused(false)
        setTextareaFocused(false)
      }}
      role="button"
      tabIndex={position + 1}
      className={cx(
        'review-panel-entry',
        {
          // Picked by hand, which is how a card outside the viewport is shown.
          'review-panel-entry-selected': selected,
          // Clicked but not selected, e.g. a menu inside it. Only raises it
          // above the others, which are not in visual order in the DOM.
          'review-panel-entry-focused': focused,
          // The cursor is inside this range. More than one can be.
          'review-panel-entry-highlighted': highlighted,
          // Only changes the border, so typing does not reposition anything.
          'review-panel-entry-textarea-focused': textareaFocused,
          'review-panel-entry-disabled': disabled,
        },
        className
      )}
      data-top={top}
      data-pos={position}
      style={{
        position: top === undefined ? 'relative' : 'absolute',
        visibility: top === undefined ? 'visible' : 'hidden',
        transition: 'top .3s, left .1s, right .1s',
      }}
    >
      {entryIndicator && (
        <EntryIndicator
          type={entryIndicator}
          handleMouseEnter={handleEnter}
          handleMouseLeave={handleLeave}
        />
      )}
      {children}
    </div>
  )
}

export default ReviewPanelEntry
