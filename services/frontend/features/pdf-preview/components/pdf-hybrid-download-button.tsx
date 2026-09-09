'use client'

import { useTranslation } from '@/lib/i18n'
import { useCompile } from '@/features/ide/contexts/compile-context'
import Tooltip from '@/components/ol/tooltip'
import { Button } from '@/components/ol/button'
import MaterialIcon from '@/components/ol/material-icon'

/** Downloads the PDF; a link that does nothing until there is one. */
function PdfHybridDownloadButton() {
  const { pdfDownloadUrl, showLogs } = useCompile()
  const { t } = useTranslation()

  if (showLogs) {
    return null
  }
  const description = pdfDownloadUrl ? t('download_pdf') : t('please_compile_pdf_before_download')

  function handleOnClick(e: React.MouseEvent) {
    const event = e as React.MouseEvent<HTMLAnchorElement>
    if (event.currentTarget.dataset.disabled === 'true') {
      event.preventDefault()
    }
  }

  return (
    <Tooltip id="download-pdf" description={description} overlayProps={{ placement: 'bottom' }}>
      <Button
        onClick={handleOnClick}
        variant="link"
        className="pdf-toolbar-btn"
        draggable={false}
        disabled={!pdfDownloadUrl}
        download
        href={pdfDownloadUrl || '#'}
        target="_blank"
        style={{ pointerEvents: 'auto' }}
        aria-label={t('download_pdf')}
      >
        <MaterialIcon type="download" />
      </Button>
    </Tooltip>
  )
}

export default PdfHybridDownloadButton
