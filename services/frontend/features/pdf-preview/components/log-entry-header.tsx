'use client'

import { useState, useCallback, useMemo, type MouseEventHandler, type KeyboardEventHandler, type ReactNode } from 'react'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import Tooltip from '@/components/ol/tooltip'
import type { ErrorLevel, SourceLocation, LogEntry as LogEntryData } from '../util/types'
import { useResizeObserver } from '@/features/source-editor/hooks/use-resize-observer'
import IconButton from '@/components/ol/icon-button'
import MaterialIcon from '@/components/ol/material-icon'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'

/**
 * The top line of a log entry: the level in colour, the message, where it
 * came from, and a button that goes there.
 */
function LogEntryHeader({
  sourceLocation,
  level,
  headerTitle,
  logType,
  showSourceLocationLink = true,
  onSourceLocationClick,
  collapsed,
  onToggleCollapsed,
  actionButtonsOverride,
  openCollapseIconOverride,
}: {
  headerTitle: string | ReactNode
  level: ErrorLevel
  logType?: string
  sourceLocation?: SourceLocation
  showSourceLocationLink?: boolean
  onSourceLocationClick?: MouseEventHandler<HTMLButtonElement>
  collapsed: boolean
  onToggleCollapsed: () => void
  id?: string
  logEntry?: LogEntryData
  actionButtonsOverride?: ReactNode
  openCollapseIconOverride?: string
}) {
  const { t } = useTranslation()
  const [locationSpanOverflown, setLocationSpanOverflown] = useState(false)
  const { entryByPath } = useProject()
  const editor = useEditor()
  const openDocId = editor.current?.id ?? editor.currentFile?.id

  const { elementRef: logLocationSpanRef } = useResizeObserver(
    useCallback((element: Element) => {
      setLocationSpanOverflown(element.scrollWidth > element.clientWidth)
    }, [])
  )

  const file = sourceLocation ? sourceLocation.file : null
  const line = sourceLocation ? sourceLocation.line : null
  const logEntryHeaderTextClasses = cx('log-entry-header-text', {
    'log-entry-header-text-error': level === 'error',
    'log-entry-header-text-warning': level === 'warning',
    'log-entry-header-text-info': level === 'info' || level === 'typesetting',
    'log-entry-header-text-success': level === 'success',
    'log-entry-header-text-raw': level === 'raw',
  })

  const locationText = showSourceLocationLink && file ? `${file}${line ? `, ${line}` : ''}` : null

  // Because we want an ellipsis on the left-hand side (e.g. "...longfilename.tex"), the
  // `log-entry-location` class has text laid out from right-to-left using the CSS
  // rule `direction: rtl;`.
  // This works most of the times, except when the first character of the filename is considered
  // a punctuation mark, like `/` (e.g. `/foo/bar/baz.sty`). In this case, because of
  // right-to-left writing rules, the punctuation mark is moved to the right-side of the string,
  // resulting in `...bar/baz.sty/` instead of `...bar/baz.sty`.
  // To avoid this edge-case, we wrap the `logLocationLinkText` in two directional formatting
  // characters:
  //   * ‪ LEFT-TO-RIGHT EMBEDDING Treat the following text as embedded left-to-right.
  //   * ‬ POP DIRECTIONAL FORMATTING End the scope of the last LRE, RLE, RLO, or LRO.
  // This essentially tells the browser that, althought the text is laid out from right-to-left,
  // the wrapped portion of text should follow left-to-right writing rules.
  const formattedLocationText = locationText ? (
    <span ref={logLocationSpanRef} className="log-entry-location">
      {`‪${locationText}‬`}
    </span>
  ) : null

  const headerTitleText = logType ? `${logType} ${headerTitle}` : headerTitle
  const fileData = useMemo(() => {
    if (!file) return null
    const path = file.replace(/^\.\//, '')
    return entryByPath(path) ?? entryByPath('/' + path) ?? null
  }, [file, entryByPath])
  const showGoToCodeButton = showSourceLocationLink && !!fileData && !(fileData.id === openDocId && !line)

  const handleKeyDown: KeyboardEventHandler<HTMLDivElement> = useCallback(
    event => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault()
        onToggleCollapsed()
      }
    },
    [onToggleCollapsed]
  )

  return (
    <header className="log-entry-header-card">
      <div
        data-action="expand-collapse"
        data-collapsed={collapsed}
        className="log-entry-header-button"
        onClick={onToggleCollapsed}
        role="button"
        tabIndex={0}
        onKeyDown={handleKeyDown}
        aria-label={collapsed ? t('expand') : t('collapse')}
      >
        <MaterialIcon
          className="log-entry-expand-icon"
          type={openCollapseIconOverride ?? (collapsed ? 'chevron_right' : 'expand_more')}
        />
        <div className="log-entry-header-content">
          <h3 className={logEntryHeaderTextClasses}>{headerTitleText}</h3>
          {locationSpanOverflown && formattedLocationText && locationText ? (
            <Tooltip
              id={locationText}
              description={locationText}
              overlayProps={{ placement: 'right' }}
              tooltipProps={{ className: 'log-location-tooltip' }}
            >
              {formattedLocationText}
            </Tooltip>
          ) : (
            formattedLocationText
          )}
        </div>
      </div>

      {actionButtonsOverride ?? (
        <div className="log-entry-header-actions">
          {showGoToCodeButton && (
            <Tooltip id={`go-to-location-${locationText}`} description={t('go_to_code_location')} overlayProps={{ placement: 'bottom' }}>
              <IconButton
                onClick={onSourceLocationClick}
                variant="ghost"
                icon="my_location"
                accessibilityLabel={t('go_to_code_location')}
              />
            </Tooltip>
          )}
        </div>
      )}
    </header>
  )
}

export default LogEntryHeader
