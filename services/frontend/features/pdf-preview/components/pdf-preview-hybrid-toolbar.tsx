'use client'

import { memo } from 'react'
import { useTranslation } from '@/lib/i18n'
import { useLayout } from '@/features/ide/contexts/layout-context'
import PdfCompileButton from './pdf-compile-button'
import PdfHybridDownloadButton from './pdf-hybrid-download-button'
import SwitchToEditorButton from './switch-to-editor-button'
import PdfHybridLogsButton from './pdf-hybrid-logs-button'

/**
 * The bar across the top of the PDF pane: compile, logs and download on the
 * left; the viewer's own controls (which it puts into the empty element
 * here) and the way back to the editor on the right.
 */
function PdfPreviewHybridToolbar() {
  const { t } = useTranslation()

  return (
    <div role="toolbar" className="btn-toolbar toolbar toolbar-pdf toolbar-pdf-hybrid" aria-label={t('pdf')}>
      <PdfPreviewHybridToolbarInner />
    </div>
  )
}

function PdfPreviewHybridToolbarInner() {
  const { focusMode } = useLayout()
  return (
    <>
      <div className="toolbar-pdf-left">
        <PdfCompileButton />
        <PdfHybridLogsButton />
        <PdfHybridDownloadButton />
      </div>
      <div className="toolbar-pdf-right">
        <div className="toolbar-pdf-controls" id="toolbar-pdf-controls" />
        {!focusMode && <SwitchToEditorButton />}
      </div>
    </>
  )
}

export default memo(PdfPreviewHybridToolbar)
