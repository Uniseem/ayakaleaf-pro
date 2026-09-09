'use client'

import { forwardRef, memo, useCallback } from 'react'
import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import { useCompile } from '@/features/ide/contexts/compile-context'
import Tooltip from '@/components/ol/tooltip'
import { Button } from '@/components/ol/button'
import { OLBadge } from '@/components/ol/badge'

/**
 * The logs button: a document icon with a count of errors and warnings on
 * it, which becomes "Back to PDF" while the logs are showing.
 */
const PdfHybridLogsButton = forwardRef<HTMLButtonElement>((_, ref) => {
  const { error, logEntries, toggleLogs, showLogs, stoppedOnFirstError } = useCompile()

  const { t } = useTranslation()

  const handleClick = useCallback(() => {
    toggleLogs()
  }, [toggleLogs])

  const errorCount = Number(logEntries?.errors?.length)
  const warningCount = Number(logEntries?.warnings?.length)
  const totalCount = errorCount + warningCount

  if (showLogs) {
    return (
      <Button ref={ref} variant="secondary" size="sm" disabled={Boolean(error || stoppedOnFirstError)} onClick={handleClick}>
        {t('back_to_pdf')}
      </Button>
    )
  }

  return (
    <Tooltip id="logs-toggle" description={t('logs_and_output_files')} overlayProps={{ placement: 'bottom' }}>
      <Button
        ref={ref}
        variant="link"
        disabled={Boolean(error || stoppedOnFirstError)}
        className="pdf-toolbar-btn toolbar-item log-btn"
        onClick={handleClick}
        style={{ position: 'relative' }}
        aria-label={t('view_logs')}
      >
        <MaterialIcon type="description" />

        {totalCount > 0 && <OLBadge bg={errorCount === 0 ? 'warning' : 'danger'}>{totalCount}</OLBadge>}
      </Button>
    </Tooltip>
  )
})

PdfHybridLogsButton.displayName = 'PdfHybridLogsButton'

export default memo(PdfHybridLogsButton)
