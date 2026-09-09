'use client'

import { memo, Suspense } from 'react'
import cx from '@/lib/cx'
import PdfViewer from './pdf-viewer'
import { FullSizeLoadingSpinner } from '@/components/ol/spinner'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { PdfPreviewMessages } from './pdf-preview-messages'
import { PdfPreviewProvider } from './pdf-preview-provider'
import PdfPreviewHybridToolbar from './pdf-preview-hybrid-toolbar'
import PdfCodeCheckFailedBanner from './pdf-code-check-failed-banner'
import PdfLogsViewer from './pdf-logs-viewer'

/**
 * The PDF pane: the toolbar, the viewer, and the logs that take its place
 * when asked for. From pdf-preview/components/pdf-preview-pane.
 */
function PdfPreviewPane() {
  const { pdfUrl, pdfViewer, darkModePdf: darkModeSetting, activeOverallTheme } = useCompile()
  const darkModePdf = pdfViewer === 'pdfjs' && activeOverallTheme === 'dark' && darkModeSetting

  const classes = cx('pdf', 'full-size', {
    'pdf-empty': !pdfUrl,
    'pdf-dark-mode': darkModePdf,
  })

  return (
    <div className={classes}>
      <PdfPreviewProvider>
        <PdfPreviewHybridToolbar />
        <PdfCodeCheckFailedBanner />
        <PdfPreviewMessages />
        <Suspense fallback={<FullSizeLoadingSpinner delay={500} />}>
          <div className="pdf-viewer" data-testid="pdf-viewer">
            <PdfViewer />
          </div>
        </Suspense>
        <PdfLogsViewer />
      </PdfPreviewProvider>
    </div>
  )
}

export default memo(PdfPreviewPane)
