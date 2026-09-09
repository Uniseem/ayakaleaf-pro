'use client'

import { memo, useState, type Dispatch, type MouseEventHandler, type ReactNode, type SetStateAction } from 'react'
import { hintFor } from '../human-readable-logs/hints'
import type { ErrorLevel, LogEntry as LogEntryData, SourceLocation } from '../util/types'
import LogEntryHeader from './log-entry-header'
import PdfLogEntryContent from './pdf-log-entry-content'
import cx from '@/lib/cx'

type LogEntryProps = {
  headerTitle: string | ReactNode
  level: ErrorLevel
  ruleId?: string
  rawContent?: string
  logType?: string
  formattedContent?: ReactNode
  extraInfoURL?: string | null
  sourceLocation?: SourceLocation
  showSourceLocationLink?: boolean
  entryAriaLabel?: string
  contentDetails?: string[]
  onSourceLocationClick?: MouseEventHandler<HTMLButtonElement>
  index?: number
  logEntry?: LogEntryData
  id?: string
  alwaysExpandRawContent?: boolean
  className?: string
  actionButtonsOverride?: ReactNode
  openCollapseIconOverride?: string
}

/** A card in the logs pane: a header that opens and closes the detail. */
function LogEntry(props: LogEntryProps & { autoExpand?: boolean }) {
  const [collapsed, setCollapsed] = useState(!props.autoExpand)

  return <ControlledLogEntry {...props} collapsed={collapsed} setCollapsed={setCollapsed} />
}

export function ControlledLogEntry({
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
  alwaysExpandRawContent = false,
  className,
  collapsed,
  setCollapsed,
  actionButtonsOverride,
  openCollapseIconOverride,
}: LogEntryProps & {
  collapsed: boolean
  setCollapsed: Dispatch<SetStateAction<boolean>>
}) {
  const hint = ruleId ? hintFor(ruleId) : undefined
  if (hint) {
    formattedContent = hint.formattedContent(contentDetails)
    extraInfoURL = hint.extraInfoURL
  }

  return (
    <div className={cx('log-entry', className)} data-ruleid={ruleId} data-log-entry-id={id} aria-label={entryAriaLabel}>
      <LogEntryHeader
        level={level}
        sourceLocation={sourceLocation}
        headerTitle={headerTitle}
        logType={logType}
        showSourceLocationLink={showSourceLocationLink}
        onSourceLocationClick={onSourceLocationClick}
        collapsed={collapsed}
        onToggleCollapsed={() => setCollapsed(collapsed => !collapsed)}
        id={id}
        logEntry={logEntry}
        actionButtonsOverride={actionButtonsOverride}
        openCollapseIconOverride={openCollapseIconOverride}
      />
      <div className={cx('horizontal-divider', { hidden: collapsed })} />
      <PdfLogEntryContent
        className={cx({ hidden: collapsed })}
        alwaysExpandRawContent={alwaysExpandRawContent}
        rawContent={rawContent}
        formattedContent={formattedContent}
        extraInfoURL={extraInfoURL}
        index={index}
        logEntry={logEntry}
      />
    </div>
  )
}

export default memo(LogEntry)
