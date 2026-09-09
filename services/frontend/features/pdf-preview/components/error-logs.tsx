'use client'

/**
 * The compile log as a list of entries under four tabs (all, errors,
 * warnings, info), from pdf-preview/components/error-logs.
 */

import { memo, useCallback, useMemo, useState } from 'react'
import { useTranslation } from '@/lib/i18n'
import cx from '@/lib/cx'
import { usePdfPreviewContext } from './pdf-preview-provider'
import StopOnFirstErrorPrompt from './stop-on-first-error-prompt'
import PdfPreviewError from './pdf-preview-error'
import PdfValidationIssue from './pdf-validation-issue'
import PdfLogsEntries from './pdf-logs-entries'
import PdfPreviewErrorBoundaryFallback from './pdf-preview-error-boundary-fallback'
import { withErrorBoundary } from '@/components/ol/error-boundary'
import { useCompile } from '@/features/ide/contexts/compile-context'
import type { LogEntry as LogEntryData } from '../util/types'
import LogEntry from './log-entry'
import PdfClearCacheButton from './pdf-clear-cache-button'
import PdfDownloadFilesButton from './pdf-download-files-button'

type ErrorLogTab = {
  key: string
  label: string
  entries: LogEntryData[] | undefined
}

function ErrorLogs({ includeActionButtons }: { includeActionButtons?: boolean }) {
  const { error, logEntries, rawLog, validationIssues, stoppedOnFirstError } = useCompile()
  const { t } = useTranslation()

  const tabs = useMemo<ErrorLogTab[]>(() => {
    return [
      {
        key: 'all',
        label: t('all_logs'),
        entries: logEntries?.all,
      },
      { key: 'errors', label: t('errors'), entries: logEntries?.errors },
      { key: 'warnings', label: t('warnings'), entries: logEntries?.warnings },
      { key: 'info', label: t('info'), entries: logEntries?.typesetting },
    ]
  }, [logEntries, t])

  const { loadingError } = usePdfPreviewContext()

  const [activeTab, setActiveTab] = useState<string>('all')

  const changeTab = useCallback(
    (key: string) => {
      if (tabs.some(tab => tab.key === key)) {
        setActiveTab(key)
      }
    },
    [tabs]
  )

  const entries = useMemo(() => {
    return tabs.find(tab => tab.key === activeTab)?.entries || []
  }, [activeTab, tabs])

  const includeErrors = activeTab === 'all' || activeTab === 'errors'
  const includeWarnings = activeTab === 'all' || activeTab === 'warnings'

  return (
    <>
      <div className="nav error-logs-tabs" role="tablist">
        {tabs.map(tab => (
          <TabHeader key={tab.key} tab={tab} active={activeTab === tab.key} onSelect={changeTab} />
        ))}
      </div>
      <div className="tab-content error-logs new-error-logs">
        <div className="logs-pane-content" role="tabpanel">
          {stoppedOnFirstError && includeErrors && <StopOnFirstErrorPrompt />}

          {loadingError && (
            <PdfPreviewError error="pdf-viewer-loading-error" includeErrors={includeErrors} includeWarnings={includeWarnings} />
          )}

          {error && <PdfPreviewError error={error} />}

          {includeErrors &&
            validationIssues &&
            Object.entries(validationIssues).map(([name, issue]) => <PdfValidationIssue key={name} name={name} issue={issue} />)}

          {entries && (
            <PdfLogsEntries entries={entries} hasErrors={includeErrors && logEntries?.errors && logEntries?.errors.length > 0} />
          )}

          {rawLog && activeTab === 'all' && (
            <LogEntry
              headerTitle={t('raw_logs')}
              rawContent={rawLog}
              entryAriaLabel={t('raw_logs_description')}
              level="raw"
              alwaysExpandRawContent
              showSourceLocationLink={false}
            />
          )}

          {includeActionButtons && (
            <div className="logs-pane-actions">
              <PdfClearCacheButton />
              <PdfDownloadFilesButton />
            </div>
          )}
        </div>
      </div>
    </>
  )
}

function formatErrorNumber(num: number | undefined) {
  if (num === undefined) {
    return undefined
  }

  if (num > 99) {
    return '99+'
  }

  return Math.floor(num).toString()
}

const TabHeader = ({ tab, active, onSelect }: { tab: ErrorLogTab; active: boolean; onSelect: (key: string) => void }) => {
  return (
    <button
      type="button"
      role="tab"
      className={cx('nav-link', 'error-logs-tab-header', { active })}
      aria-selected={active}
      onClick={() => onSelect(tab.key)}
    >
      {tab.label}
      <div className="error-logs-tab-count">{formatErrorNumber(tab.entries?.length)}</div>
    </button>
  )
}

export default withErrorBoundary(memo(ErrorLogs), () => <PdfPreviewErrorBoundaryFallback type="logs" />)
