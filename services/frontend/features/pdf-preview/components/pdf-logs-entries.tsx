'use client'

import { memo } from 'react'
import { useTranslation } from '@/lib/i18n'
import PreviewLogsPaneMaxEntries from './preview-logs-pane-max-entries'
import PdfLogEntry from './pdf-log-entry'
import { useCompile } from '@/features/ide/contexts/compile-context'
import type { LogEntry } from '../util/types'

const LOG_PREVIEW_LIMIT = 100

/** The entries of one tab, the first open, and no more than a hundred. */
function PdfLogsEntries({ entries, hasErrors }: { entries: LogEntry[]; hasErrors?: boolean }) {
  const { t } = useTranslation()
  const { syncToEntry } = useCompile()
  const logEntries = entries.slice(0, LOG_PREVIEW_LIMIT)

  return (
    <>
      {entries.length > LOG_PREVIEW_LIMIT && (
        <PreviewLogsPaneMaxEntries totalEntries={entries.length} entriesShown={LOG_PREVIEW_LIMIT} hasErrors={hasErrors} />
      )}

      {logEntries.map((logEntry, index) => (
        <PdfLogEntry
          key={logEntry.key}
          autoExpand={index === 0}
          index={index}
          id={logEntry.key}
          logEntry={logEntry}
          ruleId={logEntry.ruleId}
          headerTitle={logEntry.messageComponent ?? logEntry.message}
          rawContent={logEntry.content}
          logType={logEntry.type}
          level={logEntry.level}
          contentDetails={logEntry.contentDetails}
          entryAriaLabel={t('log_entry_description', {
            level: logEntry.level,
          })}
          sourceLocation={{
            file: logEntry.file,
            line: logEntry.line,
            column: logEntry.column,
          }}
          onSourceLocationClick={syncToEntry}
        />
      ))}
    </>
  )
}

export default memo(PdfLogsEntries)
