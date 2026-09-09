'use client'

import { useCallback, useId } from 'react'
import Tooltip from '@/components/ol/tooltip'
import { useTranslation } from '@/lib/i18n'
import { useCompile } from '@/features/ide/contexts/compile-context'
import IconButton from '@/components/ol/icon-button'

/** Inverts the PDF's colours, offered only in the dark theme. */
export const PdfHybridThemeButton = () => {
  const id = useId()
  const { t } = useTranslation()
  const { pdfViewer, darkModePdf, setDarkModePdf, activeOverallTheme, showLogs } = useCompile()

  const onClick = useCallback(() => {
    setDarkModePdf(!darkModePdf)
  }, [darkModePdf, setDarkModePdf])

  if (activeOverallTheme !== 'dark') {
    return null
  }

  if (pdfViewer !== 'pdfjs') {
    // the embedded viewer's theme is the browser's, not ours
    return null
  }

  if (showLogs) {
    return null
  }

  const tooltipText = darkModePdf ? t('showing_pdf_preview_with_inverted_colors') : t('invert_pdf_preview_colors')

  return (
    <Tooltip id={id} description={tooltipText} overlayProps={{ placement: 'bottom' }}>
      <IconButton
        icon="invert_colors"
        accessibilityLabel={tooltipText}
        variant="link"
        active={darkModePdf}
        className="pdf-toolbar-btn toolbar-item theme-toggle-btn"
        onClick={onClick}
        style={{ position: 'relative' }}
      />
    </Tooltip>
  )
}
