'use client'

import { lazy, memo } from 'react'
import { useCompile } from '@/features/ide/contexts/compile-context'

const PdfJsViewer = lazy(() => import('./pdf-js-viewer'))

/** The PDF, in pdf.js or the browser's own viewer as the settings say. */
function PdfViewer() {
  const { pdfUrl, pdfFile, pdfViewer } = useCompile()

  if (!pdfUrl || !pdfFile) {
    return null
  }

  switch (pdfViewer) {
    case 'native':
      return <iframe title="PDF Preview" src={pdfUrl} />

    case 'pdfjs':
    default:
      return <PdfJsViewer url={pdfUrl} pdfFile={pdfFile} />
  }
}

export default memo(PdfViewer)
