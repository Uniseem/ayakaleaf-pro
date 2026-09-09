'use client'

import { useCallback, useState } from 'react'
import { useResizeObserver } from '@/features/source-editor/hooks/use-resize-observer'
import { useTranslation } from '@/lib/i18n'
import cx from '@/lib/cx'
import { Button } from '@/components/ol/button'

/** The raw text of an entry, cut short with an Expand button when long. */
export default function PdfLogEntryRawContent({
  rawContent,
  collapsedSize = 0,
  alwaysExpanded = false,
}: {
  rawContent: string
  collapsedSize?: number
  alwaysExpanded?: boolean
}) {
  const [expanded, setExpanded] = useState(alwaysExpanded)
  const [needsExpander, setNeedsExpander] = useState(!alwaysExpanded)

  const { elementRef } = useResizeObserver(
    useCallback(
      (element: Element) => {
        if (element.scrollHeight === 0) return // skip update when logs-pane is closed
        setNeedsExpander(!alwaysExpanded && element.scrollHeight > collapsedSize)
      },
      [collapsedSize, alwaysExpanded]
    )
  )

  const { t } = useTranslation()

  return (
    <div className="log-entry-content-raw-container">
      <div
        className="expand-collapse-container"
        style={{
          height: expanded || !needsExpander ? 'auto' : collapsedSize,
        }}
      >
        <pre className="log-entry-content-raw" ref={elementRef} translate="no">
          {rawContent.trim()}
        </pre>
      </div>

      {needsExpander && (
        <div
          className={cx('log-entry-content-button-container', {
            'log-entry-content-button-container-collapsed': !expanded,
          })}
        >
          <Button
            variant="secondary"
            size="sm"
            leadingIcon={expanded ? 'expand_less' : 'expand_more'}
            onClick={() => setExpanded(value => !value)}
          >
            {expanded ? t('collapse') : t('expand')}
          </Button>
        </div>
      )}
    </div>
  )
}
