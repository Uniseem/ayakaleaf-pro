'use client'

import type { ReactNode } from 'react'
import { useTranslation } from '@/lib/i18n'
import PdfLogEntryRawContent from './pdf-log-entry-raw-content'
import type { LogEntry } from '../util/types'
import cx from '@/lib/cx'

/** The body of a log entry: the hint, a link to more, and the raw text. */
export default function PdfLogEntryContent({
  rawContent,
  formattedContent,
  extraInfoURL,
  alwaysExpandRawContent = false,
  className,
}: {
  rawContent?: string
  formattedContent?: ReactNode
  extraInfoURL?: string | null
  index?: number
  logEntry?: LogEntry
  alwaysExpandRawContent?: boolean
  className?: string
}) {
  const { t } = useTranslation()

  return (
    <div className={cx('log-entry-content', className)}>
      {formattedContent && <div className="log-entry-formatted-content">{formattedContent}</div>}

      {extraInfoURL && (
        <div className="log-entry-content-link">
          <a href={extraInfoURL} target="_blank" rel="noopener">
            {t('log_hint_extra_info')}
          </a>
        </div>
      )}

      {rawContent && <PdfLogEntryRawContent rawContent={rawContent} collapsedSize={150} alwaysExpanded={alwaysExpandRawContent} />}
    </div>
  )
}
