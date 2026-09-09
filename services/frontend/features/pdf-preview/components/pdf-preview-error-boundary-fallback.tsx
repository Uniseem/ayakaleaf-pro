'use client'

import { Trans, useTranslation } from '@/lib/i18n'
import { ErrorBoundaryFallback } from '@/components/ol/error-boundary'

/** What stands in for the preview, the PDF or the logs when one of them throws. */
function PdfPreviewErrorBoundaryFallback({ type }: { type: 'preview' | 'pdf' | 'logs' }) {
  const { t } = useTranslation()

  const showInfoLink = (
    <Trans
      i18nKey="try_recompile_project_or_troubleshoot"
      components={[<a href="/learn/how-to/Resolving_access%2C_loading%2C_and_display_problems" target="_blank" key="troubleshooting-link" />]}
    />
  )

  switch (type) {
    case 'pdf':
      return (
        <ErrorBoundaryFallback>
          <p>{t('pdf_viewer_error')}</p>
          <p>{showInfoLink}</p>
        </ErrorBoundaryFallback>
      )

    case 'logs':
      return (
        <ErrorBoundaryFallback>
          <p>{t('log_viewer_error')}</p>
          <p>{showInfoLink}</p>
        </ErrorBoundaryFallback>
      )

    case 'preview':
    default:
      return (
        <ErrorBoundaryFallback>
          <p>{t('pdf_preview_error')}</p>
          <p>{showInfoLink}</p>
        </ErrorBoundaryFallback>
      )
  }
}

export default PdfPreviewErrorBoundaryFallback
