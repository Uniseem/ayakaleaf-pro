'use client'

/**
 * The card where a new comment is written, from
 * review-panel/components/review-panel-add-comment.
 *
 * It exists only while the range it belongs to is in the editor's
 * addCommentRanges set: cancelling removes the range, which removes the card.
 * Blurring an empty one counts as cancelling, so a card started by accident
 * goes away by clicking elsewhere.
 */

import {
  memo,
  useCallback,
  useRef,
  useState,
  type FormEventHandler,
  type KeyboardEvent,
} from 'react'
import { EditorSelection } from '@codemirror/state'
import { useTranslation } from '@/lib/i18n'
import { Button as OLButton } from '@/components/ol/button'
import { debugConsole } from '@/lib/debug'
import {
  useCodeMirrorStateContext,
  useCodeMirrorViewContext,
} from '@/features/source-editor/components/codemirror-context'
import { removeNewCommentRangeEffect } from '@/features/source-editor/extensions/review-tooltip'
import { useThreadsActionsContext } from '../contexts/threads-context'
import { ReviewPanelEntry } from './review-panel-entry'

export const ReviewPanelAddComment = memo<{
  docId: string
  from: number
  to: number
  threadId: string
  top: number | undefined
}>(function ReviewPanelAddComment({ from, to, threadId, top, docId }) {
  const { t } = useTranslation()
  const view = useCodeMirrorViewContext()
  const state = useCodeMirrorStateContext()
  const { addComment } = useThreadsActionsContext()
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [content, setContent] = useState('')

  const handleClose = useCallback(() => {
    view.dispatch({
      effects: removeNewCommentRangeEffect.of(threadId),
    })
  }, [view, threadId])

  const submitForm = useCallback(async () => {
    if (content.trim().length === 0) {
      return
    }

    setSubmitting(true)
    setError(null)

    const text = view.state.sliceDoc(from, to)

    try {
      await addComment(from, text, content)
      handleClose()
      view.dispatch({
        selection: EditorSelection.cursor(view.state.selection.main.anchor),
      })
    } catch (err) {
      debugConsole.error(err)
      setError(t('add_comment_error_message'))
    }
    setSubmitting(false)
  }, [content, view, from, to, addComment, handleClose, t])

  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLTextAreaElement>) => {
      // Enter submits; Shift+Enter is a new line, as in the chat.
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault()
        void submitForm()
      }
    },
    [submitForm]
  )

  const handleBlur = useCallback(() => {
    if (content === '') {
      window.setTimeout(() => {
        handleClose()
      })
    }
  }, [content, handleClose])

  const handleSubmit = useCallback<FormEventHandler>(
    event => {
      event.preventDefault()
      void submitForm()
    },
    [submitForm]
  )

  // Focused once the card has been positioned, not on mount: the panel sets
  // `top` after measuring, and focusing before that scrolls the panel to
  // wherever the card was first drawn.
  const hasBeenFocused = useRef(false)
  const observerRef = useRef<MutationObserver | null>(null)

  const handleElement = useCallback((element: HTMLElement | null) => {
    if (element) {
      element.dispatchEvent(new Event('review-panel:position'))

      observerRef.current = new MutationObserver(mutationList => {
        if (hasBeenFocused.current) {
          return
        }
        for (const mutation of mutationList) {
          const target = mutation.target as HTMLElement
          if (target.style.top) {
            const textArea = target.getElementsByTagName('textarea')[0]
            if (textArea) {
              textArea.focus()
              hasBeenFocused.current = true
            }
          }
        }
      })
      const entryWrapper = element.closest('.review-panel-entry')
      if (entryWrapper) {
        observerRef.current.observe(entryWrapper, {
          attributes: true,
          attributeFilter: ['style'],
        })
      }
    } else if (observerRef.current) {
      observerRef.current.disconnect()
    }
  }, [])

  return (
    <ReviewPanelEntry
      docId={docId}
      top={top}
      position={from}
      op={{
        p: from,
        c: state.sliceDoc(from, to),
        t: threadId,
      }}
      selectLineOnFocus={false}
      disabled={submitting}
    >
      <form
        className="review-panel-entry-content"
        onBlur={handleBlur}
        onSubmit={handleSubmit}
        ref={handleElement}
      >
        <textarea
          name="message"
          className="review-panel-add-comment-textarea"
          onChange={event => setContent(event.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={t('add_your_comment_here')}
          value={content}
          disabled={submitting}
          rows={3}
        />
        {error && <div className="review-panel-add-comment-error">{error}</div>}
        <div className="review-panel-add-comment-buttons">
          <OLButton
            variant="ghost"
            size="sm"
            className="review-panel-add-comment-cancel-button"
            disabled={submitting}
            onClick={handleClose}
          >
            {t('cancel')}
          </OLButton>
          <OLButton
            type="submit"
            variant="primary"
            size="sm"
            disabled={content === '' || submitting}
          >
            {t('comment')}
          </OLButton>
        </div>
      </form>
    </ReviewPanelEntry>
  )
})
