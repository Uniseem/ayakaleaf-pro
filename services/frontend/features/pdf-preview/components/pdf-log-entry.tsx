'use client'

import { memo, type ReactNode } from 'react'
import { hintFor } from '../human-readable-logs/hints'
import type { ErrorLevel, LogEntry, SourceLocation } from '../util/types'
import NewLogEntry from './log-entry'
import useHandleLogEntryClick from '../hooks/use-handle-log-entry-click'

/**
 * One entry of the log with, when the rule it matched has one, a hint
 * explaining it in place of the raw message.
 */
function PdfLogEntry({
  autoExpand,
  ruleId,
  headerTitle,
  rawContent,
  logType,
  formattedContent,
  extraInfoURL,
  level,
  sourceLocation,
  showSourceLocationLink = true,
  entryAriaLabel = undefined,
  contentDetails,
  onSourceLocationClick,
  index,
  logEntry,
  id,
}: {
  headerTitle: string | ReactNode
  level: ErrorLevel
  autoExpand?: boolean
  ruleId?: string
  rawContent?: string
  logType?: string
  formattedContent?: ReactNode
  extraInfoURL?: string | null
  sourceLocation?: SourceLocation
  showSourceLocationLink?: boolean
  entryAriaLabel?: string
  contentDetails?: string[]
  onSourceLocationClick?: (sourceLocation: SourceLocation) => void
  index?: number
  logEntry?: LogEntry
  id?: string
}) {
  const hint = ruleId ? hintFor(ruleId) : undefined
  if (hint) {
    formattedContent = hint.formattedContent(contentDetails)
    extraInfoURL = hint.extraInfoURL
  }

  const handleLogEntryLinkClick = useHandleLogEntryClick({
    level,
    ruleId,
    sourceLocation,
    onSourceLocationClick,
  })

  return (
    <NewLogEntry
      autoExpand={autoExpand}
      index={index}
      id={id}
      logEntry={logEntry}
      ruleId={ruleId}
      headerTitle={headerTitle}
      formattedContent={formattedContent}
      rawContent={rawContent}
      logType={logType}
      level={level}
      contentDetails={contentDetails}
      entryAriaLabel={entryAriaLabel}
      sourceLocation={sourceLocation}
      onSourceLocationClick={handleLogEntryLinkClick}
      showSourceLocationLink={showSourceLocationLink}
      extraInfoURL={extraInfoURL}
    />
  )
}

export default memo(PdfLogEntry)
