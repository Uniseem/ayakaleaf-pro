'use client'

/**
 * A piece of quoted text that can be opened out.
 *
 * A tracked change may be a paragraph. Showing all of it would make one card
 * taller than the panel, and showing a fixed slice would cut mid-word, so it
 * is clipped by line count and opens on request.
 */

import { useLayoutEffect, useRef, useState } from 'react'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'

export function ExpandableContent({
  content,
  inline = false,
  checkNewLines = true,
}: {
  content: string
  inline?: boolean
  checkNewLines?: boolean
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const [needsExpanding, setNeedsExpanding] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)

  const isMultiLine = checkNewLines && content.includes('\n')

  useLayoutEffect(() => {
    const element = contentRef.current
    if (element) {
      setNeedsExpanding(element.scrollHeight > element.clientHeight)
    }
  }, [content])

  return (
    <>
      <div
        ref={contentRef}
        className={cx('review-panel-content', {
          'review-panel-content-inline': inline,
          'review-panel-content-expanded': expanded,
          'review-panel-content-multi-line': isMultiLine,
        })}
      >
        {content}
      </div>
      {(needsExpanding || isMultiLine) && (
        <button
          type="button"
          className="review-panel-content-expand-button"
          onClick={event => {
            event.stopPropagation()
            setExpanded(value => !value)
          }}
        >
          {expanded ? t('show_less') : t('show_more')}
        </button>
      )}
    </>
  )
}

export default ExpandableContent
